const messages: Record<string, string> = {
  RESOURCE_LIMIT_UNAVAILABLE:
    "This workspace requires resources unavailable in the current runtime template. Check the deployment configuration.",
  ASSISTANT_WORKSPACE_AMBIGUOUS:
    "This project has multiple Hermes workspaces. Open the workspace you want from Workspaces.",
  HISTORY_UNAVAILABLE:
    "Private history is not available yet. The workspace may need to restart into the history-enabled image.",
  HISTORY_WORKSPACE_STOPPED:
    "Private history remains on the workspace disk. Start the workspace to browse or export it.",
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
    "Fully open networking is unavailable in this sandbox. Choose Blocked or an approved allowlist in Hermes setup or workspace settings.",
  MATRIX_SETTINGS_REQUIRED:
    "Save the Matrix homeserver, bot identity, rooms and senders in Hermes setup first.",
  MATRIX_WORKSPACE_REQUIRED:
    "Start a Hermes workspace for this project before connecting Matrix.",
  HERMES_WORKSPACE_REQUIRED:
    "Start a Hermes workspace for this project before connecting Matrix.",
  MATRIX_CREDENTIAL_REJECTED:
    "Check the bot token and user ID. Use a dedicated bot joined only to the configured encrypted rooms.",
  MATRIX_RESTART_REQUIRED:
    "Stop and start the Hermes workspace to apply the saved Matrix settings, then connect again.",
  MATRIX_ALREADY_CONNECTED:
    "This project's bot is already connected to another workspace. Disconnect it there first.",
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
  WORKSPACE_BUSY:
    "This workspace has active work or terminal processes. Finish them in T3 before retrying.",
  NOT_READY:
    "The workspace is still starting. Check its state and retry shortly.",
  QUOTA_EXCEEDED:
    "The running workspace quota is full. Stop an idle workspace and retry.",
  PROJECT_MISMATCH: "Both workspaces must belong to this project.",
  CROSS_PROJECT_DENIED: "Content cannot be transferred between projects.",
  CROSS_PROJECT_TRANSFER_DENIED:
    "Content cannot be transferred between projects.",
  DESTINATION_EXISTS:
    "The destination file already exists. Choose a new destination path.",
  WORKSPACES_NOT_READY: "Start both workspaces before copying a file.",
  FILE_UNAVAILABLE:
    "The source file is unavailable or exceeds the file-size limit.",
  FILE_HANDOFF_FAILED:
    "The file could not be copied. Check both workspaces and the destination path.",
  INVALID_PROJECT_FILE:
    "Use a relative project-file path. Credential and Git metadata paths cannot be shared.",
  PAIRING_UNAVAILABLE:
    "T3 could not issue a pairing link. Check workspace readiness and retry.",
  CONSOLE_UPGRADE_REQUIRED:
    "This workspace uses the task-only runtime. Create a new T3 workspace to use the browser.",
  CONSOLE_UNAVAILABLE:
    "The workspace connection is unavailable. Refresh its state and retry.",
  PROFILE_REVISION_CONFLICT:
    "This profile changed in another window. Refresh the list and reopen it before saving.",
  INVALID_CONNECTION_PROFILE:
    "This connection profile is unavailable. Select another profile.",
  STOP_WORKSPACE_FIRST:
    "Stop the workspace before changing this connection profile. Finish active work and close terminals first.",
  NETWORK_POLICY_UNAVAILABLE:
    "This runtime cannot apply the requested network policy.",
  RUNTIME_UNAVAILABLE:
    "The runtime is unavailable. Check the Runtime overview before retrying.",
  PROVIDER_LOGIN_REQUIRED:
    "The native provider needs its operator login refreshed. No API billing fallback was used.",
  CREDENTIAL_UNAVAILABLE:
    "The provider credential is unavailable. Run the credential repair command in the project terminal.",
  IDEMPOTENCY_CONFLICT:
    "A previous request used this key with different settings. Change the form and submit a new request.",
  NOT_FOUND_OR_FORBIDDEN:
    "This resource is unavailable in the selected project.",
  FORBIDDEN: "This action is not permitted for the selected project.",
  INVALID_REQUEST: "Check the form fields and try again.",
};

export class APIError extends Error {
  constructor(
    public code: string,
    public status: number,
  ) {
    super(
      messages[code] ||
        "The request could not finish. Refresh to check the latest state.",
    );
  }
}

export async function request<T>(
  path: string,
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const timeout = AbortSignal.timeout(body === undefined ? 15000 : 45000);
  const response = await fetch("/ui-api" + path, {
    method: body === undefined ? "GET" : "POST",
    headers:
      body === undefined
        ? {}
        : { "Content-Type": "application/json", "X-Workbench-Request": "1" },
    body: body === undefined ? undefined : JSON.stringify(body),
    cache: "no-store",
    redirect: "error",
    signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
  });
  let value: unknown;
  try {
    value = await response.json();
  } catch {
    throw new APIError("UNAVAILABLE", response.status);
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
