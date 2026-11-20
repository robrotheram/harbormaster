# Subscription credentials

## Personal agent subscriptions

Open **Connections** from the Platform or user profile menu. This page combines
**Agent subscriptions** (private to the signed-in user) with **Repository
connections** (shared across the platform). Existing `/app/repositories` links
open the repository section of this page.

Each user can save up to 50 named subscriptions, including multiple ChatGPT and
Claude accounts. Choose **Add subscription**, name the account, select its provider,
and import a current login file:

- ChatGPT / Codex: `$CODEX_HOME/auth.json`, normally `~/.codex/auth.json`, created
  by signing into Codex with ChatGPT.
- Claude: `~/.claude/.credentials.json`, where available, created by signing into
  Claude Code with a Claude subscription. The `claudeAiOauth` object must include
  `accessToken`, `expiresAt` (milliseconds), and the `user:inference` scope.
  Some operating systems store the login in a keychain rather than this file.

This is an expiring access-token import, not an automatic login/refresh flow.
The browser sends only the current access token and required account/expiry/scope
metadata, never refresh tokens or API keys. The server validates the import and
encrypts it with owner-and-record binding using a purpose-separated key derived
from the control secret. Retain that secret when restoring the database.
Credentials are never returned by the user-facing API or copied into the worker
filesystem. Expired accounts remain editable but cannot be loaded until refreshed.
Use **Edit** to rename an account or replace its login without changing its provider.
Replacing a login updates workers already using that saved subscription.

In **Workers → Subscription**, select an account independently for ChatGPT / Codex
and Claude. A worker can load one account per provider. The same saved subscription
can be used across multiple workers. Saving alone does not bind a worker.
Switching accounts changes only that provider's binding. Deleting a subscription
removes only that saved account and its bindings; unloading removes only the chosen
provider's binding on that worker. The gateway rechecks bindings during inference,
so switching, replacing, unloading, or deleting ends requests using the old login
at the next check (normally within five seconds; resolver timeouts can delay this).

Loading authorizes **all work on that worker** to use your account. This matters
especially for the shared reference workspace: projects and other operators can
use the loaded subscription. Another user cannot replace or unload your binding;
you must unload it first. This is not per-user isolation of work on a shared worker.

Hosted ownership comes from the verified OAuth2 Proxy or WorkOS session, never a
browser-supplied user ID. Local loopback mode uses one local-operator identity.
User-subscription endpoints are unavailable to project/MCP credentials.
Migration 14 preserves existing saved Codex credentials as a named default account,
with their existing worker bindings and encryption context intact.

Enable `subscriptions.user_accounts` and configure provider models and network
policy in inference-gateway. Set the gateway's `CONTROLPLANE_URL` and the
runtime controller's `CODEX_MODEL` and `CLAUDE_MODEL`. There is no API-billing
fallback.

Provision a random resolver credential of at least 32 characters as
`SUBSCRIPTION_GATEWAY_TOKEN_FILE` on inference-gateway and controlplane only.
It must differ from the runtime gateway, controller and admin tokens.
Controlplane also needs `GATEWAY_SERVICE_TOKEN_FILE` to verify the forwarded
inference grant with the controller. The resolver matches the verified workspace
and project to the stored worker before returning its credential. Roll out
controlplane and inference-gateway together after mounting the new token.

New imports use the controlplane-only encryption key when configured. Set
`CREDENTIAL_ENCRYPTION_KEY_FILE` for a single persistent key, or set
`CREDENTIAL_ENCRYPTION_KEYRING_FILE` with a JSON object containing `active`
and `keys` to retain old key IDs during rotation. Keep the legacy control token
available until untagged credentials have been re-imported. Compose provisions
a dedicated key on bootstrap.

New T3 consoles enable Claude when `CLAUDE_MODEL` is configured; existing explicit
provider settings are preserved, so enable the Claude provider in T3 if previously
disabled. Both native agents use the worker grant and provider-specific gateway
route. For retained Kubernetes workers, update the suspended Sandbox image/model
template before restarting; controller defaults do not rewrite existing templates.
No user login is bundled in deployment manifests.

