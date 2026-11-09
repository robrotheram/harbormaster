#!/usr/bin/env python3
"""Provision local application credentials; never print their contents."""
import os
from pathlib import Path
import secrets
import shutil
import subprocess

root = Path(__file__).resolve().parent.parent
local = root / ".local"
local.mkdir(mode=0o700, exist_ok=True)
os.chmod(local, 0o700)
directory = local / "secrets"
directory.mkdir(mode=0o700, exist_ok=True)
os.chmod(directory, 0o700)

def provision(name, value=None):
    path = directory / name
    if not path.exists():
        # Parent directories protect host access. Each file is mounted only into
        # its authorized container; readable by the non-root application UID.
        with path.open("x") as stream:
            stream.write(value if value is not None else secrets.token_hex(32))
        path.chmod(0o644)
    return path.read_text().strip()

for name in ("control_token", "controller_token", "gateway_service_token", "grant_secret", "mcp_token"):
    provision(name)
password = provision("postgres_password")
provision("database_url", f"postgres://workbench:{password}@postgres:5432/workbench?sslmode=disable")
# An empty file mounts before Vault provisioning. It conveys no Vault authority;
# fixture mode never reads it. init-vault replaces it with a read-only token.
pending = directory / "subscription_vault_token"
if not pending.exists():
    pending.write_text("")
    pending.chmod(0o644)
tls_dir = local / "vault-tls"
tls_dir.mkdir(mode=0o700, exist_ok=True)
certificate = tls_dir / "cert.pem"
key = directory / "vault_tls_key"
if not certificate.exists() and not key.exists():
    subprocess.run(["openssl", "req", "-x509", "-newkey", "rsa:3072", "-nodes",
                    "-days", "365", "-subj", "/CN=vault",
                    "-addext", "subjectAltName=DNS:vault,DNS:localhost,IP:127.0.0.1",
                    "-keyout", str(key), "-out", str(certificate)],
                   check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    key.chmod(0o644)
elif not certificate.exists() or not key.exists():
    raise SystemExit("Incomplete Vault TLS material; restore the existing certificate/key pair.")
if not (root / ".env").exists():
    shutil.copyfile(root / ".env.example", root / ".env")
print("Local credentials provisioned in .local/secrets; existing credentials preserved.")
