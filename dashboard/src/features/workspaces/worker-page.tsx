import { WorkerDatasets } from "@/features/analysis/worker-datasets";
import { lazy, Suspense, useCallback, useState } from "react";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, ExternalLink } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Empty, ErrorNotice, State } from "@/shared/components";
import { keys } from "@/shared/query-keys";
import { accessBlocked } from "@/shared/query-policy";
import type { Pairing, Project, Workspace } from "@/shared/types";
import { WorkspaceHistory } from "@/features/history/workspace-history";
import { WorkerAgentActivity } from "./worker-agent-activity";
import { WorkspaceLogs } from "@/features/history/workspace-logs";
import { workspaceOptions, workspacesApi } from "./api";
import { consoleURL, safePairing } from "./connections";
import { DeleteWorker } from "./delete-worker";
import { PairingPanel } from "./pairing-panel";
import { WorkspaceSettings } from "./settings";
import { StartWorker } from "./start-worker";
import { WorkspaceForm } from "./workspace-form";
import { WorkerSetup } from "./worker-setup";

const WorkerTerminal = lazy(() =>
  import("./worker-terminal").then((module) => ({
    default: module.WorkerTerminal,
  })),
);

const workerRoute = "/projects/$projectId/workspaces/$workerId";
const listRoute = "/projects/$projectId/$section";
type Panel = "environment" | "datasets" | "settings" | "activity";
const panels = [
  [undefined, "Terminal"],
  ["environment", "Environment"],
  ["datasets", "Datasets"],
  ["settings", "Settings"],
  ["activity", "Activity"],
] as const;

export function WorkerPage({
  project,
  workerID,
  panel,
}: {
  project: Project;
  workerID: string;
  panel?: Panel;
}) {
  const query = useQuery(workspaceOptions(project.id));
  const worker = accessBlocked(query.error)
    ? undefined
    : query.data?.find((entry) => entry.id === workerID);
  return (
    <>
      {query.isError && (
        <ErrorNotice
          error={query.error}
          onRetry={() => void query.refetch()}
          retrying={query.isFetching}
        />
      )}
      {worker ? (
        <WorkerDetail project={project} worker={worker} panel={panel} />
      ) : query.isPending ? (
        <p role="status" className="text-sm text-muted-foreground">
          Loading worker…
        </p>
      ) : !query.isError ? (
        <Empty title="Worker unavailable">
          This worker may have been removed or moved to another project.
        </Empty>
      ) : null}
    </>
  );
}

