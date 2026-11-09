#!/usr/bin/env python3
"""Operator-only subscription setup. Secret values travel over stdin, never argv/logs."""
import argparse
import base64
import ipaddress
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import time
from urllib.request import Request, urlopen
import uuid

ROOT = Path(__file__).resolve().parent.parent
LOCAL = ROOT / ".local"


def private_json(path, data):
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w") as stream:
        json.dump(data, stream)
    path.chmod(0o600)


def vault(arguments, payload=None, token="", allowed=(0,)):
    command = ["docker", "compose", "exec", "-T", "vault", "sh", "-c",
               'IFS= read -r VAULT_TOKEN; export VAULT_TOKEN; '
               'export VAULT_ADDR=https://vault:8200 VAULT_CACERT=/vault/tls/cert.pem; '
               'exec vault "$@"', "vault-private", *arguments]
    data = token + "\n"
    if payload is not None:
        data += payload if isinstance(payload, str) else json.dumps(payload)
    result = subprocess.run(command, cwd=ROOT, input=data, text=True,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=35)
    if result.returncode not in allowed:
        # Never reflect Vault error bodies, secret payloads or process arguments.
        raise SystemExit("Vault operation failed. Check listener, seal state and operator permissions.")
    if not result.stdout.strip():
        return None
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError:
        return None


def init_vault(rotate_token=False):
    status = vault(["status", "-format=json"], allowed=(0, 2))
    backup = LOCAL / "vault-init.json"
    if not status["initialized"]:
        if backup.exists():
            raise SystemExit("Vault recovery file already exists for another initialization; refusing to replace it.")
        result = vault(["operator", "init", "-format=json", "-key-shares=1", "-key-threshold=1"])
        private_json(backup, result)
    if not backup.exists():
        raise SystemExit("This Vault was initialized elsewhere. Local operator recovery material is unavailable.")
    recovery = json.loads(backup.read_text())
    token = recovery["root_token"]
    if status["sealed"]:
        vault(["write", "-format=json", "sys/unseal", "-"],
              {"key": recovery["unseal_keys_b64"][0]}, token)
    mounts = vault(["secrets", "list", "-format=json"], token=token)
    if "subscription-auth/" not in mounts:
        vault(["secrets", "enable", "-path=subscription-auth", "-version=2", "kv"], token=token)
    elif mounts["subscription-auth/"].get("options", {}).get("version") != "2":
        raise SystemExit("subscription-auth must be a dedicated KV v2 mount.")
    policy = '\n'.join('path "subscription-auth/data/local-operator/'+provider+'" { capabilities = ["read"] }'
                       for provider in ("codex", "claude"))
    vault(["policy", "write", "workbench-subscriptions", "-"], policy, token)
    token_path = LOCAL / "secrets/subscription_vault_token"
    existing = token_path.read_text().strip() if token_path.exists() else ""
    if rotate_token and existing:
        # Revoke using operator authority even when the token itself has expired.
        vault(["write", "-format=json", "auth/token/revoke", "-"], {"token": existing}, token)
    if existing and not rotate_token:
        renewed = vault(["token", "renew", "-format=json"], token=existing, allowed=(0, 2))
        if not renewed or not renewed.get("auth"):
            existing = ""
            print("Expired gateway Vault lease detected; provisioning a replacement.")
    if not existing or rotate_token:
        created = vault(["token", "create", "-format=json", "-orphan", "-policy=workbench-subscriptions",
                         "-period=24h", "-display-name=workbench-subscription-gateway"], token=token)
        token_path.write_text(created["auth"]["client_token"])
        token_path.chmod(0o644)  # Only mounted in gateway; parent directories are 0700.
        print("Gateway token replaced. Recreate inference-gateway to load it.")
    print("Vault unsealed; dedicated KV v2 storage and read-only gateway policy ready.")
    print("Local development recovery material is protected in .local/vault-init.json.")


def jwt_claims(token):
    try:
        value = token.split(".")[1]
        return json.loads(base64.urlsafe_b64decode(value + "=" * (-len(value) % 4)))
    except (IndexError, ValueError):
        raise SystemExit("The saved subscription access token has no usable expiry. Sign in again.")


