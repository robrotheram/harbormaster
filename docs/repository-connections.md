# Repository connections, issue board and reviews

Platform navigation includes **Connections** and **Network profiles**.
Project navigation includes **Issue board** and **Reviews**. Repository connections
are shared across the platform; existing project connections remain available.
Existing work conversations, workers and outputs remain available.

## Add an account in the UI

From the local operator console, open **Platform → Connections → Repositories → Add account**. Give the account a name, choose
GitHub, GitLab.com or GitLab self-hosted, and enter its access token. For a private
GitLab installation, enter its HTTPS address (including any installation subpath).
Select **Test & save account** to verify the credential with the provider before
saving. The account is selected automatically in the repository connection form.

Use **Update token** to replace an expired or revoked credential. The provider and
server address stay fixed; add a separate account for a different destination.
A failed authentication check leaves the previously saved credential intact.
Account setup and repository connections require local platform operator
authority. A browser sign-in (AuthKit or OAuth2 Proxy) grants a human identity.
Project administrators and project-scoped agent credentials cannot manage these
platform-wide connections. If the page requires local platform operator access,
ask a platform operator to perform the setup from the local console.

Tokens entered in the UI are encrypted with AES-256-GCM before persistence. The
key is purpose-separated from the persistent control-plane authentication secret,
which is mounted separately from the database. Account name, provider and API URL
are authenticated with the ciphertext. Tokens are never returned by the API or
saved to browser storage. Preserve the control-plane secret across deployments
and database restores. If you rotate that secret, re-enter each account token
through **Update token**; account listings remain available for recovery.

Verification uses the authenticated-user endpoints documented by
[GitHub](https://docs.github.com/en/rest/users/users) and
[GitLab](https://docs.gitlab.com/api/users/). It checks authentication, not permission
to every repository or to mutate tickets; those operations are checked separately.

## Optional operator-provisioned accounts

For deployments that manage credentials outside the UI, the control plane also
reads `WORKBENCH_FORGE_CONFIG`, an absolute path to an
operator-owned JSON secret file. Mount it read-only and make it readable only
by the control-plane user. Do not commit the file. Example contents (replace the
placeholder credentials locally):

```json
[
  {"name":"github-team","provider":"github","api_url":"https://api.github.com","token":"REPLACE"},
  {"name":"gitlab-cloud","provider":"gitlab","api_url":"https://gitlab.com/api/v4","token":"REPLACE"},
  {"name":"gitlab-office","provider":"gitlab","api_url":"https://gitlab.example.com/api/v4","token":"REPLACE"}
]
```

For Compose, put the real JSON under `.local/secrets/forge_accounts.json` and
use an override file:

```yaml
services:
  controlplane:
    environment:
      WORKBENCH_FORGE_CONFIG: /run/secrets/forge_accounts
    secrets:
      - forge_accounts
secrets:
  forge_accounts:
    file: ./.local/secrets/forge_accounts.json
```

Supply the override alongside `compose.yaml` when rebuilding/recreating the
control plane. Native deployments set the environment variable before startup.
The file is read on each operation, so rotating a token does not require changing
the saved repository connection. Removing its account disables remote operations;
existing imported work stays available.

GitHub fine-grained tokens need access to the selected repository and Issues
read permission to connect it. Mutations require Issues write permission.
GitLab project, group, and personal tokens need appropriate project access and
an API scope that allows reads (`read_api` or `api`). Writes require `api`.
Prefer dedicated accounts with access limited to the intended repositories. References:
[GitHub issues](https://docs.github.com/en/rest/issues/issues) and
[GitLab access token scopes](https://docs.gitlab.com/security/tokens/access_token_scopes/).

On-prem GitLab must be reachable from the control plane, use HTTPS, and present
a certificate trusted by its system CA store. TLS verification is never disabled.
Base URLs can include a GitLab installation subpath. Redirects are refused.
Credentials are bound to the configured API URL; updating a token cannot change
an existing account’s destination. Tokens are never returned by the account API or stored
in connection metadata, work instructions, or browser storage. Provider response
bodies are not reflected in errors. Connections and remote mutations currently
require the local administrator, not project-scoped agent credentials.

## Tickets and factory work

Open **Platform → Connections → Repositories**. Select an account, enter `owner/repository` (GitHub) or `group/subgroup/project`
(GitLab), and choose **Connect repository**. Access is checked before saving.
Select the connection to browse paginated tickets, view descriptions, labels and
milestones, create a ticket, or close/reopen one. GitHub pull requests are excluded.
**Open in provider** opens the provider's richer editing interface.

Choose a destination project before importing a ticket.
**Import to factory** creates a planned software work item, with the issue title,
source URL and description in its private workspace specification. It does not
start execution. Open the imported work to run it. Repeating the import of an
unchanged ticket on the same connection returns the same work item. Repeating an import preserves the existing work specification. Use board sync to
refresh title/description/status and continue the existing conversation with new requirements.
A disconnected/recreated connection has a new identity.

The issue board supports drag-and-drop planning, a hidden archive, highlighted
work problems, and explicit two-way title/description/status sync for imported issues.
See [Issue board](issue-board.md) for movement semantics and conflict handling.

## Review deliveries

Reviews provides a pending queue and completed/returned history. Select a delivery
to inspect its request, latest attempt's artifacts (including size and digest),
and attempt history. Open its title to inspect the complete conversation and
previews. Confirm inspection before accepting; **Request changes** returns it for
another attempt. Add revision guidance in the conversation. Each decision checks
the latest attempt ID and persists its disposition atomically with work state.

## Current boundaries

This is token-based ticket integration with explicit title/description/status sync and a local Kanban board. Native GitHub
Projects/GitLab board column sync, Scrum sprint planning, webhook/background sync,
OAuth installation, repository cloning credentials, PR/MR review and multi-user
reviewer identities/roles are not implemented here. Closing a provider ticket is
an explicit action; accepting factory work does not close or merge anything
remotely. Review feedback remains in the existing work conversation. Accounts need real credentials before live integration can be tested.