## Legacy operator-managed credentials

Deployments without `subscriptions.user_accounts` can still use the provider CLI
and Vault procedure below. The former automatic setup/repair commands remain removed.

## Local development

Run `make bootstrap`, then `docker compose up -d --wait vault`. Use the Vault CLI
inside that container (`docker compose exec vault sh`) with:

```sh
export VAULT_ADDR=https://vault:8200
export VAULT_CACERT=/vault/tls/cert.pem
vault status
```

For a new Vault, initialize it once with `vault operator init`, securely retain the
recovery material, and unseal using `vault operator unseal`'s interactive prompt.
For an existing Vault, use its existing recovery material; never reinitialize it.
Authenticate using `vault login`'s interactive prompt. Do not put tokens or unseal
keys in command arguments, shell history, source control or application logs.

Enable a dedicated KV v2 mount named `subscription-auth` if it does not exist:

```sh
vault secrets enable -path=subscription-auth -version=2 kv
```

The gateway policy requires only these paths:

```hcl
path "subscription-auth/data/local-operator/codex" { capabilities = ["read"] }
path "subscription-auth/data/local-operator/claude" { capabilities = ["read"] }
```

Install that policy as `workbench-subscriptions` with `vault policy write`. Create
an orphan periodic token with that policy (`vault token create -orphan
-policy=workbench-subscriptions -period=24h`). Save its client token in
`.local/secrets/subscription_vault_token`. That file must be readable by the
container user; keep its parent directories mode 0700. Recovery/root credentials
must never be mounted into the gateway. Recreate only the gateway when replacing
its token; the gateway renews its own lease during normal operation.

Sign in using the native Codex or Claude CLI in your trusted operator environment.
Codex ChatGPT credentials are in `$CODEX_HOME/auth.json` (default `~/.codex/auth.json`);
Claude subscription credentials are in `$CLAUDE_CONFIG_DIR/.credentials.json`
(default `~/.claude/.credentials.json`). Copy only the current access token into
Vault, using a private JSON file with `vault kv put <path> @<file>`:

| Vault path | Fields |
| --- | --- |
| `subscription-auth/local-operator/codex` | `access_token`, `account_id`, `expires_at` |
| `subscription-auth/local-operator/claude` | `access_token`, `expires_at` |

`expires_at` is the Unix expiry in seconds. Codex requires ChatGPT auth and its
account binding; Claude's saved `expiresAt` is milliseconds and must be divided
by 1000. Never copy refresh tokens, API keys or entire login files. Reject missing
or expired access tokens. Remove the temporary import file after use.

Configure exact provider model IDs and public destination CIDRs in a private copy
of `config/inference.json`; see its `subscriptions` schema and
[the gateway configuration](../config/inference.json). Point
`INFERENCE_POLICY_FILE` at that copy and set `WORKSPACE_AGENT`, `CODEX_MODEL` and/or
`CLAUDE_MODEL` in `.env`. DNS changes require reviewing and updating destination
CIDRs. Apply configuration with:

```sh
DOCKER_SOCKET_GID="$(stat -c %g /var/run/docker.sock)" docker compose up -d --build --wait inference-gateway runtime-controller controlplane mcp
```

Create new workspaces to receive changed agent defaults. After recreating a gateway,
open each running workspace in the dashboard to reconcile its network attachment.
Existing workspace files and project state are retained.

## Kubernetes reference environment

Use the cluster's established Vault and secret provisioning procedures. The
Kustomize overlay consumes existing Secrets; it does not run local-development
bootstrap commands. See [the Kubernetes guide](k3s.md).

## Recovery

Check Vault seal state and the gateway token lease first. Unseal with the existing
recovery keys, or replace an expired gateway token using operator authority. Refresh
expired provider logins through the native CLI, then replace only the corresponding
Vault access-token record. There is no API-billing fallback. Recreate the gateway
only when its mounted Vault token changes, then reopen running workspaces.