def sync(provider, auth_file):
    if auth_file:
        path = Path(auth_file).expanduser()
    elif provider == "codex":
        path = Path(os.environ.get("CODEX_HOME", str(Path.home() / ".codex"))) / "auth.json"
    else:
        local_login = LOCAL / "native-logins/claude/.claude/.credentials.json"
        path = local_login if local_login.is_file() else Path(os.environ.get("CLAUDE_CONFIG_DIR", str(Path.home() / ".claude"))) / ".credentials.json"
    if not path.is_file():
        raise SystemExit("Saved login unavailable. Sign in with the native "+provider+" CLI, then retry sync.")
    saved = json.loads(path.read_text())
    if provider == "codex":
        if saved.get("auth_mode") != "chatgpt":
            raise SystemExit("Codex must be signed in with ChatGPT, not an API key.")
        tokens = saved.get("tokens", {})
        access = tokens.get("access_token", "")
        claims = jwt_claims(access)
        secret = {"access_token": access, "account_id": tokens.get("account_id", ""), "expires_at": claims.get("exp", 0)}
        if not secret["account_id"]:
            raise SystemExit("The saved Codex subscription has no account binding.")
    else:
        tokens = saved.get("claudeAiOauth", {})
        access = tokens.get("accessToken", "")
        secret = {"access_token": access, "expires_at": int(tokens.get("expiresAt", 0)) // 1000}
    if not access or secret["expires_at"] <= time.time() + 60:
        raise SystemExit("Subscription access token is missing or expired. Refresh the native CLI login, then sync again.")
    recovery = json.loads((LOCAL / "vault-init.json").read_text())
    vault(["kv", "put", "-format=json", "subscription-auth/local-operator/"+provider, "-"], secret, recovery["root_token"])
    print(provider+" subscription access token stored in Vault; refresh tokens and host login settings were not copied.")


def configure(default, codex_model, claude_model):
    models = {"codex": codex_model, "claude": claude_model}
    if not models[default]:
        raise SystemExit("An exact model ID is required for the default native agent.")
    origins = {"codex": "chatgpt.com", "claude": "api.anthropic.com"}
    providers = {}
    for provider, model in models.items():
        if not model:
            continue
        if any(c.isspace() for c in model) or len(model) > 128:
            raise SystemExit("Invalid model ID.")
        addresses = sorted({entry[4][0] for entry in socket.getaddrinfo(origins[provider], 443, type=socket.SOCK_STREAM)})
        if not addresses or any(not ipaddress.ip_address(address).is_global for address in addresses):
            raise SystemExit("Provider DNS did not return exclusively public destinations.")
        providers[provider] = {"models": [model], "network": {"allowed_cidrs": [address+("/128" if ":" in address else "/32") for address in addresses]}}
    target = LOCAL / "inference.json"
    source = target if target.exists() else ROOT / "config/inference.json"
    config = json.loads(source.read_text())
    private_json(LOCAL / "inference-before-subscriptions.json", config) if "subscriptions" not in config else None
    config["subscriptions"] = {"providers": providers, "principals": {"local-operator": list(providers)}}
    target.write_text(json.dumps(config, indent=2)+"\n")
    env_path = ROOT / ".env"
    lines = env_path.read_text().splitlines()
    values = {"WORKSPACE_AGENT": default, "CODEX_MODEL": codex_model, "CLAUDE_MODEL": claude_model,
              "INFERENCE_POLICY_FILE": "./.local/inference.json"}
    lines = [line for line in lines if not any(line.startswith(key+"=") for key in values)]
    env_path.write_text("\n".join(lines+[key+"="+value for key, value in values.items()])+"\n")
    print("Native provider/model allowlists written. Only current provider IPs are permitted; DNS changes fail closed.")
    print("Apply with: docker compose up -d --build --wait inference-gateway runtime-controller controlplane mcp")
    print("Create a new workspace to receive its new native-agent defaults. Existing containers are preserved.")


def reconnect_running_workspaces():
    # Recreated gateway containers lose dynamic workspace network attachments.
    # Idempotent start reconciles those attachments without restarting agents.
    result = subprocess.run(["docker", "compose", "port", "controlplane", "8080"], cwd=ROOT,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, timeout=15)
    endpoint = result.stdout.strip()
    host, _, port = endpoint.rpartition(":")
    if result.returncode or not port.isdigit() or not ipaddress.ip_address(host.strip("[]")).is_loopback:
        raise SystemExit("Gateway recovered. Open each running workspace from the launcher to restore its network attachment.")
    origin = "http://" + endpoint
    token = (LOCAL / "secrets/control_token").read_text().strip()
    headers = {"Authorization": "Bearer " + token, "Content-Type": "application/json"}
    with urlopen(Request(origin + "/v1/workspaces", headers=headers), timeout=40) as response:
        workspaces = json.load(response)["items"]
    for workspace in workspaces:
        if workspace["state"] == "STOPPED":
            continue
        request = Request(origin + "/v1/workspaces/" + workspace["id"] + "/start",
                          data=json.dumps({"idempotency_key": "repair-"+uuid.uuid4().hex}).encode(), headers=headers)
        with urlopen(request, timeout=40) as response:
            response.read(1 << 20)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    command = sub.add_parser("init-vault", help="Initialize/unseal this local development Vault and provision gateway policy")
    command.add_argument("--rotate-token", action="store_true", help="Replace the gateway's Vault token; recreate gateway afterwards")
    command = sub.add_parser("login", help="Run the baked native CLI's subscription sign-in in an operator-only container")
    command.add_argument("provider", choices=["claude"])
    command = sub.add_parser("sync", help="Copy an existing subscription access token into Vault")
    command.add_argument("provider", choices=["codex", "claude"])
    command.add_argument("--auth-file")
    sub.add_parser("repair", help="Recover Vault access, sync the existing Codex login and recreate the gateway")
    command = sub.add_parser("configure", help="Enable fixed native provider routes and choose the default agent")
    command.add_argument("--default", choices=["codex", "claude"], default="codex")
    command.add_argument("--codex-model", default="gpt-6.1-sol")
    command.add_argument("--claude-model", default="")
    args = parser.parse_args()
    try:
        if args.command == "init-vault":
            init_vault(args.rotate_token)
        elif args.command == "login":
            if not sys.stdin.isatty():
                raise SystemExit("Run this login command in your interactive terminal so you can complete the provider sign-in.")
            home = LOCAL / "native-logins/claude"
            home.mkdir(parents=True, mode=0o700, exist_ok=True)
            # This is a trusted operator login container, not an agent workspace.
            # Only its private login cache is mounted; no source or runtime socket.
            result = subprocess.run(["docker", "run", "--rm", "-it", "--cap-drop=ALL",
                                     "--security-opt=no-new-privileges", "--user", str(os.getuid())+":"+str(os.getgid()),
                                     "--mount", "type=bind,src="+str(home)+",dst=/auth", "-e", "HOME=/auth",
                                     "-e", "CLAUDE_CONFIG_DIR=/auth/.claude", "--entrypoint", "claude",
                                     "agent-workbench-workspace:dev", "auth", "login"], cwd=ROOT)
            if result.returncode:
                raise SystemExit("Claude sign-in did not complete.")
            print("Sign-in complete. Run: python3 scripts/subscription_auth.py sync claude")
        elif args.command == "sync":
            sync(args.provider, args.auth_file)
        elif args.command == "repair":
            init_vault()
            sync("codex", None)
            result = subprocess.run(["docker", "compose", "up", "-d", "--force-recreate", "--no-deps", "--wait", "inference-gateway"], cwd=ROOT)
            if result.returncode:
                raise SystemExit("Gateway restart failed. Vault data and workspace storage were preserved.")
            reconnect_running_workspaces()
            print("Codex credential access repaired. Retry your task in T3.")
        else:
            configure(args.default, args.codex_model, args.claude_model)
    except (OSError, ValueError, KeyError, subprocess.TimeoutExpired):
        raise SystemExit("Setup failed. Check local login/recovery files, Docker access and Vault availability; no secrets were printed.")


if __name__ == "__main__":
    main()
