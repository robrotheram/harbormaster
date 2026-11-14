import { useEffect, useRef, useState, type RefObject } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Globe,
  Monitor,
  PanelRightClose,
  RefreshCw,
  ExternalLink,
  MousePointer2,
  Settings2,
  Smartphone,
  Play,
  Terminal,
  MoreHorizontal,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { ErrorNotice, Form, Modal, useRequestKey } from "@/shared/components";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import { request } from "@/shared/http";
import { privateContentOptions } from "@/shared/private-content";

import { ApplicationLogs, type Application } from "./application-logs";

export function WorkPreview({
  project,
  workspace,
  revision,
  onCollapse,
  onReady,
  onSelection,
  collapsible = true,
  reviewMode = false,
}: {
  project: string;
  workspace: string;
  revision?: string;
  onCollapse: () => void;
  onReady: () => void;
  onSelection: (text: string) => void;
  collapsible?: boolean;
  reviewMode?: boolean;
}) {
  const client = useQueryClient();
  const key = useRequestKey();
  const frame = useRef<HTMLIFrameElement>(null);
  const nonce = useRef("");
  const selectionMenuClosing = useRef(false);
  const popup = useRef<Window | null>(null);
  const [mode, setMode] = useState<"web" | "desktop">("web");
  const [settings, setSettings] = useState(false);
  const [logs, setLogs] = useState(false);
  const [selectedID, setSelectedID] = useState("");
  const [directory, setDirectory] = useState(".");
  const [port, setPort] = useState("4173");
  const [command, setCommand] = useState(
    'python3 -m http.server "$PORT" --bind 127.0.0.1',
  );
  const [sequence, setSequence] = useState(0);
  const [refresh, setRefresh] = useState(0);
  const [completedRevision, setCompletedRevision] = useState(revision);
  useEffect(() => {
    if (revision) setCompletedRevision(revision);
  }, [revision]);
  const [phone, setPhone] = useState(false);
  const [inspecting, setInspecting] = useState(false);
  const queryKey = ["project", project, "applications", workspace];
  const query = useQuery({
    ...privateContentOptions,
    queryKey,
    queryFn: ({ signal }) =>
      request<{ items: Application[] }>(
        `/workspaces/${workspace}/applications`,
        undefined,
        signal,
      ),
    refetchInterval: 3000,
  });
  const apps = (query.isError ? [] : query.data?.items || []).filter((a) =>
    mode === "desktop"
      ? a.application.kind === "desktop"
      : a.application.kind !== "desktop",
  );
  const active = apps.filter((a) => a.state === "active");
  const selected = active.find((a) => a.id === selectedID) || active[0];
  const restartable = apps.find(
    (a) => a.application.kind !== "forward" && !!a.application.command,
  );
  const stopped =
    !!query.data?.items.length &&
    !query.data.items.some((a) => a.state === "active");
  useEffect(() => {
    if (stopped) onCollapse();
  }, [stopped, onCollapse]);
  useEffect(() => {
    if (selected?.ready) onReady();
  }, [selected?.id, selected?.ready, onReady]);
  const detected = useQuery({
    ...privateContentOptions,
    queryKey: ["project", project, "ports", workspace],
    queryFn: ({ signal }) =>
      request<{ items: { port: number; listener: string }[] }>(
        `/workspaces/${workspace}/ports`,
        undefined,
        signal,
      ),
    refetchInterval: 3000,
    retry: false,
  });
  const tried = useRef(new Set<string>());
  const forward = useMutation({
    mutationFn: (port: { port: number; listener: string; manual?: boolean }) =>
      request(`/workspaces/${workspace}/applications`, {
        application: {
          kind: "forward",
          directory: ".",
          command: "",
          port: port.port,
          listener: port.listener,
        },
        ttl_seconds: 3600,
        idempotency_key: port.manual
          ? key({ listener: port.listener, sequence })
          : `forward-${port.listener}`,
      }),
    onSuccess: () => {
      setSequence((v) => v + 1);
      return client.invalidateQueries({ queryKey });
    },
  });
  useEffect(() => {
    if (mode !== "web" || query.isPending || query.isError || forward.isPending)
      return;
    const port = detected.data?.items.find(
      (p) =>
        !tried.current.has(p.listener) &&
        !query.data?.items.some(
          (a) =>
            a.application.listener === p.listener ||
            (a.state === "active" && a.application.port === p.port),
        ),
    );
    if (port) {
      tried.current.add(port.listener);
      forward.mutate(port);
    }
  }, [
    mode,
    query.data,
    query.isPending,
    query.isError,
    detected.data,
    forward.isPending,
    forward.mutate,
  ]);
  async function connect(id: string, embedded: boolean, signal?: AbortSignal) {
    const result = await request<{ preview_url: string }>(
      `/workspaces/${workspace}/services/${id}/connect`,
      { embedded },
      signal,
    );
    const url = new URL(result.preview_url);
    if (
      url.protocol !== "http:" ||
      !/^preview-[a-f0-9]+\.localhost$/.test(url.hostname)
    )
      throw new Error("Invalid preview address");
    return result.preview_url;
  }
  const [url, setURL] = useState("");
  useEffect(() => {
    if (!detected.data?.items.length) forward.reset();
  }, [detected.data]);
  useEffect(() => {
    setInspecting(false);
    nonce.current = "";
    if (!url) return;
    const origin = new URL(url).origin;
    const receive = (event: MessageEvent) => {
      if (
        event.source !== frame.current?.contentWindow ||
        event.origin !== origin ||
        event.data?.type !== "workbench-selection" ||
        !nonce.current ||
        event.data.nonce !== nonce.current
      )
        return;
      const { selector, text, path } = event.data;
      if (
        typeof selector !== "string" ||
        typeof text !== "string" ||
        typeof path !== "string"
      )
        return;
      nonce.current = "";
      setInspecting(false);
      onSelection(
        `Change ${selector.slice(0, 200)} on ${path.slice(0, 200)}${text ? ` ("${text.slice(0, 300)}")` : ""}: `,
      );
    };
    window.addEventListener("message", receive);
    return () => window.removeEventListener("message", receive);
  }, [url, onSelection]);
  const mutation = useMutation({
    mutationFn: async (
      action: "start" | "restart" | "desktop" | "stop" | "open",
    ) => {
      if (action === "start" || action === "restart" || action === "desktop") {
        const application =
          action === "restart"
            ? restartable!.application
            : action === "desktop"
              ? {
                  kind: "desktop",
                  directory: ".",
                  command: "workbench-desktop",
                  port: 6080,
                }
              : { directory: directory.trim(), command, port: Number(port) };
        return request(`/workspaces/${workspace}/applications`, {
          application,
          ttl_seconds: 3600,
          idempotency_key: key({ application, sequence }),
        });
      }
      if (action === "stop")
        return request(
          `/workspaces/${workspace}/services/${selected!.id}/revoke`,
          {},
        );
      const next = await connect(selected!.id, false);
      popup.current?.location.replace(next);
    },
    onSuccess: async (_, action) => {
      if (action === "start" || action === "restart" || action === "desktop") {
        setSequence((v) => v + 1);
        setSettings(false);
      }
      await client.invalidateQueries({ queryKey });
    },
    onError: (_, action) => {
      if (action === "open") popup.current?.close();
    },
  });
  return (
    <aside className="work-visual-pane" aria-label="Preview workspace">
      {!reviewMode && (
        <div className="visual-pane-heading">
          <div className="visual-tabs" role="tablist" aria-label="Preview mode">
            {(
              [
                { id: "web", text: "Web", icon: Globe },
                { id: "desktop", text: "Desktop", icon: Monitor },
              ] as const
            ).map((tab) => (
              <button
                key={tab.id}
                role="tab"
                aria-selected={mode === tab.id}
                onClick={() => {
                  if (tab.id !== mode) {
                    setMode(tab.id);
                    setRefresh((v) => v + 1);
                  }
                }}
              >
                <tab.icon size={14} />
                {tab.text}
              </button>
            ))}
          </div>
          {collapsible && (
            <Button
              size="icon-sm"
              variant="ghost"
              aria-label="Collapse preview"
              title="Collapse preview"
              onClick={onCollapse}
            >
              <PanelRightClose size={14} />
            </Button>
          )}
        </div>
      )}
      <>
        <div className="visual-toolbar">
          {reviewMode && (
            <select
              className="review-preview-mode"
              aria-label="Preview mode"
              value={mode}
              onChange={(e) => {
                setMode(e.target.value as "web" | "desktop");
                setRefresh((v) => v + 1);
              }}
            >
              <option value="web">Web app</option>
              <option value="desktop">Desktop</option>
            </select>
          )}
          <select
            aria-label="Preview application"
            value={selected?.id || ""}
            onChange={(e) => {
              setSelectedID(e.target.value);
              setRefresh((v) => v + 1);
            }}
          >
            <option value="" disabled>
              {mode === "desktop" ? "Workspace desktop" : "Application preview"}
            </option>
            {active.map((a) => (
              <option key={a.id} value={a.id}>
                {mode === "desktop" ? "Linux desktop" : a.application.directory}{" "}
                · {a.application.port}
              </option>
            ))}
          </select>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="Reload preview"
            disabled={!selected?.ready}
            onClick={() => setRefresh((v) => v + 1)}
          >
            <RefreshCw size={14} />
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label="Preview options"
              >
                <MoreHorizontal size={16} />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent
              align="end"
              className="preview-options-menu"
              onCloseAutoFocus={(event) => {
                if (selectionMenuClosing.current) {
                  event.preventDefault();
                  selectionMenuClosing.current = false;
                }
              }}
            >
              {mode === "web" && (
                <>
                  <DropdownMenuItem asChild>
                    <Button
                      variant="ghost"
                      size="sm"
                      aria-label={phone ? "Desktop viewport" : "Phone viewport"}
                      aria-pressed={phone}
                      onClick={() => setPhone(!phone)}
                    >
                      {phone ? <Monitor size={14} /> : <Smartphone size={14} />}{" "}
                      {phone ? "Desktop viewport" : "Phone viewport"}
                    </Button>
                  </DropdownMenuItem>
                  <DropdownMenuItem asChild>
                    <Button
                      variant="ghost"
                      size="sm"
                      aria-label="Select element to change"
                      title="Select an element, then describe your change in chat"
                      aria-pressed={inspecting}
                      disabled={!url}
                      onClick={() => {
                        selectionMenuClosing.current = true;
                        nonce.current = crypto.randomUUID();
                        frame.current?.contentWindow?.postMessage(
                          {
                            type: "workbench-select",
                            nonce: nonce.current,
                            enabled: !inspecting,
                          },
                          new URL(url).origin,
                        );
                        setInspecting(!inspecting);
                      }}
                    >
                      <MousePointer2 size={14} /> Select element to change
                    </Button>
                  </DropdownMenuItem>
                </>
              )}
              <DropdownMenuItem asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label="Open preview in new tab"
                  disabled={!selected?.ready || mutation.isPending}
                  onClick={() => {
                    popup.current = window.open("about:blank", "_blank");
                    if (popup.current) popup.current.opener = null;
                    mutation.mutate("open");
                  }}
                >
                  <ExternalLink size={14} /> Open preview in new tab
                </Button>
              </DropdownMenuItem>
              <DropdownMenuItem asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label="Application logs"
                  title="Application logs"
                  disabled={!apps.length}
                  onClick={() => setLogs(true)}
                >
                  <Terminal size={14} /> Application logs
                </Button>
              </DropdownMenuItem>
              <DropdownMenuItem asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label="Preview settings and logs"
                  onClick={() => {
                    if (selected && mode === "web") {
                      setDirectory(selected.application.directory);
                      setCommand(selected.application.command);
                      setPort(String(selected.application.port));
                    }
                    setSettings(true);
                  }}
                >
                  <Settings2 size={14} /> Preview settings
                </Button>
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
        {query.isError && <ErrorNotice error={query.error} />}
        {mode === "web" && detected.isError && (
          <p className="visual-hint">
            Port detection is unavailable. The workspace runtime may need
            updating.
          </p>
        )}
        {forward.isError && (
          <div className="visual-hint">
            <ErrorNotice error={forward.error} />
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                tried.current.clear();
                forward.reset();
                detected.refetch();
              }}
            >
              Retry detection
            </Button>
          </div>
        )}
        {mutation.isError && !settings && (
          <ErrorNotice error={mutation.error} />
        )}
        {inspecting && (
          <p className="visual-inspecting">
            Click an element to describe a change in the conversation.
          </p>
        )}
        {selected ? (
          <div
            className={`visual-browser ${phone && mode === "web" ? "is-phone" : ""}`}
          >
            {selected.ready ? (
              <ConnectedPreview
                key={`${selected.id}:${refresh}:${mode}:${mode === "web" ? completedRevision : ""}`}
                project={project}
                workspace={workspace}
                service={selected.id}
                title={
                  mode === "desktop"
                    ? "Workspace desktop"
                    : "Application preview"
                }
                frame={frame}
                connect={connect}
                onConnection={setURL}
              />
            ) : (
              <div className="visual-empty" role="status">
                <RefreshCw size={22} className="animate-spin" />
                <p>Starting application…</p>
              </div>
            )}
          </div>
        ) : (
          <div className="visual-empty">
            {mode === "desktop" ? <Monitor size={32} /> : <Globe size={32} />}
            <h3>
              {mode === "desktop"
                ? "A desktop for this workspace"
                : "See what you’re building"}
            </h3>
            <p>
              {mode === "desktop"
                ? "Run and interact with Linux desktop apps alongside your agent."
                : reviewMode
                  ? "Start the app to review it here. This keeps your review unchanged."
                  : "Ask the agent to start your app in the workspace. Its running HTTP port will appear here automatically."}
            </p>
            <Button
              disabled={
                mutation.isPending || query.isPending || forward.isPending
              }
              onClick={() =>
                mode === "desktop"
                  ? mutation.mutate("desktop")
                  : detected.data?.items[0]
                    ? forward.mutate({
                        ...detected.data.items[0],
                        manual: true,
                      })
                    : reviewMode
                      ? restartable
                        ? mutation.mutate("restart")
                        : (() => {
                            setCommand("");
                            setSettings(true);
                          })()
                      : onSelection(
                          "Start this project’s web application inside the workspace using workbench-preview. Inspect the project to choose its launch command, verify it is ready, and keep it running for the Web preview.\n",
                        )
              }
            >
              <Play size={14} />
              {mode === "desktop"
                ? "Start worker desktop"
                : detected.data?.items.length
                  ? "Open detected app"
                  : reviewMode
                    ? restartable
                      ? "Start preview"
                      : "Set up preview"
                    : "Ask agent to run app"}
            </Button>
            {apps[0] && <small>Previous session: {apps[0].state}</small>}
          </div>
        )}
      </>
      {logs && (
        <ApplicationLogs
          applications={apps}
          initialID={selected?.id || apps[0]?.id || ""}
          error={query.isError ? query.error : undefined}
          onClose={() => setLogs(false)}
          onDebug={(message) => {
            setLogs(false);
            onSelection(message);
          }}
        />
      )}
      {settings && (
        <Modal
          title={
            mode === "desktop" ? "Desktop session" : "Application settings"
          }
          onClose={() => setSettings(false)}
        >
          {selected && (
            <div className="visual-session-info">
              <span>
                Port {selected.application.port} ·{" "}
                {selected.ready ? "Ready" : "Starting"}
              </span>
              <Button
                variant="outline"
                size="sm"
                disabled={mutation.isPending}
                onClick={() => mutation.mutate("stop")}
              >
                {selected.application.kind === "forward"
                  ? "Disconnect preview"
                  : "Stop app"}
              </Button>
            </div>
          )}
          {mode === "web" && selected?.application.kind !== "forward" && (
            <Form onSubmit={() => mutation.mutate("start")}>
              <label>
                Application directory
                <Input
                  value={directory}
                  onChange={(e) => setDirectory(e.target.value)}
                  required
                />
              </label>
              <label>
                Start command
                <Textarea
                  rows={3}
                  value={command}
                  onChange={(e) => setCommand(e.target.value)}
                  required
                />
              </label>
              <label>
                Port
                <Input
                  type="number"
                  min="1024"
                  max="65535"
                  value={port}
                  onChange={(e) => setPort(e.target.value)}
                  required
                />
              </label>
              <p className="visual-hint">
                Use $PORT in your command. Stop an existing app before reusing
                its port. Sessions run for one hour.
              </p>
              <Button
                disabled={
                  mutation.isPending || !directory.trim() || !command.trim()
                }
              >
                Start preview
              </Button>
            </Form>
          )}
          {mode === "desktop" && (
            <p className="visual-hint">
              The agent can launch GUI apps on DISPLAY=:99 while this session is
              active. The desktop runs inside this workspace.
            </p>
          )}
          <Button
            variant="outline"
            disabled={!apps.length}
            onClick={() => {
              setSettings(false);
              setLogs(true);
            }}
          >
            <Terminal size={14} />
            Open application logs
          </Button>
          {mutation.isError && <ErrorNotice error={mutation.error} />}
        </Modal>
      )}
    </aside>
  );
}

