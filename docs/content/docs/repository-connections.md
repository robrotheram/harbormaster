+++
title = "Connect a repository and import issues"
+++

Repository connections are shared across the platform. Use the local operator console to manage provider accounts and connections. Use the project's **Overview** to manage imported tasks.

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

The control plane encrypts account tokens before saving them. Back up its credential encryption key or keyring with the database. Retain the original control token for records encrypted with the legacy key. If a token can no longer be decrypted, re-enter it through **Update token**.

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
**Import to factory** creates a planned software task with the issue title,
description, and source URL. Importing needs no worker capacity. **Start work**
uses an eligible worker and publishes the task as instructions for the agent. Select a worker if the task has none assigned.
Repeating an import returns the same task and preserves local edits. Use board
sync to refresh the title, description, or status from the repository.
A disconnected/recreated connection has a new identity.

You can also import selected issues from **Overview**. Use the task details to sync or refresh imported fields. See [project task sync](issue-board.md#import-and-sync-repository-issues) for conflict handling.

## Review imported work

Open the local task from **Overview**. Start work when the task is ready, then [review the result in Workspace](reviews.md). A review changes the local work state. Update the provider issue through its explicit sync action.

The connection supports token-based issue access and explicit sync of title, description, and open or closed state. Git hosting project boards and webhook sync are outside that scope. Set up worker Git credentials separately if you need to clone a private repository.
