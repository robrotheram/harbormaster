# Subscription credentials

## Personal Codex subscriptions

In Workers, choose **Subscription** on a worker. Import `auth.json` from a Codex
ChatGPT login, then choose **Load into worker**. Each dashboard user has one saved
subscription, reusable across workers. Saving alone does not bind any worker.
The browser sends only the access token and account ID, never refresh tokens or
API keys. The server checks the account binding and expiry and encrypts the saved
credential with an owner-bound, purpose-separated key derived from the control
secret. Retain that secret when restoring the database; changing it makes existing
saved subscriptions unreadable.

This is an expiring access-token import, not an automatic login/refresh flow.
The dialog shows expiry; sign in again using Codex and import a current login when
needed. Credentials are never returned by the user-facing API or copied into the
worker filesystem. Delete removes the saved credential and all worker bindings.
Unload removes only that worker binding. The gateway rechecks bindings during
inference, so unloading/deleting ends active requests at the next check (normally
within five seconds; resolver timeouts can delay this).

Loading authorizes **all work on that worker** to use your account. This matters
especially for the shared reference workspace: projects and other operators can
use the loaded subscription. Another user cannot replace or unload your binding;
you must unload it first. This is not per-user isolation of work on a shared worker.

Hosted ownership comes from the verified OAuth2 Proxy or WorkOS session, never a
browser-supplied user ID. Local loopback mode uses one local-operator identity.
User-subscription endpoints are unavailable to project/MCP credentials.

The reference overlay enables `subscriptions.user_accounts` on the inference
gateway and uses the `gpt-5.4` Codex model. Its dedicated internal resolver uses the
gateway service credential, mounted read-only into controlplane through
`SUBSCRIPTION_GATEWAY_TOKEN_FILE`; the normal control credential cannot call it.
The gateway's policy still bounds allowed model IDs and destination CIDRs. Review
`user-subscriptions.yaml` when provider DNS changes. No API-billing fallback exists.

For other deployments, configure the gateway's `CONTROLPLANE_URL`, enable
`subscriptions.user_accounts`, provide a Codex provider policy, set `CODEX_MODEL`
on the runtime controller, and supply the same gateway service token through
`SUBSCRIPTION_GATEWAY_TOKEN_FILE` on controlplane. For retained Kubernetes workers,
update the suspended Sandbox image/model template before restarting; controller
defaults do not rewrite existing templates. No user login is bundled in deployment
manifests.

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
docker compose up -d --build --wait inference-gateway runtime-controller controlplane mcp
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
