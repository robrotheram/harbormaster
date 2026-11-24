const messages: Record<string, string> = {
  INVALID_DATASET:
    "DuckDB could not read this file. Check the CSV or Parquet format.",
  INVALID_DATASET_UPLOAD: "Choose a CSV or Parquet file of at most 1 GiB.",
  DATASET_TOO_LARGE:
    "The file exceeds the upload limit or remaining worker storage.",
  ANALYSIS_STORAGE_LIMIT: "The worker has reached its analysis storage limit.",
  ANALYSIS_UPLOAD_BUSY: "Another dataset upload is running on this worker.",
  ANALYSIS_QUERY_BUSY:
    "The worker is running another data query. Try again when it finishes.",
  ANALYSIS_QUERY_TIMEOUT:
    "The data query timed out. Try fewer columns or a smaller selection.",
  ANALYSIS_QUERY_FAILED:
    "The worker could not read these rows. Check the file and filter values.",
  ANALYSIS_ENGINE_UNAVAILABLE: "This worker image lacks the analysis engine.",
  ANALYSIS_UNKNOWN_FIELD: "A requested field is absent from this dataset.",
  ANALYSIS_SCHEMA_TOO_WIDE:
    "The dataset schema exceeds the worker's column or schema size limit.",
  ANALYSIS_DATA_UNAVAILABLE: "The source data is unavailable on this worker.",
  ANALYSIS_ARTIFACT_UNAVAILABLE:
    "The visualization is unavailable on this worker.",
  INVALID_VISUALIZATION_TITLE:
    "Enter a title of 1 to 200 characters on one line.",
  INVALID_VISUALIZATION_DESCRIPTION:
    "Enter a description of 2,000 characters or fewer.",
  INVALID_ANALYSIS_CURSOR:
    "The page cursor expired or the query changed. Return to the first page.",
  ANALYSIS_RESPONSE_TOO_LARGE:
    "The response exceeds the transfer limit. Try fewer columns.",
  ANALYSIS_ROW_TOO_WIDE:
    "A row exceeds the transfer limit. Choose fewer columns.",
  PROJECT_ISOLATED_WORKERS_REQUIRED:
    "This deployment uses a shared worker across projects. Ask the operator to configure project-isolated workers before running or opening private output with project membership.",
  PROJECT_ADMIN_REQUIRED: "A project administrator must make this change.",
  MEMBERSHIP_CONFLICT:
    "Membership changed, or this would remove the last active administrator. Refresh the member list before trying again.",
  AGENT_CONTROLLER_REQUIRED:
    "Only the current controller can instruct or stop this agent. Open Details to claim unassigned control or ask the controller to transfer it.",
  ASSIGNED_REVIEWER_REQUIRED:
    "Only the assigned reviewer can decide this output revision.",
  ACTIVE_MEMBER_REQUIRED: "Choose an active member of this project.",
  STOP_RUN_BEFORE_HANDOFF:
    "Stop the current run before transferring control or changing its reviewer.",
  CONTROLLER_OR_ADMIN_REQUIRED:
    "The controller or a project administrator must make this change.",
  HISTORY_EXPORT_INCOMPLETE:
    "The history export did not finish correctly. No file was downloaded. Check that the worker is running, then try exporting again.",
  EVIDENCE_UNAVAILABLE:
    "This saved file could not be read. Check the worker's status and try again.",
  EVIDENCE_READ_BUDGET_EXCEEDED:
    "This file is too large for the built-in reader. Inspect it in the worker instead.",
  RETAINED_PREVIEW_UNAVAILABLE:
    "The preview could not be prepared. Check the source and preview workers in Workers, then try again.",
  RETENTION_METADATA_UNAVAILABLE:
    "The preview may have started, but its details could not be saved. Check Workers before creating another preview.",
  INVALID_SERVICE_RESPONSE:
    "Workbench could not verify the preview's status. Refresh and check the preview worker before trying again.",
  PREVIEW_RELEASE_NOT_CONFIRMED:
    "Workbench could not confirm that preview access has ended. Refresh its status, revoke access if it is still active, then release the environment again.",
  WORK_DEPENDENCIES_PENDING:
    "Finish the required dependencies before running this task.",
  REVIEW_FEEDBACK_REQUIRED:
    "Describe what needs to change before sending this review.",
  WORK_STATE_CONFLICT:
    "This work changed since you opened it. Reopen the conversation or review to inspect the latest result before trying again.",
  WORKSPACE_UNAVAILABLE:
    "The worker is unavailable. Open Workers to check its status and start it if needed, then try again.",
  INSTRUCTION_UNAVAILABLE:
    "The original instructions could not be loaded. Check that the worker is running and the instruction file still exists.",
  ARTIFACT_UNAVAILABLE:
    "This output could not be loaded. Check the worker's status and try again. If it was not retained, inspect the original work conversation.",
  OUTPUT_NOT_RETAINED:
    "This output was not saved with the run. Open the conversation to inspect available files or ask the agent to generate it again.",
  TASK_CHANGES_UNAVAILABLE:
    "The worker could not load the file changes for this attempt.",
  PRIVATE_PUBLICATION_FAILED:
    "The instructions could not be saved to the worker. Check its status before trying again.",
  PRIVATE_PUBLICATION_UNCONFIRMED:
    "Workbench could not confirm that the instructions were saved. Check the conversation's current state before submitting again.",
  CONVERSATION_CONTEXT_UNAVAILABLE:
    "Workbench could not save the previous run’s context, so it did not start a follow-up. Check that the worker is running, then retry your message.",
  SPECIFICATION_ACCEPTANCE_FAILED:
    "The worker could not accept these instructions. Check that its runtime version matches the server, then retry this task.",
  SPECIFICATION_DIGEST_MISMATCH:
    "The instruction file changed after this work was created. Review the current file and submit a new work request with the intended instructions.",
  NETWORK_UNAVAILABLE:
    "Workbench could not reach the server. Check your connection and try again. If you submitted a change, check its current state before resubmitting.",
  REQUEST_TIMEOUT:
    "The server took too long to respond. Check the current state before trying again; your request may still have completed.",
  SESSION_REQUIRED:
    "Your session is unavailable or has expired. Reopen Workbench through your signed-in operator session, then try again.",
  SERVICE_UNAVAILABLE:
    "The service is temporarily unavailable. Try again shortly. If this continues, ask your administrator to check Platform → Platform services.",
  RATE_LIMITED: "Too many requests were sent. Wait a moment, then try again.",
  BOARD_REVISION_CONFLICT:
    "This task changed in another session. Overview has refreshed; review the latest details before retrying your change.",
  BOARD_SYNC_CONFLICT:
    "The repository issue changed since the last sync. Your edits have been kept. Open issue details to resolve the conflict.",
  BOARD_SYNC_LOCAL_CHANGES:
    "This issue has unsynced local edits. Sync them first, or choose Use repository version in issue details to discard them.",
  FORGE_CONNECTION_REMOVED:
    "The repository connection was removed. Reconnect the repository before syncing this issue.",
  FORGE_ACCOUNT_INVALID:
    "Check the account name, provider, HTTPS server address and access token.",
  FORGE_ACCOUNT_CONFLICT:
    "This account name is already in use or its settings changed. Refresh the accounts and retry; use a new name for a different server.",
  FORGE_CREDENTIAL_UNAVAILABLE:
    "The saved token could not be decrypted. Update the account token and retry.",
  STORAGE_UNAVAILABLE:
    "The server could not access its saved data. Try again shortly. If this continues, share the support details with your administrator so they can check the database.",
  FORGE_HOST_REQUIRED:
    "This provider account is unavailable. Ask the operator to check the server configuration.",
  FORGE_CONFIG_UNAVAILABLE:
    "The control plane cannot read its repository account configuration.",
  FORGE_CONFIG_INVALID:
    "The repository account configuration is invalid. Check the provider, HTTPS API URL and token.",
  FORGE_UNAVAILABLE:
    "The repository provider could not be reached. Check its address, network access and TLS certificate.",
  FORGE_HTTP_401:
    "The provider rejected this account credential. Rotate its token and retry.",
  FORGE_HTTP_403:
    "The provider denied access. Check token permissions and provider rate limits.",
  FORGE_HTTP_404:
    "The repository or ticket was not found, or this account cannot access it.",
  FORGE_HTTP_429: "The provider rate limit was reached. Wait before retrying.",
  FORGE_HTTP_302:
    "The provider redirected this request. Configure its canonical API address.",
  INVALID_REPOSITORY:
    "Enter owner/repository for GitHub or group/project for GitLab. Nested GitLab groups are supported.",
  DESKTOP_NOT_INSTALLED:
    "Desktop support is not installed in this worker image. Install the updated runtime before starting a desktop.",
  WORKSPACE_FILES_UNAVAILABLE: "Could not load files from this worker.",
  REPORT_CHANGED:
    "This report changed on the worker. Reopen it and apply your edits to the latest version.",
  REPORT_SAVE_FAILED: "Could not save the report on this worker.",
  INVALID_REPORT: "Use a Markdown report of up to 256 KiB.",
  WORKSPACE_FILE_TOO_LARGE: "Files larger than 1 MiB cannot be opened here.",
  APPLICATION_PORT_UNAVAILABLE:
    "The detected server is no longer listening. Start it again in the worker, then retry detection.",
  APPLICATION_PORT_BUSY:
    "An application already uses this port. Use its detected preview or choose a different port.",
  APPLICATION_LIMIT:
    "Four preview sessions are already active. Disconnect or stop one before opening another.",
  SCHEDULE_REVISION_CONFLICT:
    "This schedule changed in another session. Close this dialog, refresh the list and reopen it before saving.",
  SCHEDULE_HAS_UNFINISHED_RUNS:
    "This schedule has unfinished runs. Disable future triggers and let those runs finish before editing. Deleting the schedule cancels queued runs and preserves accepted work.",
  SCHEDULE_WORKSPACE_UNAVAILABLE:
    "This schedule’s worker is no longer available. Edit the schedule and select an existing worker before enabling or running it.",
  RESOURCE_LIMIT_UNAVAILABLE:
    "This worker requires resources unavailable in the current runtime template. Check the deployment configuration.",
  ASSISTANT_WORKSPACE_AMBIGUOUS:
    "This project has multiple Hermes workers. Open the worker you want from Workers.",
  HISTORY_UNAVAILABLE:
    "Recorded activity is unavailable. Try again. If this continues, check the worker’s status under Workers and share the support details with your administrator.",
  HANDOFF_HISTORY_UNAVAILABLE:
    "The latest request is missing from the central cache. Bring the original worker online to sync its history before moving the conversation.",
  RESULT_UNAVAILABLE:
    "The task’s final response could not be loaded. Check its recorded activity in the conversation and try again. If it remains unavailable, share the support details with your administrator.",
  HISTORY_WORKSPACE_STOPPED:
    "No cached history is available for this worker. Start the worker to load its saved history.",
  MATRIX_HOMESERVER_INVALID:
    "This address does not provide a valid Matrix homeserver. Enter your Element or Matrix HTTPS address and save the configuration to discover it.",
  MATRIX_TOKEN_INVALID:
    "The bot token was rejected. Paste just the token value from a valid bot session.",
  MATRIX_BOT_ID_MISMATCH:
    "This token belongs to a different Matrix user. Check the bot user ID and token.",
  MATRIX_BOT_OUTSIDE_ROOMS:
    "This bot has joined rooms outside this project. Use a dedicated bot account joined only to the configured rooms.",
  MATRIX_INVITE_REQUIRED:
    "Invite the bot to every configured room from your Matrix account, then retry saving the token. In Element: People → Invite. Invitations to a space alone are insufficient.",
  MATRIX_ACCEPT_INVITATIONS_REQUIRED:
    "The bot has pending invitations. Select Accept invitations to the configured rooms, then save the token again.",
  MATRIX_ROOM_ENCRYPTION_REQUIRED:
    "Every configured room must have end-to-end encryption enabled. Enable encryption in the room settings, then retry.",
  OPEN_NETWORK_FORBIDDEN_IN_SANDBOX:
    "Fully open networking is unavailable in this sandbox. Choose Blocked or an approved allowlist in Hermes setup or worker settings.",
  MATRIX_SETTINGS_REQUIRED:
    "Save the Matrix homeserver, bot identity, rooms and senders in Hermes setup first.",
  MATRIX_WORKSPACE_REQUIRED:
    "Start a Hermes worker for this project before connecting Matrix.",
  HERMES_WORKSPACE_REQUIRED:
    "Start a Hermes worker for this project before connecting Matrix.",
  MATRIX_CREDENTIAL_REJECTED:
    "Check the bot token and user ID. Use a dedicated bot joined only to the configured encrypted rooms.",
  MATRIX_RESTART_REQUIRED:
    "Stop and start the Hermes worker to apply the saved Matrix settings, then connect again.",
  MATRIX_ALREADY_CONNECTED:
    "This project's bot is already connected to another worker. Disconnect it there first.",
  MATRIX_UNAVAILABLE:
    "Matrix could not connect. Check the homeserver, bot device and encryption verification.",
  HERMES_SETUP_REQUIRED:
    "Open Hermes setup for this project, save a model, and check that its runtime is available before starting.",
  INVALID_HERMES_SETUP:
    "Check the model and Matrix settings. Use HTTPS for the homeserver and exact room and sender IDs.",
  INVALID_HERMES_CONNECTION_PROFILE:
    "Choose an existing blocked or allowlist connection profile. Fully open networking cannot be used for Hermes.",
  HERMES_REVISION_CONFLICT:
    "Hermes settings changed in another session. Reload this page before saving again.",
  SETUP_UNAVAILABLE:
    "Setup is unavailable on this worker. Ask an operator to update the worker image and runtime.",
  SETUP_BUSY: "This worker is preparing another setup change.",
  SETUP_REVIEW_PENDING:
    "Apply or discard the pending changes before starting another setup.",
  SETUP_UNSUPPORTED_CONFIG:
    "Setup supports standard mise tool declarations and skills locked to an exact GitHub commit.",
  SETUP_CHECKOUT_CHANGED:
    "The checkout changed after review. Prepare the change again.",
  SETUP_SKILL_CONFLICT:
    "This skill already exists or has local changes. Check the skill files before continuing.",
  SETUP_NETWORK_BLOCKED:
    "The worker’s connection policy blocked the download. Check the allowed destinations in Connection settings.",
  SETUP_DOWNLOAD_FAILED:
    "Couldn’t prepare the download. Check the worker’s connection settings and confirm the version or repository is available.",
  SETUP_STORAGE_FAILED:
    "The worker couldn’t save the setup change. Check available disk space and try again.",
  SETUP_INTERRUPTED: "Setup preparation was interrupted. Try the change again.",
  WORKSPACE_BUSY:
    "This worker has active work or terminal processes. Wait for them to finish before retrying, or choose another worker if available.",
  WORKER_SELECTION_REQUIRED:
    "Choose an existing worker or create one before starting this task.",
  WORKSPACE_UNSCHEDULABLE:
    "Kubernetes cannot schedule this worker. Free capacity by stopping an idle worker, or ask your platform administrator to check the cluster.",
  NOT_READY: "The worker is still starting. Check its state and retry shortly.",
  QUOTA_EXCEEDED:
    "The running worker quota is full. Stop an idle worker and retry.",
  PROJECT_NAMESPACE_UNAVAILABLE:
    "The deployment namespace could not be prepared. Retry the operation to complete setup.",
  DEFAULT_PROJECT_DELETE_UNSUPPORTED:
    "The default project owns the shared worker in this deployment.",
  SHARED_WORKSPACE_ONLY:
    "This environment uses one shared workspace. Open the existing worker; creating, branching or deleting workers is disabled.",
  PROJECT_MISMATCH:
    "This action does not match the selected project. Reopen the intended project and check the worker selection before trying again.",
  CROSS_PROJECT_DENIED: "Content cannot be transferred between projects.",
  CROSS_PROJECT_TRANSFER_DENIED:
    "Content cannot be transferred between projects.",
  DESTINATION_EXISTS:
    "The destination file already exists. Choose a new destination path.",
  WORKSPACES_NOT_READY: "Start both workers before copying a file.",
  FILE_UNAVAILABLE:
    "The source file is unavailable or exceeds the file-size limit.",
  FILE_HANDOFF_FAILED:
    "The file could not be copied. Check both workers and the destination path.",
  INVALID_PROJECT_FILE:
    "Use a relative project-file path. Credential and Git metadata paths cannot be shared.",
  PAIRING_UNAVAILABLE:
    "T3 could not issue a pairing link. Check worker readiness and retry.",
  CONSOLE_UPGRADE_REQUIRED:
    "This worker uses the task-only runtime. Create a new T3 worker to use the browser.",
  CONSOLE_UNAVAILABLE:
    "The worker connection is unavailable. Refresh its state and retry.",
  CONNECTION_PROFILE_IN_USE:
    "This profile is still used by a worker or Hermes setup. Choose another profile in those settings before deleting it.",
  PROFILE_REVISION_CONFLICT:
    "This profile changed in another window. Refresh the list and reopen it before trying again.",
  INVALID_CONNECTION_PROFILE:
    "This connection profile is unavailable. Select another profile.",
  STOP_WORKSPACE_FIRST:
    "The worker must stop before this action can finish. Finish active work and close terminals first, then stop the worker in Workers and try again.",
  WORKSPACE_CHANGED:
    "A worker changed while this action was running. Refresh Workers and check its current state before trying again.",
  NETWORK_POLICY_UNAVAILABLE:
    "This runtime cannot apply the requested network policy.",
  RUNTIME_CAPABILITIES_INCOMPATIBLE:
    "The runtime controller uses an incompatible capability format. Update the host controller to match the control plane, then check again.",
  DOCKER_ADDRESS_POOL_EXHAUSTED:
    "Docker has no free bridge subnets for a new worker. Ask an operator to free unused Docker networks or expand the address pools, then retry.",
  RUNTIME_UNAVAILABLE:
    "The runtime could not complete this request. Use the worker startup check or check Platform services before retrying.",
  SUBSCRIPTION_LIMIT_REACHED:
    "You have reached the limit of 50 saved subscriptions. Delete an unused account before adding another.",
  SUBSCRIPTION_INVALID:
    "This login is invalid or expired for the selected provider. Sign in again and import a fresh ChatGPT or Claude login file.",
  SUBSCRIPTION_IN_USE:
    "Another user has a subscription loaded for this provider in this worker. Its owner must unload it before you can load yours.",
  SUBSCRIPTION_NOT_CONFIGURED:
    "This deployment cannot load saved subscriptions yet. Ask an administrator to enable personal subscriptions on the inference gateway and control plane.",
  MODEL_NOT_ALLOWED:
    "This account cannot use the selected model. Refresh the model list and choose an available model.",
  PROVIDER_LOGIN_REQUIRED:
    "This worker needs a valid subscription for the selected provider. Load an account in Worker settings → Subscriptions. If the login expired, replace it in Connections, then retry.",
  AGENT_SELECTION_REQUIRED:
    "Choose a model in the message composer before sending. If the model list is empty, check Worker settings → Subscriptions.",
  INFERENCE_REJECTED:
    "The inference gateway rejected this run. Check that the worker has a loaded subscription and that you selected an agent and model before retrying.",
  PROVIDER_NOT_CONFIGURED:
    "Codex or the selected provider is not configured for this worker. Ask an administrator to configure its model and subscription connection before retrying. Worker readiness only confirms that compute is running.",
  AGENT_EXECUTABLE_UNAVAILABLE:
    "The native agent could not start. Ask an administrator to check the worker image and installed provider executable.",
  NATIVE_AGENT_FAILED:
    "The native agent stopped before completing this task. Check worker subscriptions and the selected model before retrying.",
  INVALID_AGENT_RESPONSE:
    "The native agent returned an unreadable result. Check this worker’s activity logs before retrying.",
  CREDENTIAL_UNAVAILABLE:
    "The service could not read or use a saved credential. Ask an administrator to check the credential encryption configuration and, for deployments that use Vault, its connection.",
  IDEMPOTENCY_CONFLICT:
    "A previous request used this key with different settings. Change the form and submit a new request.",
  NOT_FOUND_OR_FORBIDDEN:
    "This resource is unavailable in the selected project.",
  FORBIDDEN: "This action is not permitted for the selected project.",
  TERMINAL_UNAVAILABLE:
    "The terminal is unavailable. Update the worker runtime and controller, then restart the worker.",
  TERMINAL_EXPIRED:
    "This terminal session has ended. Open a new shell to reconnect.",
  TERMINAL_LIMIT:
    "This worker already has four terminal sessions. Close one before opening another.",
  INVALID_REQUEST: "Check the form fields and try again.",
};