// Connection tickets are single-use. Each iframe mount needs its own ticket,
// including when application readiness changes or the user reopens this view.
function ConnectedPreview({
  project,
  workspace,
  service,
  title,
  frame,
  connect,
  onConnection,
}: {
  project: string;
  workspace: string;
  service: string;
  title: string;
  frame: RefObject<HTMLIFrameElement | null>;
  connect: (
    id: string,
    embedded: boolean,
    signal?: AbortSignal,
  ) => Promise<string>;
  onConnection: (url: string) => void;
}) {
  const [instance] = useState(() => crypto.randomUUID());
  const connection = useQuery({
    ...privateContentOptions,
    queryKey: [
      "project",
      project,
      "preview-connection",
      workspace,
      service,
      instance,
    ],
    queryFn: ({ signal }) => connect(service, true, signal),
    staleTime: Infinity,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  const url = connection.isError ? "" : connection.data || "";
  useEffect(() => {
    onConnection(url);
    return () => onConnection("");
  }, [url, onConnection]);
  if (connection.isError)
    return (
      <div className="visual-empty">
        <ErrorNotice error={connection.error} />
        <Button variant="outline" onClick={() => connection.refetch()}>
          Reconnect preview
        </Button>
      </div>
    );
  if (!url)
    return (
      <div className="visual-empty" role="status">
        <RefreshCw size={22} className="animate-spin" />
        <p>Connecting preview…</p>
      </div>
    );
  return (
    <iframe
      ref={frame}
      src={url}
      title={title}
      sandbox="allow-scripts allow-same-origin allow-forms allow-downloads allow-modals allow-pointer-lock"
      allow="fullscreen"
    />
  );
}
