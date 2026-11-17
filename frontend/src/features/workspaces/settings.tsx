import { workspacesApi } from "./api";
import {
  parseDestinations,
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
}: {
  workspace: Workspace;
  onClose: () => void;
  onSaved: (updated: Workspace) => void;
}) {
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
        ? original.destinations.length === 1 &&
          original.destinations[0] === "github.com:443"
          ? "github"
          : "custom"
        : original?.mode === "blocked"
          ? "blocked"
          : workspace.github_access
            ? "github"
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
  const canSave =
    !mutation.isPending &&
    !needsStop &&
    (!globalID || (globals.isSuccess && !!globalProfile));
  return (
    <Modal
      title="Worker settings"
      onClose={onClose}
      closeDisabled={mutation.isPending}
    >
      <Form
        onSubmit={() => {
          if (canSave) mutation.mutate();
        }}
      >
        <fieldset disabled={mutation.isPending} className="min-w-0 space-y-5">
          <p>{workspace.name || workspace.id}</p>
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
            Opening starts a fresh idle interval. Active runs and background
            jobs prevent shutdown.
          </p>
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
          <p className="form-note">
            Manage shared profiles in Network profiles in the sidebar. Save here
            to apply the latest version. Worker-specific settings detach this
            worker from global updates.
          </p>
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
                    proxy; public guest DNS and direct connections stay blocked.
                    Private/LAN targets are excluded in this profile.
                  </p>
                </>
              )}
            </>
          )}
          {connections.mode === "open" ? (
            <p className="notice">
              Fully open allows direct TCP/UDP connections and external DNS,
              including Internet, LAN and reachable host services. Destination
              restrictions are disabled for this worker. Project storage and
              private worker data stay separate.
            </p>
          ) : (
            <p className="form-note">
              Approved platform integrations remain available. Tools must use
              the supplied HTTP/HTTPS proxy for external connections. Provider
              credentials remain in the platform.
            </p>
          )}
          {needsStop && (
            <p role="status" className="notice">
              Stop this worker before applying this profile. Finish active work,
              close terminals, then use Stop on its worker row. Changing direct
              networking or upgrading an older proxy requires a stop/start;
              files and history are preserved.
            </p>
          )}
          {mutation.isError && <ErrorNotice error={mutation.error} />}
          <div className="form-actions">
            <Button variant="outline" type="button" onClick={onClose}>
              Cancel
            </Button>
            <Button variant="default" disabled={!canSave}>
              {mutation.isPending ? "Saving…" : "Save settings"}
            </Button>
          </div>
        </fieldset>
      </Form>
    </Modal>
  );
}
