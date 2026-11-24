import { WorkerSubscriptions } from "./subscription";
import { workspacesApi } from "./api";
import {
  parseDestinations,
  isGitHubOnly,
  connectionChangeNeedsStop,
} from "@/features/connection-profiles/policy";
import { useState } from "react";
import { useMutation } from "@tanstack/react-query";

import { ErrorNotice, Form, Modal } from "@/shared/components";
import { Button } from "@/components/ui/button";

import { Textarea } from "@/components/ui/textarea";
import { NativeSelect } from "@/components/ui/native-select";
import { useConnectionProfiles } from "@/features/connection-profiles/api";
import type { Workspace, ConnectionPolicy } from "@/shared/types";
export function WorkspaceSettings({
  workspace,
  onClose,
  onSaved,
  initialSection = "worker",
}: {
  workspace: Workspace;
  onClose?: () => void;
  onSaved: (updated: Workspace) => void;
  initialSection?: "worker" | "subscriptions";
}) {
  const [section, setSection] = useState(initialSection);
  const globals = useConnectionProfiles();
  const [globalID, setGlobalID] = useState(
    workspace.connection_profile_id || "",
  );
  const [idle, setIdle] = useState(workspace.idle_timeout_minutes ?? 60);
  const original = workspace.connections;
  const [preset, setPreset] = useState(
    original?.mode === "open"
      ? "open"
      : original?.mode === "allowlist"
        ? isGitHubOnly(original)
          ? "github"
          : "custom"
        : "blocked",
  );
  const [destinations, setDestinations] = useState(
    original?.destinations.join("\n") ||
      "github.com:443\nregistry.npmjs.org:443",
  );
  const localConnections: ConnectionPolicy = {
    mode:
      preset === "open"
        ? "open"
        : preset === "blocked"
          ? "blocked"
          : "allowlist",
    destinations:
      preset === "github"
        ? ["github.com:443"]
        : preset === "custom"
          ? parseDestinations(destinations)
          : [],
  };
  const globalProfile = globals.data?.find((p) => p.id === globalID);
  const connections = globalProfile?.policy || localConnections;
  const needsStop = connectionChangeNeedsStop(workspace, connections);
  const mutation = useMutation({
    mutationFn: () =>
      workspacesApi.settings(workspace.id, {
        idle_timeout_minutes: idle,
        ...(globalID ? { connection_profile_id: globalID } : { connections }),
      }),
    onSuccess: onSaved,
  });
  const hasEdits =
    idle !== (workspace.idle_timeout_minutes ?? 60) ||
    globalID !== (workspace.connection_profile_id || "") ||
    (!globalID &&
      (localConnections.mode !== (original?.mode ?? "blocked") ||
        localConnections.destinations.join("\n") !==
          (original?.destinations ?? []).join("\n")));
  const profileUpdate =
    !!globalProfile &&
    globalProfile.revision > (workspace.connection_profile_revision ?? 0);
  const canSave =
    !mutation.isPending &&
    !needsStop &&
    (!globalID || (globals.isSuccess && !!globalProfile));
  const content = (
    <>
      <div
        className="settings-tabs mb-4"
        role="group"
        aria-label="Worker settings sections"
      >
        <button
          type="button"
          aria-pressed={section === "worker"}
          onClick={() => setSection("worker")}
        >
          General
        </button>
        <button
          type="button"
          aria-pressed={section === "subscriptions"}
          onClick={() => setSection("subscriptions")}
        >
          Subscriptions
        </button>
      </div>
      {section === "subscriptions" ? (
        <WorkerSubscriptions workspace={workspace} />
      ) : (
        <Form
          onSubmit={() => {
            if (canSave) mutation.mutate();
          }}
        >
          <fieldset disabled={mutation.isPending} className="min-w-0 space-y-5">
            {onClose && <p>{workspace.name || workspace.id}</p>}
            <section
              className="settings-panel"
              aria-labelledby="worker-idle-title"
            >
              <h2 id="worker-idle-title" className="settings-panel-title">
                Idle shutdown
              </h2>
              <label>
                Automatic idle shutdown
                <NativeSelect
                  aria-label="Automatic idle shutdown"
                  value={idle}
                  onChange={(e) => setIdle(Number(e.target.value))}
                >
                  {[
                    0,
                    15,
                    30,
                    60,
                    120,
                    480,
                    1440,
                    ...(![0, 15, 30, 60, 120, 480, 1440].includes(idle)
                      ? [idle]
                      : []),
                  ].map((v) => (
                    <option key={v} value={v}>
                      {v === 0 ? "Never automatically stop" : v + " minutes"}
                    </option>
                  ))}
                </NativeSelect>
              </label>
              <p className="form-note">
                The timer resets when you open the worker. Active work keeps it
                running.
              </p>
            </section>
            <section
              className="settings-panel"
              aria-labelledby="worker-network-title"
            >
              <h2 id="worker-network-title" className="settings-panel-title">
                Network access
              </h2>
              <label>
                Global connection profile
                <NativeSelect
                  aria-label="Global connection profile"
                  value={globalID}
                  disabled={globals.isPending || globals.isError}
                  onChange={(e) => setGlobalID(e.target.value)}
                >
                  <option value="">Worker-specific settings</option>
                  {globalID && !globalProfile && (
                    <option value={globalID} disabled>
                      {globals.isPending
                        ? "Loading selected profile…"
                        : globals.isError
                          ? "Selected profile could not be checked"
                          : "Selected profile is unavailable"}
                    </option>
                  )}
                  {globals.data?.map((p) => (
                    <option
                      key={p.id}
                      value={p.id}
                      disabled={
                        workspace.runtime_type === "firecracker" &&
                        p.policy.mode === "open"
                      }
                    >
                      {p.name} · version {p.revision}
                    </option>
                  ))}
                </NativeSelect>
              </label>
              {globals.isError && (
                <ErrorNotice
                  error={globals.error}
                  onRetry={() => void globals.refetch()}
                  retrying={globals.isFetching}
                />
              )}
              {globalID && globals.isSuccess && !globalProfile && (
                <p role="status" className="form-note">
                  Choose an available connection profile before saving.
                </p>
              )}
              {globalID && (
                <p className="form-note">
                  Save to apply the latest version of the shared profile.
                </p>
              )}
              {globalID && globalProfile && (
                <p>
                  {globalProfile.description} Selected mode:{" "}
                  {globalProfile.policy.mode}.{" "}
                  {globalProfile.policy.destinations.join(", ")}
                </p>
              )}
              {!globalID && (
                <>
                  <label>
                    Connection profile
                    <NativeSelect
                      aria-label="Connection profile"
                      value={preset}
                      onChange={(e) => setPreset(e.target.value)}
                    >
                      <option value="blocked">
                        Blocked — platform integrations only
                      </option>
                      <option value="github">GitHub only</option>
                      <option value="custom">Custom allowlist</option>
                      <option
                        value="open"
                        disabled={workspace.runtime_type === "firecracker"}
                      >
                        Fully open —{" "}
                        {workspace.runtime_type === "firecracker"
                          ? "unavailable in Firecracker"
                          : "unrestricted network"}
                      </option>
                    </NativeSelect>
                  </label>
                  {preset === "custom" && (
                    <>
                      <label>
                        Allowed destinations
                        <Textarea
                          rows={6}
                          value={destinations}
                          onChange={(e) => setDestinations(e.target.value)}
                          placeholder={"github.com:443\nregistry.npmjs.org:443"}
                        />
                      </label>
                      <p className="form-note">
                        One exact public hostname or IP and TCP port per line,
                        maximum 64. No URLs or wildcards. HTTP and HTTPS use the
                        proxy; public guest DNS and direct connections stay
                        blocked. Private/LAN targets are excluded in this
                        profile.
                      </p>
                    </>
                  )}
                </>
              )}
              {connections.mode === "open" ? (
                <p className="notice">
                  Fully open allows direct TCP/UDP connections and external DNS,
                  including Internet, LAN and reachable host services.
                  Destination restrictions are disabled for this worker. Project
                  storage and private worker data stay separate.
                </p>
              ) : (
                <p className="form-note">
                  Platform integrations stay available. External HTTP and HTTPS
                  traffic uses the worker proxy.
                </p>
              )}
              {needsStop && (
                <p role="status" className="notice">
                  Stop this worker before applying this profile. Finish active
                  work, close terminals, then select Stop worker. Changing
                  direct networking or upgrading an older proxy requires a
                  stop/start; files and history are preserved.
                </p>
              )}
            </section>
            {mutation.isError && <ErrorNotice error={mutation.error} />}
            <div className="form-actions flex-wrap">
              {!onClose && (hasEdits || profileUpdate) && (
                <span className="mr-auto self-center text-xs text-muted-foreground">
                  {hasEdits ? "Unsaved changes" : "Profile update available"}
                </span>
              )}
              {onClose && (
                <Button variant="outline" type="button" onClick={onClose}>
                  Cancel
                </Button>
              )}
              <Button
                variant={
                  onClose || hasEdits || profileUpdate ? "default" : "outline"
                }
                disabled={!canSave}
              >
                {mutation.isPending ? "Saving…" : "Save settings"}
              </Button>
            </div>
          </fieldset>
        </Form>
      )}
    </>
  );
  if (!onClose)
    return <section aria-label="Worker settings">{content}</section>;
  return (
    <Modal
      title="Worker settings"
      onClose={onClose}
      closeDisabled={mutation.isPending}
    >
      {content}
    </Modal>
  );
}