export class APIError extends Error {
  constructor(
    public code: string,
    public status: number,
  ) {
    super(
      messages[code] ||
        (status === 401
          ? messages.SESSION_REQUIRED
          : status === 403
            ? messages.FORBIDDEN
            : status === 429
              ? messages.RATE_LIMITED
              : status >= 500
                ? messages.SERVICE_UNAVAILABLE
                : "The request could not finish. Check the current state before trying again. If this continues, share the support details with your administrator."),
    );
  }
}

export async function request<T>(
  path: string,
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  return send<T>(
    path,
    body === undefined
      ? { method: "GET" }
      : {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "X-Workbench-Request": "1",
          },
          body: JSON.stringify(body),
        },
    body === undefined ? 15000 : 45000,
    signal,
  );
}

export function uploadFile<T>(path: string, file: File, signal: AbortSignal) {
  return send<T>(
    path,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/octet-stream",
        "X-Workbench-Request": "1",
      },
      body: file,
    },
    600_000,
    signal,
  );
}

async function send<T>(
  path: string,
  init: RequestInit,
  timeoutMs: number,
  signal?: AbortSignal,
): Promise<T> {
  const timeout = AbortSignal.timeout(timeoutMs);
  let response: Response;
  try {
    response = await fetch("/ui-api" + path, {
      ...init,
      cache: "no-store",
      redirect: "error",
      signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
    });
  } catch (error) {
    if (signal?.aborted) throw error;
    throw new APIError(
      timeout.aborted ? "REQUEST_TIMEOUT" : "NETWORK_UNAVAILABLE",
      0,
    );
  }
  let value: unknown;
  try {
    value = await response.json();
  } catch {
    throw new APIError("UNAVAILABLE", response.status);
  }
  if (
    response.status === 401 &&
    (value as { code?: string })?.code === "AUTH_REQUIRED" &&
    typeof window !== "undefined"
  ) {
    window.dispatchEvent(new Event("workbench:auth-required"));
  }
  if (!response.ok)
    throw new APIError(
      (value as { code?: string })?.code || "UNAVAILABLE",
      response.status,
    );
  return value as T;
}

// Detect a broken boundary rather than render another project's cached/server data.
export function requireProject<T extends { project_id: string }>(
  items: T[],
  project: string,
): T[] {
  if (items.some((item) => item.project_id !== project))
    throw new APIError("PROJECT_MISMATCH", 403);
  return items;
}

export async function projectList<T extends { project_id: string }>(
  path: string,
  project: string,
  signal?: AbortSignal,
) {
  const value = await request<{ items: T[] }>(
    path + "?project_id=" + encodeURIComponent(project),
    undefined,
    signal,
  );
  return requireProject(value.items, project);
}
