# Development with Codex and Claude subscriptions

The native Codex or Claude Code CLI runs **inside the workspace container** and edits its persistent `/data/files` directory. MCP submits and follows its tasks. Both agents are baked into the workspace image alongside Git, Bash, Python, Node and ripgrep; no runtime package downloads are needed for the included Python example.

Subscription authentication is separate from API-key billing. The gateway forwards each native CLI's protocol to its fixed provider endpoint. It does not translate subscription credentials into an OpenAI-compatible Chat Completions service.

## Credential boundary

Vault KV v2 holds an expiring access-token snapshot under `subscription-auth/local-operator/codex` or `subscription-auth/local-operator/claude`. The inference gateway alone receives a read-only Vault token for those two paths. It connects to Vault over TLS using a locally generated, pinned certificate.

Workspaces receive generation-bound platform grants. The gateway validates the grant, binds its verified principal to a provider allowlist, checks the model and current destination IPs, and injects the real subscription token. No provider access token, refresh token, Vault token, host home directory or host CLI configuration is mounted into the workspace. Native command processes can read their workspace grant; it is authority to make approved inference calls while that workspace is running.

The gateway exposes only native Codex Responses/compaction and Claude Messages/token-count operations. It rejects arbitrary paths and destinations, redirects, unknown models and expired credentials. Native inference responses stream directly to the workspace. Provider error bodies and credentials do not reach platform logs, metadata or the dashboard.

Ordinary Docker isolation and complete workspace egress enforcement remain unverified. These native CLI integrations do not change that status.

## Set up the local deployment

Requires Docker/Compose, Python 3 and OpenSSL. First build/start the current version:

```sh
make up
python3 scripts/subscription_auth.py init-vault
```

`init-vault` initializes an empty **local development** Vault, or unseals this deployment using its existing recovery file. It creates the dedicated KV v2 mount and read-only gateway policy. It never resets an existing Vault. Root-token and single-share unseal recovery material are kept in `.local/vault-init.json`, mode 0600, under a mode-0700 directory. Keep a separate protected backup. Storing recovery material on this host is a local-development convenience, not production key custody.

After a Vault restart, rerun `init-vault` to unseal it. Do not delete its persistent volume or TLS key/certificate. The gateway renews its own 24-hour periodic Vault token at startup and every ten minutes, including while no agents are making requests. If the gateway has been offline long enough for that lease to expire, `init-vault` detects the expired token and provisions a replacement. The gateway must then be recreated to load it.

### Codex

Sign in using ChatGPT with the normal operator CLI, then sync its access token:

```sh
codex login
python3 scripts/subscription_auth.py sync codex
python3 scripts/subscription_auth.py configure --default codex --codex-model gpt-6.1-sol
docker compose up -d --build --wait inference-gateway runtime-controller controlplane mcp
```

If Codex is already signed in, skip `codex login`. API-key logins are rejected by the sync command. `--auth-file PATH` supports a separate native CLI auth cache. Host auth files are read privately by the operator script; it copies only the access token, expiry and account binding to Vault. No refresh token is copied or refreshed by the gateway, so it cannot rotate or invalidate the host CLI's refresh token.

### Claude Code

Use the official native Claude Code subscription login first:

```sh
claude auth login
python3 scripts/subscription_auth.py sync claude
```

If the host Claude launcher is unavailable, use the pinned CLI in a trusted operator-only login container from an interactive terminal:

```sh
python3 scripts/subscription_auth.py login claude
python3 scripts/subscription_auth.py sync claude
```

Complete the provider's browser/code sign-in in that terminal. The private operator cache stays under `.local/native-logins/claude`, outside workspaces. No host source or Docker socket is mounted in that login container.

Then rerun `configure` with **both exact model IDs** so the allowlists include both providers:

```sh
python3 scripts/subscription_auth.py configure --default codex \
  --codex-model gpt-6.1-sol --claude-model YOUR_CLAUDE_MODEL_ID
docker compose up -d --build --wait inference-gateway runtime-controller controlplane mcp
```

Use an exact Claude model ID available to your account, not the literal placeholder or a `sonnet`/`opus` alias. `--default claude` selects Claude for submissions that omit `provider`. No Claude API key is requested or configured. Availability and subscription limits remain controlled by the provider.