function WorkerDetail({
  project,
  worker,
  panel,
}: {
  project: Project;
  worker: Workspace;
  panel?: Panel;
}) {
  const client = useQueryClient();
  const navigate = useNavigate();
  const { tab } = useSearch({ strict: false });
  const params = { projectId: project.id, workerId: worker.id };
  const [dialog, setDialog] = useState<"start" | "branch" | "delete">();
  const [historyOpen, setHistoryOpen] = useState(false);
  const [pairing, setPairing] = useState<Pairing>();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState<unknown>();
  const refresh = useCallback(
    () => client.invalidateQueries({ queryKey: keys.project(project.id) }),
    [client, project.id],
  );
  const openSettings = () => {
    setDialog(undefined);
    void navigate({ to: workerRoute, params, search: { panel: "settings" } });
  };
  const consoleEnabled = worker.console_enabled || worker.agent_kind === "t3";
  const schedulingBlocked = worker.startup_reason === "WORKSPACE_UNSCHEDULABLE";
  const starting = worker.state === "STARTING" && !schedulingBlocked;
  const transitioning = starting || worker.state === "STOPPING";
  const name = worker.name || worker.id;

  async function operate(action: "stop" | "open" | "pair" | "enable") {
    if (busy) return;
    const tab = action === "open" ? window.open("about:blank", "_blank") : null;
    if (tab) tab.opener = null;
    setBusy(true);
    setMessage("");
    setError(undefined);
    try {
      switch (action) {
        case "stop":
          await workspacesApi.stop(worker.id);
          setMessage("Worker stopped.");
          break;
        case "pair":
          setPairing(
            safePairing(
              await workspacesApi.pair(worker.id),
              worker.id,
              location.origin,
            ),
          );
          break;
        case "enable":
          await workspacesApi.enableConsole(worker.id);
          setMessage("T3 enabled.");
          break;
        case "open": {
          const result = await workspacesApi.start(worker.id);
          if (!result.console_ticket)
            throw new Error(
              "The T3 connection is unavailable. Try opening T3 again.",
            );
          const port = import.meta.env.DEV
            ? import.meta.env.VITE_WORKBENCH_PORT || "8080"
            : location.port;
          const url = consoleURL(
            worker.id,
            result.console_ticket,
            port,
            location.origin,
          );
          if (tab) tab.location.replace(url);
          else setMessage("Allow popups to open T3.");
          break;
        }
      }
    } catch (reason) {
      tab?.close();
      setError(reason);
    } finally {
      setBusy(false);
      refresh();
    }
  }

  return (
    <div className="min-w-0 space-y-4">
      <header className="flex flex-wrap items-center justify-between gap-x-4 gap-y-3 py-1">
        <div className="flex min-w-0 items-start gap-2 sm:items-center sm:gap-3">
          <Button asChild size="icon-sm" variant="ghost" className="shrink-0">
            <Link
              to={listRoute}
              params={{ projectId: project.id, section: "workspaces" }}
              aria-label="Back to Workers"
              title="Back to Workers"
            >
              <ArrowLeft aria-hidden="true" />
            </Link>
          </Button>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="break-words text-xl font-semibold leading-tight [overflow-wrap:anywhere]">
                {name}
              </h1>
              <State value={schedulingBlocked ? "BLOCKED" : worker.state} />
            </div>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {worker.ready && consoleEnabled && (
            <Button
              variant="outline"
              disabled={busy || transitioning}
              onClick={() => void operate("open")}
            >
              <ExternalLink aria-hidden="true" /> Open T3
            </Button>
          )}
          {worker.ready ? (
            <Button
              variant="outline"
              disabled={busy || worker.state !== "READY"}
              onClick={() => void operate("stop")}
            >
              Stop worker
            </Button>
          ) : (
            <Button
              disabled={busy || transitioning}
              onClick={() => setDialog("start")}
            >
              {starting
                ? "Starting…"
                : worker.state === "STOPPING"
                  ? "Stopping…"
                  : "Check & start"}
            </Button>
          )}
        </div>
      </header>
      <nav aria-label="Worker sections" className="settings-tabs">
        {panels.map(([value, label]) => (
          <Link
            key={label}
            to={workerRoute}
            params={params}
            search={{ panel: value }}
            aria-current={panel === value ? "page" : undefined}
            activeOptions={{
              exact: true,
              includeSearch: true,
              explicitUndefined: true,
            }}
          >
            {label}
          </Link>
        ))}
      </nav>
      {schedulingBlocked && (
        <p role="status" className="notice">
          Kubernetes cannot schedule this worker. Stop an idle worker to free
          capacity or contact your platform administrator.
        </p>
      )}
      {error != null && <ErrorNotice error={error} />}
      {message && (
        <p role="status" className="notice">
          {message}
        </p>
      )}
      {!panel &&
        (worker.ready ? (
          <Suspense fallback={<p role="status">Connecting to terminal…</p>}>
            <WorkerTerminal workspace={worker} onSessionClosed={refresh} />
          </Suspense>
        ) : (
          <section
            aria-label="Worker terminal"
            className="rounded-md border bg-card p-5 text-sm text-muted-foreground"
          >
            {starting ? (
              <p role="status">Waiting for the worker to become ready…</p>
            ) : (
              "Start the worker to connect to its terminal."
            )}
          </section>
        ))}
      {panel === "datasets" && (
        <WorkerDatasets key={worker.id} worker={worker} project={project.id} />
      )}
      {panel === "environment" && (
        <WorkerSetup workspace={worker} onSettings={openSettings} />
      )}
      {panel === "settings" && (
        <div className="min-w-0 space-y-4">
          <WorkspaceSettings
            key={tab}
            initialSection={tab === "subscriptions" ? tab : "worker"}
            workspace={worker}
            onSaved={() => {
              refresh();
              setMessage("Worker settings saved.");
            }}
          />
          <section className="settings-panel" aria-labelledby="worker-controls">
            <h2 id="worker-controls" className="settings-panel-title">
              Worker controls
            </h2>
            <p className="break-all font-mono text-xs text-muted-foreground">
              {worker.id}
            </p>
            <div className="flex flex-wrap gap-2">
              {worker.ready && consoleEnabled && (
                <Button
                  variant="outline"
                  disabled={busy}
                  onClick={() => void operate("pair")}
                >
                  Connect T3 desktop
                </Button>
              )}
              {worker.ready &&
                !consoleEnabled &&
                worker.agent_kind === "codex" && (
                  <Button
                    variant="outline"
                    disabled={busy}
                    onClick={() => void operate("enable")}
                  >
                    Enable T3
                  </Button>
                )}
              {!worker.shared && (
                <Button
                  variant="outline"
                  disabled={busy}
                  onClick={() => setDialog("branch")}
                >
                  Branch worker
                </Button>
              )}
              <Button
                variant="outline"
                onClick={() => {
                  void navigator.clipboard
                    .writeText(worker.id)
                    .then(() => setMessage("Worker ID copied."), setError);
                }}
              >
                Copy worker ID
              </Button>
            </div>
          </section>
          {!worker.shared && (
            <section className="settings-panel" aria-labelledby="remove-worker">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <h2 id="remove-worker" className="settings-panel-title">
                    Remove worker
                  </h2>
                  <p className="mt-1 text-sm text-muted-foreground">
                    Remove the worker from the project. Private disks are
                    retained.
                  </p>
                </div>
                <Button
                  variant="outline"
                  className="text-destructive"
                  disabled={busy}
                  onClick={() => setDialog("delete")}
                >
                  Delete worker
                </Button>
              </div>
            </section>
          )}
        </div>
      )}
      {panel === "activity" && (
        <section
          aria-label={historyOpen ? "Message history" : "Activity logs"}
          className="min-w-0 space-y-4"
        >
          <WorkerAgentActivity
            projectID={project.id}
            worker={worker}
            watchingHistory={historyOpen}
          />
          <div className="flex flex-wrap items-center justify-between gap-3 border-b pb-3">
            <div
              className="flex gap-1"
              aria-label="Activity views"
              role="group"
            >
              <Button
                size="sm"
                variant={historyOpen ? "ghost" : "secondary"}
                aria-pressed={!historyOpen}
                onClick={() => setHistoryOpen(false)}
              >
                Activity logs
              </Button>
              <Button
                size="sm"
                variant={historyOpen ? "secondary" : "ghost"}
                aria-pressed={historyOpen}
                onClick={() => setHistoryOpen(true)}
              >
                Message history
              </Button>
            </div>
          </div>
          {historyOpen ? (
            <WorkspaceHistory
              project={project}
              workspace={worker}
              workspaces={[worker]}
            />
          ) : (
            <WorkspaceLogs
              project={project}
              workspace={worker}
              workspaces={[worker]}
            />
          )}
        </section>
      )}
      {dialog === "start" && (
        <StartWorker
          workspace={worker}
          onClose={() => setDialog(undefined)}
          onSettings={openSettings}
          onStartRequested={async (ready) => {
            setError(undefined);
            setMessage(ready ? "Worker started." : "");
            await refresh();
            setDialog(undefined);
          }}
        />
      )}
      {dialog === "branch" && (
        <WorkspaceForm
          project={project}
          parent={worker}
          onClose={() => setDialog(undefined)}
          onCreated={() => {
            refresh();
            setDialog(undefined);
            setMessage("Worker branched.");
          }}
        />
      )}
      {dialog === "delete" && (
        <DeleteWorker
          workspace={worker}
          onClose={() => setDialog(undefined)}
          onDeleted={() => {
            setDialog(undefined);
            refresh();
            void navigate({
              to: listRoute,
              params: { projectId: project.id, section: "workspaces" },
              replace: true,
            });
          }}
        />
      )}
      {pairing && (
        <PairingPanel
          workspace={worker}
          value={pairing}
          onClose={() => setPairing(undefined)}
        />
      )}
    </div>
  );
}
