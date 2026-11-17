import { assistantApi, assistantKey, assistantOptions } from "./api";
import {
  hermesRuntimeNotice,
  type HermesConfig,
  type HermesSnapshot,
} from "./model";
import { useEffect, useState } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Bot,
  Check,
  Save,
  RefreshCw,
  MessageSquare,
  ArrowUpRight,
} from "lucide-react";
import { Tabs } from "radix-ui";

import { useConnectionProfiles } from "@/features/connection-profiles/api";
import { ErrorNotice, Form, useRequestKey } from "@/shared/components";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { NativeSelect } from "@/components/ui/native-select";
import { Skeleton } from "@/components/ui/skeleton";
import type { Project } from "@/shared/types";

const lines = (value: string) =>
  value
    .split("\n")
    .map((v) => v.trim())
    .filter(Boolean);

export function HermesSettings({ project }: { project: Project }) {
  const query = useQuery(assistantOptions(project.id));
  return (
    <div className="hermes-setup w-full">
      {query.isError && (
        <ErrorNotice
          error={query.error}
          onRetry={() => void query.refetch()}
          retrying={query.isFetching}
        />
      )}
      {query.isPending ? (
        <Skeleton className="h-96 w-full" />
      ) : query.data ? (
        <HermesForm
          key={project.id}
          project={project}
          initial={query.data}
          statusUnavailable={query.isError}
        />
      ) : null}
    </div>
  );
}