Configuration is written under ignored `.local/` and `.env`; the checked-in fixture policy stays unchanged. Configuration pins the current DNS addresses as individual allowed CIDRs. A provider DNS change fails closed: rerun `configure` to explicitly refresh that allowlist. This does not broaden it to arbitrary public addresses.

## Run real development through MCP

Create a **new** workspace after enabling native defaults; existing workspace containers and storage are preserved. The updated image is used when new containers are created.

Ask the connected T3 agent:

> Create an Agent Workbench workspace with key `my-development-workspace`. Submit a task using provider `codex` to create a Python add function and unittest tests, run the tests, and report the result. Follow its task status and result. Use the returned IDs.

Use `provider: "claude"` for Claude tasks once its login and model are configured. Either agent can work on the same workspace files; tasks are serialized. Changing provider with a reused task idempotency key is a conflict. Use a new key for new work.

The same operation from the terminal:

```sh
python3 scripts/client.py create --key my-development-workspace
python3 scripts/client.py submit WORKSPACE_ID \
  'Create arithmetic.py with add(a, b), add unittest tests, and run python3 -m unittest -v.' \
  --provider codex --key python-development-v1
python3 scripts/client.py status WORKSPACE_ID TASK_ID
python3 scripts/client.py result WORKSPACE_ID TASK_ID
```

Tasks continue after MCP disconnects, stop on confirmed cancellation, and have a ten-minute overall limit. Their native subprocess group is terminated when execution ends. The native CLIs use their file/command tools within the existing container boundary; the older thirty-second command-tool limit applies to the built-in Chat Completions runner, not native CLI commands. Background processes are unsupported and tracked processes block idle-stop. Files persist across tasks and workspace stop/start. Native runs currently start fresh conversations; recorded task results are durable, but CLI conversation resume is not implemented.

The runtime overview remains read-only. The [workspace launcher](interactive-workspaces.md) can manage projects and open native T3 chat, files and terminals. Private repositories, dependency downloads and preview servers require additional explicit import/network work; no host repository is implicitly mounted or copied.

## Expiry and troubleshooting

For `503 CREDENTIAL_UNAVAILABLE`, recover the local Vault lease, sync the existing Codex login and recreate the gateway with one command:

```sh
python3 scripts/subscription_auth.py repair
```

This uses the existing local recovery material, preserves Vault and workspace volumes, and reconciles gateway network attachments for already-running workspaces. It does not perform a new provider login or change the inference destination/model allowlist. If the host subscription login has expired, refresh it with the normal Codex CLI, then rerun repair. OpenAI documents automatic token refresh during normal signed-in Codex use; the gateway stores only an access-token snapshot and cannot refresh it independently. [OpenAI authentication](https://learn.chatgpt.com/docs/auth)

`CREDENTIAL_UNAVAILABLE` indicates Vault access or lease renewal failed; it does not establish that the ChatGPT subscription expired. `PROVIDER_LOGIN_REQUIRED` indicates a missing, expired or rejected provider credential. No provider body or token is included in those errors.

- Subscription access tokens expire. Refresh the normal native CLI login, then rerun `sync codex` or `sync claude`. Do not paste tokens into chat. Vault is the workspace system's credential store; the native operator login retains its own cache.
- A missing/expired Vault secret or a sealed Vault fails closed. No API billing or synthetic fallback is used for a native task.
- `AGENT_EXECUTABLE_UNAVAILABLE` means a workspace was created from an older image. Build the workspace image and create a new workspace; its existing storage is not deleted automatically.
- `NATIVE_AGENT_FAILED` means the CLI exited unsuccessfully. Check credential expiry, the configured model, the gateway process health and the destination allowlist. Raw provider errors are deliberately kept out of platform responses.
- Existing stopped containers retain their original defaults. Pass an explicit provider only to containers with the native runner/image; new workspaces are the supported upgrade path for this MVP.

Reference: [OpenAI authentication](https://learn.chatgpt.com/docs/auth), [Codex noninteractive execution](https://learn.chatgpt.com/docs/non-interactive-mode), [Claude Code authentication](https://code.claude.com/docs/en/authentication), [Claude native gateway protocol](https://code.claude.com/docs/en/llm-gateway-protocol).
