+++
title = "Subscription credentials"
+++

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
encrypts it with owner-and-record binding. Retain the credential encryption key or keyring with database backups. Retain the original control token for imports encrypted with the legacy key.
Credentials are never returned by the user-facing API or copied into the worker
filesystem. Expired accounts remain editable but cannot be loaded until refreshed.
Use **Edit** to rename an account or replace its login without changing its provider.
Replacing a login updates workers already using that saved subscription.

In **Workers**, open the worker's **Settings**, then **Subscriptions**. Select an account for Codex, Claude, or both. A worker can load one account per provider. The same saved subscription
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

Add a `subscriptions.providers` policy to the inference gateway config. Set the
runtime controller's `CODEX_MODEL` and `CLAUDE_MODEL` to the approved model names
for the providers you enable.

Give the inference gateway and control plane the same random
`SUBSCRIPTION_GATEWAY_TOKEN` of at least 32 characters. Use a value distinct
from other service credentials and encryption keys. The control plane uses
the controller's gateway service token to verify the forwarded worker grant.
The resolver matches the verified workspace and project to the stored worker.
It returns credentials only through its HTTPS listener on port 8443.

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

## Configure Compose

Copy `deploy/config/inference.json` to a file under `.local/` and add the
providers you use. For example, this policy enables Codex and keeps fixture
inference available.

```json
{
  "models": ["fixture"],
  "subscriptions": {
    "providers": {
      "codex": {
        "models": ["gpt-6.1-sol"],
        "network": {"allowed_hosts": ["chatgpt.com"]}
      }
    }
  }
}
```

For Claude, add a `claude` provider with your approved model and
`api.anthropic.com` as the allowed host. Set `INFERENCE_POLICY_FILE` in `.env` to
the policy path. Set `INFERENCE_GATEWAY_MODE=subscription` if every task must
select a native agent.

Run `make up` to provision the resolver token and apply the configuration.
Then import your login in **Connections** and bind it in the worker's settings.
After a gateway is recreated, open each running worker in the dashboard to
reconcile its network attachment.

Bootstrap creates a local resolver certificate valid for one year. Before it
expires, move both `subscription_resolver_tls` and `subscription_resolver_ca`
out of `.local/secrets` to protected storage. Run `make bootstrap`, then
`docker compose up -d --no-deps --force-recreate --wait controlplane inference-gateway`.

## Configure Kubernetes

Set `GATEWAY_MODE=subscription` on the inference gateway and control plane for
a subscription-only deployment. Provision the resolver certificate, trust
bundle, and credentials before rolling out both services.
[Kubernetes subscription configuration](operations/kubernetes.md#configure-the-subscription-resolver)

## Replace an expired login

Sign in again through Codex or Claude Code. Open **Connections**, edit the
existing subscription, and import the new login. Workers already bound to that
subscription pick up the replacement automatically.