function HermesForm({
  project,
  initial,
  statusUnavailable,
}: {
  project: Project;
  initial: HermesSnapshot;
  statusUnavailable: boolean;
}) {
  const client = useQueryClient();
  const navigate = useNavigate(),
    key = useRequestKey();
  const [tab, setTab] = useState(
    initial.config.mode === "matrix" ? "matrix" : "agent",
  );
  const available = initial.runtime_available;
  const runtimeNotice = hermesRuntimeNotice(initial);
  const profiles = useConnectionProfiles();
  const [config, setConfig] = useState(initial.config);
  const [rooms, setRooms] = useState(
    initial.config.matrix?.room_ids.join("\n") || "",
  );
  const [senders, setSenders] = useState(
    initial.config.matrix?.sender_ids.join("\n") || "",
  );
  const [baseline, setBaseline] = useState(JSON.stringify(initial.config));
  const [saved, setSaved] = useState(initial.saved);
  const [notice, setNotice] = useState("");
  const [botToken, setBotToken] = useState("");
  const [botNotice, setBotNotice] = useState("");
  const [acceptInvitations, setAcceptInvitations] = useState(false);
  const matrix = config.matrix || {
    homeserver: "",
    bot_user_id: "",
    room_ids: [],
    sender_ids: [],
  };
  const needsMatrix =
    config.mode === "matrix" ||
    !!(
      matrix.homeserver ||
      matrix.bot_user_id ||
      rooms.trim() ||
      senders.trim()
    );
  const payload: HermesConfig = {
    ...config,
    model: config.model.trim(),
    matrix:
      config.mode === "workspace" || !needsMatrix
        ? null
        : {
            ...matrix,
            homeserver: matrix.homeserver.trim(),
            bot_user_id: matrix.bot_user_id.trim(),
            room_ids: lines(rooms),
            sender_ids: lines(senders),
          },
  };
  const dirty = JSON.stringify(payload) !== baseline;
  useEffect(() => {
    const next = JSON.stringify(initial.config);
    if (!dirty && !botToken && next !== baseline) {
      setConfig(initial.config);
      setBaseline(next);
      setSaved(initial.saved);
      setRooms(initial.config.matrix?.room_ids.join("\n") || "");
      setSenders(initial.config.matrix?.sender_ids.join("\n") || "");
    }
  }, [initial.config, initial.saved, baseline, dirty, botToken]);
  const mutation = useMutation({
    mutationFn: () => assistantApi.save(project.id, payload),
    onSuccess: (value) => {
      setConfig(value.config);
      setBaseline(JSON.stringify(value.config));
      setSaved(true);
      setRooms(value.config.matrix?.room_ids.join("\n") || "");
      setSenders(value.config.matrix?.sender_ids.join("\n") || "");
      client.setQueryData(assistantKey(project.id), value);
      setNotice("Hermes configuration saved for this project.");
    },
  });
  const change = (patch: Partial<HermesConfig>) => {
    setConfig((c) => ({ ...c, ...patch }));
    setNotice("");
    mutation.reset();
  };
  const matrixChange = (patch: Partial<NonNullable<HermesConfig["matrix"]>>) =>
    change({ matrix: { ...matrix, ...patch } });
  const candidates =
    profiles.data?.filter((p) => p.policy.mode !== "open") || [];
  const launch = useMutation({
    mutationFn: () =>
      assistantApi.start(
        project.id,
        key({ project_id: project.id, revision: config.revision }),
      ),
    onSuccess: () => {
      client.invalidateQueries({ queryKey: ["project", project.id] });
      navigate({
        to: "/projects/$projectId/$section",
        params: { projectId: project.id, section: "workspaces" },
      });
    },
  });
  const saveBot = useMutation({
    mutationFn: () =>
      assistantApi.saveMatrixCredential(
        project.id,
        botToken.trim(),
        config.revision,
        acceptInvitations,
      ),
    onSuccess: () => {
      setBotToken("");
      setBotNotice(
        "Bot identity, room membership and encryption checked. Credential stored. Select Connect Matrix to start the bot.",
      );
      client.invalidateQueries({ queryKey: assistantKey(project.id) });
    },
  });
  const matrixAction = useMutation({
    mutationFn: (action: "start" | "stop") =>
      assistantApi.matrixAction(project.id, action),
    onSuccess: () =>
      client.invalidateQueries({ queryKey: assistantKey(project.id) }),
  });
  const busy =
    mutation.isPending ||
    saveBot.isPending ||
    launch.isPending ||
    matrixAction.isPending;
  const profileMissing =
    !!config.connection_profile_id &&
    profiles.isSuccess &&
    !candidates.some((profile) => profile.id === config.connection_profile_id);
  const matrixIncomplete =
    config.mode !== "workspace" &&
    needsMatrix &&
    (!matrix.homeserver.trim() ||
      !matrix.bot_user_id.trim() ||
      !lines(rooms).length ||
      !lines(senders).length);
  const canSave =
    !busy &&
    !statusUnavailable &&
    !profileMissing &&
    !matrixIncomplete &&
    !!config.model.trim() &&
    (!saved || dirty);
  const matrixState =
    initial.matrix_state === "connected"
      ? "Connected"
      : initial.matrix_state === "starting"
        ? "Connecting…"
        : initial.matrix_state === "failed"
          ? "Connection failed"
          : initial.matrix_available
            ? "Not connected"
            : "Installation required";
  const matrixEnabled = config.mode !== "workspace";
  const liveMatrix =
    !!initial.matrix_state && initial.matrix_state !== "disconnected";
  const configurationState = dirty
    ? "Unsaved changes"
    : saved
      ? `Saved · revision ${config.revision}`
      : "Not saved yet";
  const saveConfiguration = (
    <div className="flex flex-wrap items-center gap-3 border-t pt-4">
      <Button type="submit" disabled={!canSave}>
        <Save />
        {mutation.isPending ? "Saving…" : "Save settings"}
      </Button>
      <span className="text-xs text-muted-foreground">
        {configurationState}
      </span>
      {matrixIncomplete && (
        <p className="w-full text-xs text-amber-700 dark:text-amber-400">
          Complete the homeserver, bot user ID, allowed rooms and allowed
          senders in the Matrix tab before saving.
        </p>
      )}
      {profileMissing && (
        <p className="w-full text-xs text-amber-700 dark:text-amber-400">
          Choose an available connection profile in the Agent tab before saving.
        </p>
      )}
    </div>
  );
  const workspaceAction =
    available &&
    (initial.workspace_ready ? (
      <Button variant="outline" asChild>
        <Link
          to="/projects/$projectId/$section"
          params={{ projectId: project.id, section: "workspaces" }}
        >
          <ArrowUpRight />
          Open Hermes workspace
        </Link>
      </Button>
    ) : (
      <Button
        disabled={!saved || dirty || busy || statusUnavailable}
        onClick={() => launch.mutate()}
      >
        <Bot />
        {launch.isPending ? "Starting…" : "Start Hermes"}
      </Button>
    ));
  return (
    <div className="space-y-4">
      <div
        className="flex items-start justify-between gap-3 border px-4 py-3"
        aria-label="Setup status"
      >
        <div className="flex min-w-0 flex-1 flex-wrap items-center gap-x-4 gap-y-2 text-xs sm:min-h-7">
          <span className="flex items-center gap-2">
            <span
              className={
                available && !statusUnavailable
                  ? "size-1.5 bg-teal-500 rounded-full"
                  : "size-1.5 bg-amber-500 rounded-full"
              }
            />
            {statusUnavailable
              ? "Runtime status unavailable"
              : available
                ? "Hermes runtime available"
                : runtimeNotice?.label}
          </span>
          <span className="text-muted-foreground">
            Agent{" "}
            <strong className="ml-1 font-medium text-foreground">
              {statusUnavailable
                ? "Unknown"
                : initial.workspace_ready
                  ? "Running"
                  : "Not running"}
            </strong>
          </span>
          {(matrixEnabled || liveMatrix) && (
            <span className="text-muted-foreground">
              Matrix{" "}
              <strong className="ml-1 font-medium text-foreground">
                {statusUnavailable ? "Unknown" : matrixState}
              </strong>
            </span>
          )}
        </div>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="Refresh runtime status"
          title="Refresh runtime status"
          onClick={() =>
            client.refetchQueries({ queryKey: assistantKey(project.id) })
          }
        >
          <RefreshCw />
        </Button>
      </div>
      {!available && (
        <div
          className="border border-amber-500/30 bg-amber-500/5 p-4 text-xs leading-5"
          role="status"
        >
          <h2 className="mb-1 font-medium">{runtimeNotice?.title}</h2>
          <p className="text-muted-foreground">{runtimeNotice?.body}</p>
        </div>
      )}
      <Tabs.Root
        value={tab}
        onValueChange={setTab}
        className="min-w-0 border bg-background"
      >
        <Tabs.List
          aria-label="Hermes settings"
          className="hermes-tabs grid grid-cols-2 gap-2 border-b px-4 sm:flex sm:gap-6 sm:px-5"
        >
          <Tabs.Trigger value="agent">
            <Bot />
            Agent
          </Tabs.Trigger>
          <Tabs.Trigger value="matrix">
            <MessageSquare />
            Matrix
          </Tabs.Trigger>
        </Tabs.List>
        <Tabs.Content value="agent" className="min-w-0 p-4 sm:p-5">
          <Form
            onSubmit={() => {
              if (canSave) mutation.mutate();
            }}
          >
            <fieldset
              disabled={busy}
              className="min-w-0 space-y-4 disabled:opacity-70"
            >
              <div>
                <h2 className="text-sm font-semibold">Agent connection</h2>
                <p className="mt-1 text-xs text-muted-foreground">
                  Choose how this project's Hermes agent works.
                </p>
              </div>
              <label>
                Use Hermes as
                <NativeSelect
                  size="sm"
                  className="text-xs"
                  value={config.mode}
                  onChange={(e) =>
                    change({ mode: e.target.value as HermesConfig["mode"] })
                  }
                >
                  <option value="workspace">Workspace agent</option>
                  <option value="matrix">Matrix bot</option>
                  <option value="both">Workspace agent and Matrix bot</option>
                </NativeSelect>
              </label>
              <label>
                Model
                <Input
                  className="h-8 text-xs md:text-xs"
                  value={config.model}
                  required
                  maxLength={128}
                  placeholder="gpt-6.1-sol"
                  onChange={(e) => change({ model: e.target.value })}
                />
              </label>
              <div className="space-y-2 text-xs text-muted-foreground">
                <p>
                  Uses your ChatGPT subscription through the Workbench Codex
                  gateway.
                </p>
                <details className="leading-5">
                  <summary className="cursor-pointer hover:text-foreground">
                    Provider sign-in
                  </summary>
                  <p className="mt-2 max-w-prose">
                    Sign-in is managed separately. API-key and paid-overage
                    fallback remain disabled.
                  </p>
                </details>
              </div>
              <div className="space-y-3 border-t pt-4">
                <h2 className="text-sm font-semibold">Network access</h2>
                <label>
                  Connection profile
                  <NativeSelect
                    size="sm"
                    className="text-xs"
                    value={config.connection_profile_id}
                    disabled={profiles.isPending || profiles.isError}
                    onChange={(e) =>
                      change({ connection_profile_id: e.target.value })
                    }
                  >
                    <option value="">
                      Blocked · platform integrations only
                    </option>
                    {config.connection_profile_id &&
                      !candidates.some(
                        (p) => p.id === config.connection_profile_id,
                      ) && (
                        <option value={config.connection_profile_id} disabled>
                          {profiles.isPending
                            ? "Loading selected profile…"
                            : profiles.isError
                              ? "Selected profile could not be checked"
                              : "Previously selected profile is unavailable"}
                        </option>
                      )}
                    {candidates.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name}
                      </option>
                    ))}
                  </NativeSelect>
                </label>
                {profiles.isError && (
                  <ErrorNotice
                    error={profiles.error}
                    onRetry={() => void profiles.refetch()}
                    retrying={profiles.isFetching}
                  />
                )}
                <p className="text-xs leading-5 text-muted-foreground">
                  Choose an allowlist for external services.{" "}
                  <Link
                    to="/connection-profiles"
                    className="underline underline-offset-4"
                  >
                    Manage network profiles
                  </Link>
                </p>
              </div>
            </fieldset>
            {mutation.isError && <ErrorNotice error={mutation.error} />}
            {saveConfiguration}
          </Form>
          <div className="mt-5 flex flex-wrap items-center justify-between gap-3 border-t pt-4">
            <div>
              <h2 className="text-xs font-medium">Workspace agent</h2>
              <p className="mt-1 text-xs text-muted-foreground">
                {statusUnavailable
                  ? "Refresh runtime status to check whether Hermes is running. Your edits are preserved."
                  : initial.workspace_ready
                    ? "Hermes is running in this project."
                    : "Save your configuration, then start Hermes."}
              </p>
            </div>
            {workspaceAction}
          </div>
          {launch.isError && <ErrorNotice error={launch.error} />}
        </Tabs.Content>
        <Tabs.Content value="matrix" className="min-w-0 p-4 sm:p-5 space-y-5">
          {matrixEnabled ? (
            <Form
              onSubmit={() => {
                if (canSave) mutation.mutate();
              }}
            >
              <fieldset
                disabled={busy}
                className="min-w-0 space-y-4 disabled:opacity-70"
              >
                <div>
                  <h2 className="text-sm font-semibold">Matrix connection</h2>
                  <p className="mt-1 text-xs leading-5 text-muted-foreground">
                    Connect a dedicated bot to this project's encrypted rooms.
                  </p>
                </div>
                <label>
                  Homeserver URL
                  <Input
                    className="h-8 text-xs md:text-xs"
                    type="url"
                    required={needsMatrix}
                    value={matrix.homeserver}
                    placeholder="https://matrix.example.org"
                    onChange={(e) =>
                      matrixChange({ homeserver: e.target.value })
                    }
                  />
                </label>
                <label>
                  Bot user ID
                  <Input
                    className="h-8 text-xs md:text-xs"
                    required={needsMatrix}
                    value={matrix.bot_user_id}
                    placeholder="@project-bot:example.org"
                    onChange={(e) =>
                      matrixChange({ bot_user_id: e.target.value })
                    }
                  />
                </label>
                <div className="space-y-2">
                  <label htmlFor="hermes-room-ids">Allowed room IDs</label>
                  <Textarea
                    id="hermes-room-ids"
                    required={needsMatrix}
                    rows={2}
                    className="min-h-14 text-xs md:text-xs"
                    value={rooms}
                    placeholder="!room-id:example.org"
                    onChange={(e) => {
                      setRooms(e.target.value);
                      setNotice("");
                      mutation.reset();
                    }}
                  />
                </div>
                <div className="space-y-2">
                  <label htmlFor="hermes-sender-ids">Allowed sender IDs</label>
                  <Textarea
                    id="hermes-sender-ids"
                    required={needsMatrix}
                    rows={2}
                    className="min-h-14 text-xs md:text-xs"
                    value={senders}
                    placeholder="@your-user:example.org"
                    onChange={(e) => {
                      setSenders(e.target.value);
                      setNotice("");
                      mutation.reset();
                    }}
                  />
                </div>
                <details className="text-xs leading-5 text-muted-foreground">
                  <summary className="cursor-pointer hover:text-foreground">
                    Finding IDs and inviting your bot
                  </summary>
                  <p className="mt-2 max-w-prose">
                    Enter one ID per line. Find user IDs in account details and
                    each room's internal ID in room information. Room aliases
                    and invitation links cannot be used. Invite the bot to each
                    encrypted room; accepting invitations below only joins the
                    configured rooms.
                  </p>
                  {config.mode === "both" && (
                    <p className="mt-2">
                      Matrix is optional. Leave all fields empty to use only the
                      workspace agent for now.
                    </p>
                  )}
                </details>
              </fieldset>
              {mutation.isError && <ErrorNotice error={mutation.error} />}
              {saveConfiguration}
            </Form>
          ) : (
            <div className="space-y-3">
              <h2 className="text-sm font-semibold">Matrix is not enabled</h2>
              <p className="text-xs leading-5 text-muted-foreground">
                Choose Matrix bot or Workspace agent and Matrix bot in Agent
                settings to configure this connection.
              </p>
              <Button variant="outline" onClick={() => setTab("agent")}>
                Agent settings
              </Button>
            </div>
          )}
          {available && matrixEnabled && (
            <section aria-label="Bot credential" className="border-t pt-4">
              <details
                open={!initial.matrix_credential_configured || undefined}
                className="hermes-credential"
              >
                <summary className="cursor-pointer text-xs font-medium">
                  {initial.matrix_credential_configured
                    ? "Bot credential stored · replace credential"
                    : "Add bot credential"}
                </summary>
                <div className="mt-4 space-y-3">
                  <label>
                    Bot access token
                    <Input
                      className="h-8 text-xs md:text-xs"
                      type="password"
                      autoComplete="off"
                      value={botToken}
                      disabled={busy}
                      onChange={(e) => {
                        setBotToken(e.target.value);
                        setBotNotice("");
                        saveBot.reset();
                      }}
                    />
                  </label>
                  <p className="text-xs leading-5 text-muted-foreground">
                    Paste the token, then save it. It stays on the host and is
                    never returned here or exposed to the worker.
                  </p>
                  {(!saved || dirty) && (
                    <p
                      role="status"
                      className="text-xs text-amber-700 dark:text-amber-400"
                    >
                      Save the Hermes configuration above before saving the bot
                      token.
                    </p>
                  )}
                  <label className="flex items-start gap-2 text-xs leading-5">
                    <input
                      type="checkbox"
                      className="mt-1"
                      checked={acceptInvitations}
                      disabled={busy}
                      onChange={(e) => setAcceptInvitations(e.target.checked)}
                    />
                    Accept invitations to the configured rooms when saving.
                  </label>
                  <Button
                    variant="outline"
                    disabled={
                      !saved ||
                      dirty ||
                      !config.matrix ||
                      !botToken.trim() ||
                      busy ||
                      statusUnavailable
                    }
                    onClick={() => saveBot.mutate()}
                  >
                    {saveBot.isPending ? "Checking…" : "Save bot credential"}
                  </Button>
                  {saveBot.isError && <ErrorNotice error={saveBot.error} />}
                </div>
              </details>
              {botNotice && (
                <p role="status" className="mt-3 text-xs leading-5">
                  {botNotice}
                </p>
              )}
            </section>
          )}
          {available && (matrixEnabled || liveMatrix) && (
            <section
              aria-label="Matrix controls"
              className="border-t pt-4 space-y-3"
            >
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <h2 className="text-xs font-medium">
                    {statusUnavailable
                      ? "Connection status unavailable"
                      : matrixState}
                  </h2>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {statusUnavailable
                      ? "Refresh runtime status before changing the Matrix connection."
                      : !initial.workspace_ready
                        ? "Start Hermes before connecting Matrix."
                        : initial.matrix_state === "connected"
                          ? "The bot is listening in your configured rooms."
                          : "Save your settings and bot credential, then connect."}
                  </p>
                </div>
                <div className="flex flex-wrap gap-2">
                  {!initial.workspace_ready && workspaceAction}
                  {initial.matrix_state !== "connected" && (
                    <Button
                      disabled={
                        !matrixEnabled ||
                        dirty ||
                        !initial.workspace_ready ||
                        !initial.matrix_credential_configured ||
                        busy ||
                        statusUnavailable ||
                        initial.matrix_state === "connected" ||
                        initial.matrix_state === "starting"
                      }
                      onClick={() => matrixAction.mutate("start")}
                    >
                      {initial.matrix_state === "starting"
                        ? "Connecting…"
                        : "Connect Matrix"}
                    </Button>
                  )}
                  {liveMatrix && (
                    <Button
                      variant="outline"
                      disabled={
                        !initial.workspace_ready || busy || statusUnavailable
                      }
                      onClick={() => matrixAction.mutate("stop")}
                    >
                      Disconnect
                    </Button>
                  )}
                </div>
              </div>
              {matrixAction.isError && (
                <ErrorNotice error={matrixAction.error} />
              )}
              {launch.isError && <ErrorNotice error={launch.error} />}
              <details className="text-xs leading-5 text-muted-foreground">
                <summary className="cursor-pointer hover:text-foreground">
                  Connection behavior
                </summary>
                <p className="mt-2 max-w-prose">
                  Connecting applies changed settings by restarting an idle
                  workspace. Active work is never interrupted. Text chat
                  requires encryption; device verification may need completing
                  in your Matrix client.
                </p>
              </details>
            </section>
          )}
        </Tabs.Content>
      </Tabs.Root>
      {notice && (
        <p role="status" className="flex items-center gap-2 text-xs">
          <Check className="size-3.5" />
          {notice}
        </p>
      )}
    </div>
  );
}
