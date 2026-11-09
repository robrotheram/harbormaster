import { assistantApi, assistantKey, assistantOptions } from "./api";
import {
  hermesRuntimeNotice,
  type HermesConfig,
  type HermesSnapshot,
} from "./model";
import { useEffect, useState } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bot, Check, LockKeyhole, Save } from "lucide-react";

import { useConnectionProfiles } from "@/features/connection-profiles/api";
import { ErrorNotice, Form, useRequestKey } from "@/shared/components";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { NativeSelect } from "@/components/ui/native-select";
import { Badge } from "@/components/ui/badge";
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
    <>
      <div className="page-title">
        <div>
          <div className="mb-2 flex items-center gap-2">
            <h1>Hermes setup</h1>
            <Badge variant="outline">Project settings</Badge>
          </div>
          <p>
            Choose how Hermes will work in{" "}
            {project.id === "default" ? "this project" : project.name}.
          </p>
        </div>
      </div>
      {query.isError && <ErrorNotice error={query.error} />}
      {query.isPending ? (
        <Skeleton className="h-96 w-full" />
      ) : query.data ? (
        <HermesForm key={project.id} project={project} initial={query.data} />
      ) : null}
    </>
  );
}

function HermesForm({
  project,
  initial,
}: {
  project: Project;
  initial: HermesSnapshot;
}) {
  const client = useQueryClient();
  const navigate = useNavigate(),
    key = useRequestKey();
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
  return (
    <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,1fr)_280px]">
      <section className="rounded-xl border bg-background p-5 sm:p-7">
        <Form onSubmit={() => mutation.mutate()}>
          <fieldset
            disabled={mutation.isPending}
            className="space-y-6 disabled:opacity-70"
          >
            <div>
              <h2 className="mb-1 text-base font-semibold">Agent connection</h2>
              <p className="text-sm text-muted-foreground">
                These settings are private to the selected project.
              </p>
            </div>
            <label className="block space-y-2 text-sm font-medium">
              Use Hermes as
              <NativeSelect
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
            <div className="grid gap-5 sm:grid-cols-2">
              <label className="block space-y-2 text-sm font-medium">
                Provider
                <NativeSelect value="openai-codex" disabled>
                  <option value="openai-codex">
                    ChatGPT subscription · Codex
                  </option>
                </NativeSelect>
              </label>
              <label className="block space-y-2 text-sm font-medium">
                Model
                <Input
                  value={config.model}
                  required
                  maxLength={128}
                  placeholder="gpt-6.1-sol"
                  onChange={(e) => change({ model: e.target.value })}
                />
              </label>
            </div>
            <p className="text-xs leading-6 text-muted-foreground">
              Enter the exact model you want to use. Provider sign-in is a
              separate step; this form does not collect access tokens. API-key
              and paid-overage fallback are disabled.
            </p>
            <div className="border-t pt-6">
              <h2 className="mb-4 text-base font-semibold">Network access</h2>
              <label className="block space-y-2 text-sm font-medium">
                Connection profile
                <NativeSelect
                  value={config.connection_profile_id}
                  onChange={(e) =>
                    change({ connection_profile_id: e.target.value })
                  }
                >
                  <option value="">Blocked · platform integrations only</option>
                  {config.connection_profile_id &&
                    !candidates.some(
                      (p) => p.id === config.connection_profile_id,
                    ) && (
                      <option value={config.connection_profile_id} disabled>
                        Previously selected profile is unavailable
                      </option>
                    )}
                  {candidates.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
                </NativeSelect>
              </label>
              {profiles.isError && <ErrorNotice error={profiles.error} />}
              <p className="mt-2 text-xs leading-6 text-muted-foreground">
                Choose an allowlist for approved external services. Fully open
                networking is unavailable in the hardened sandbox.{" "}
                <Link
                  to="/connection-profiles"
                  className="underline underline-offset-4"
                >
                  Manage connection profiles
                </Link>
              </p>
            </div>
            {config.mode !== "workspace" && (
              <div className="space-y-5 border-t pt-6">
                <div>
                  <h2 className="text-base font-semibold">Matrix connection</h2>
                  <p className="mt-1 text-sm text-muted-foreground">
                    Bind one bot identity to exact rooms and senders in this
                    project. Saving automatically discovers and checks the
                    homeserver URL.
                  </p>
                </div>
                <label className="block space-y-2 text-sm font-medium">
                  Homeserver URL
                  <Input
                    type="url"
                    required={needsMatrix}
                    value={matrix.homeserver}
                    placeholder="https://matrix.example.org or your Element address"
                    onChange={(e) =>
                      matrixChange({ homeserver: e.target.value })
                    }
                  />
                </label>
                <label className="block space-y-2 text-sm font-medium">
                  Bot user ID
                  <Input
                    required={needsMatrix}
                    value={matrix.bot_user_id}
                    placeholder="@project-bot:example.org"
                    onChange={(e) =>
                      matrixChange({ bot_user_id: e.target.value })
                    }
                  />
                </label>
                <label className="block space-y-2 text-sm font-medium">
                  Allowed room IDs
                  <Textarea
                    required={needsMatrix}
                    rows={3}
                    value={rooms}
                    placeholder="!room-id:example.org"
                    onChange={(e) => {
                      setRooms(e.target.value);
                      setNotice("");
                    }}
                  />
                </label>
                <label className="block space-y-2 text-sm font-medium">
                  Allowed sender IDs
                  <Textarea
                    required={needsMatrix}
                    rows={3}
                    value={senders}
                    placeholder="@your-user:example.org"
                    onChange={(e) => {
                      setSenders(e.target.value);
                      setNotice("");
                    }}
                  />
                </label>
                {config.mode === "both" && (
                  <p className="text-xs text-muted-foreground">
                    You can save and use the workspace agent now, then add
                    Matrix connection details later.
                  </p>
                )}
                <p className="text-xs leading-6 text-muted-foreground">
                  One ID per line. Use a dedicated bot account. Invite it to
                  each configured encrypted room from your Matrix account. You
                  can accept those invitations below; all other room joins
                  remain blocked.
                </p>
                <p className="text-xs leading-6 text-muted-foreground">
                  Find the homeserver and full user IDs in your Matrix client’s
                  account details. Copy each room’s internal ID from its room
                  information; a #room alias or invitation link cannot be used
                  here.
                </p>
              </div>
            )}
          </fieldset>
          {mutation.isError && <ErrorNotice error={mutation.error} />}
          <div className="flex flex-wrap items-center gap-4 border-t pt-5">
            <Button
              type="submit"
              disabled={
                mutation.isPending || !config.model.trim() || (saved && !dirty)
              }
            >
              <Save />
              {mutation.isPending ? "Saving…" : "Save Hermes configuration"}
            </Button>
            {notice && (
              <p
                role="status"
                className="flex items-center gap-2 text-sm text-muted-foreground"
              >
                <Check className="size-4" />
                {notice}
              </p>
            )}
          </div>
        </Form>
      </section>
      <aside className="space-y-5 xl:sticky xl:top-6">
        <section className="rounded-xl border bg-background p-5">
          <div className="mb-5 flex items-center gap-2">
            <Bot className="size-5 text-muted-foreground" />
            <h2 className="text-sm font-semibold">Setup status</h2>
          </div>
          <dl className="space-y-4 text-sm">
            <div>
              <dt className="text-muted-foreground">Configuration</dt>
              <dd className="mt-1 font-medium">
                {saved
                  ? dirty
                    ? "Unsaved changes"
                    : `Saved · revision ${config.revision}`
                  : "Not saved yet"}
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Hermes runtime</dt>
              <dd className="mt-1 font-medium">
                {available ? "Available in Firecracker" : runtimeNotice?.label}
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Provider connection</dt>
              <dd className="mt-1 font-medium">
                {available
                  ? "Workbench Codex gateway"
                  : "Pending runtime setup"}
              </dd>
            </div>
            {config.mode !== "workspace" && (
              <div>
                <dt className="text-muted-foreground">Matrix connection</dt>
                <dd className="mt-1 font-medium">
                  {initial.matrix_state === "connected"
                    ? "Connected"
                    : initial.matrix_state === "starting"
                      ? "Connecting…"
                      : initial.matrix_state === "failed"
                        ? "Connection failed"
                        : initial.matrix_available
                          ? "Not connected"
                          : "Installation required"}
                </dd>
              </div>
            )}
          </dl>
          <Button
            className="mt-4"
            variant="outline"
            onClick={() =>
              client.refetchQueries({ queryKey: assistantKey(project.id) })
            }
          >
            Refresh runtime status
          </Button>
        </section>
        {available && (
          <section className="rounded-xl border bg-background p-5 space-y-3">
            <h2 className="font-semibold">Workspace agent</h2>
            <p className="text-sm text-muted-foreground">
              Save your settings, then start Hermes. Open its workspace and
              choose Run task.
            </p>
            <Button
              disabled={!saved || dirty || launch.isPending}
              onClick={() => launch.mutate()}
            >
              {launch.isPending ? "Starting…" : "Start Hermes workspace"}
            </Button>
            {launch.isError && <ErrorNotice error={launch.error} />}
          </section>
        )}
        {available &&
          (config.mode !== "workspace" ||
            (initial.matrix_state &&
              initial.matrix_state !== "disconnected")) && (
            <section className="rounded-xl border bg-background p-5 space-y-4">
              <h2 className="font-semibold">Connect Matrix</h2>
              <p className="text-sm leading-6 text-muted-foreground">
                Save the Matrix settings, then save the bot token. The token
                stays on the host and is never returned to this form or the
                agent VM.
              </p>
              <label className="block space-y-2 text-sm font-medium">
                Bot access token
                <Input
                  type="password"
                  autoComplete="off"
                  value={botToken}
                  onChange={(e) => {
                    setBotToken(e.target.value);
                    setBotNotice("");
                    saveBot.reset();
                  }}
                />
              </label>
              <p className="text-xs leading-5 text-muted-foreground">
                Paste just the token value, then select Save bot credential.
                Pasting alone does not save it or connect the bot.
              </p>
              {(!saved || dirty) && (
                <p
                  role="status"
                  className="text-xs leading-5 text-muted-foreground"
                >
                  Save the Hermes configuration above before saving the bot
                  token.
                </p>
              )}
              <label className="flex items-start gap-2 text-sm leading-5">
                <input
                  type="checkbox"
                  className="mt-1"
                  checked={acceptInvitations}
                  onChange={(e) => setAcceptInvitations(e.target.checked)}
                />
                Accept invitations to the configured rooms when saving. This
                cannot join other rooms.
              </label>
              <Button
                variant="outline"
                disabled={
                  !saved ||
                  dirty ||
                  !config.matrix ||
                  !botToken.trim() ||
                  saveBot.isPending
                }
                onClick={() => saveBot.mutate()}
              >
                {saveBot.isPending ? "Checking…" : "Save bot credential"}
              </Button>
              {botNotice && (
                <p role="status" className="text-sm leading-5">
                  {botNotice}
                </p>
              )}
              <p className="text-xs text-muted-foreground">
                {initial.matrix_credential_configured
                  ? "Bot credential is stored."
                  : "No bot credential stored."}
              </p>
              {saveBot.isError && <ErrorNotice error={saveBot.error} />}
              <div className="flex flex-wrap gap-2">
                <Button
                  disabled={
                    dirty ||
                    !initial.workspace_ready ||
                    !initial.matrix_credential_configured ||
                    matrixAction.isPending ||
                    initial.matrix_state === "connected"
                  }
                  onClick={() => matrixAction.mutate("start")}
                >
                  Connect Matrix
                </Button>
                <Button
                  variant="outline"
                  disabled={
                    !initial.workspace_ready ||
                    matrixAction.isPending ||
                    !initial.matrix_state ||
                    initial.matrix_state === "disconnected"
                  }
                  onClick={() => matrixAction.mutate("stop")}
                >
                  Disconnect
                </Button>
              </div>
              <p className="text-xs leading-5 text-muted-foreground">
                Start the Hermes workspace before connecting. Changed Matrix
                settings are applied by restarting an idle workspace when you
                connect. Active work is never interrupted. Text chat uses
                required encryption; device verification may need completing in
                your Matrix client.
              </p>
              {matrixAction.isError && (
                <ErrorNotice error={matrixAction.error} />
              )}
            </section>
          )}
        {!available && (
          <div className="rounded-xl border border-amber-200 bg-amber-50 p-5 text-sm leading-6 text-amber-950 dark:border-amber-900 dark:bg-amber-950/40 dark:text-amber-200">
            <h2 className="mb-2 font-semibold">{runtimeNotice?.title}</h2>
            <p>{runtimeNotice?.body}</p>
          </div>
        )}
        <p className="flex gap-2 px-1 text-xs leading-5 text-muted-foreground">
          <LockKeyhole className="mt-0.5 size-4 shrink-0" />
          Provider credentials, bot tokens and private agent history are not
          saved in these project settings.
        </p>
      </aside>
    </div>
  );
}
