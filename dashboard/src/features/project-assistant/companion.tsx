import { lazy, Suspense, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { Tabs } from "radix-ui";
import {
  Bot,
  ArrowUpRight,
  Play,
  Settings2,
  Terminal,
  Boxes,
} from "lucide-react";
import { assistantApi, assistantKey, assistantOptions } from "./api";
import { workspaceOptions } from "@/features/workspaces/api";
import { TaskPanel } from "@/features/tasks/task-panel";
import { Button } from "@/components/ui/button";
import { ErrorNotice, State, useRequestKey } from "@/shared/components";
import { keys } from "@/shared/query-keys";
import type { Project, Workspace } from "@/shared/types";

const HermesSettings = lazy(() =>
  import("./hermes").then(({ HermesSettings }) => ({
    default: HermesSettings,
  })),
);

export function ProjectCompanion({ project }: { project: Project }) {
  const { tab } = useSearch({ from: "/projects/$projectId/$section" });
  const navigate = useNavigate();
  return (
    <div className="space-y-6">
      <header className="page-title">
        <h1>Project assistant</h1>
      </header>
      <Tabs.Root
        value={tab === "hermes" ? "hermes" : "overview"}
        onValueChange={(value) =>
          void navigate({
            to: "/projects/$projectId/$section",
            params: { projectId: project.id, section: "assistant" },
            search: { tab: value === "hermes" ? "hermes" : undefined },
          })
        }
      >
        <Tabs.List aria-label="Project assistant" className="settings-tabs">
          <Tabs.Trigger value="overview">Overview</Tabs.Trigger>
          <Tabs.Trigger value="hermes">Hermes setup</Tabs.Trigger>
        </Tabs.List>
        <Tabs.Content
          value="overview"
          className="min-w-0 pt-6 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <AssistantOverview project={project} />
        </Tabs.Content>
        <Tabs.Content
          value="hermes"
          className="min-w-0 pt-6 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Suspense fallback={<p role="status">Loading Hermes setup…</p>}>
            <HermesSettings project={project} />
          </Suspense>
        </Tabs.Content>
      </Tabs.Root>
    </div>
  );
}

function AssistantOverview({ project }: { project: Project }) {
  const assistant = useQuery(assistantOptions(project.id));
  const workers = useQuery(workspaceOptions(project.id));
  const client = useQueryClient(),
    requestKey = useRequestKey();
  const [task, setTask] = useState<Workspace>();
  const config = assistant.isError ? undefined : assistant.data;
  const hermes = workers.data?.find((w) => w.id === config?.workspace_id);
  const start = useMutation({
    mutationFn: () =>
      assistantApi.start(project.id, requestKey({ project: project.id })),
    onSuccess: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey: assistantKey(project.id) }),
        client.invalidateQueries({ queryKey: keys.workspaces(project.id) }),
      ]);
    },
  });
  const workerList =
    workers.data?.filter((w) => w.agent_kind !== "hermes") || [];
  return (
    <>
      <div className="project-companion">
        <section className="companion-section">
          <div className="companion-heading">
            <span className="hermes-symbol">
              <Bot size={19} />
            </span>
            <h2>Hermes</h2>
            <Link
              to="/projects/$projectId/$section"
              params={{ projectId: project.id, section: "assistant" }}
              search={{ tab: "hermes" }}
              aria-label="Configure Hermes"
              className="ml-auto icon-link"
            >
              <Settings2 size={15} />
            </Link>
          </div>
          <p className="companion-description">
            Plan the next change, investigate a problem, or ask about this
            project.
          </p>
          {assistant.isError ? (
            <>
              <p
                className="text-xs text-amber-600 dark:text-amber-400"
                role="status"
              >
                Hermes status unavailable.
              </p>
              <Button
                variant="outline"
                size="sm"
                onClick={() => assistant.refetch()}
              >
                Retry status
              </Button>
            </>
          ) : assistant.isPending ? (
            <p className="text-xs text-muted-foreground">Checking Hermes…</p>
          ) : (
            config && (
              <>
                <div className="companion-facts">
                  <span>Status</span>
                  <span>
                    {config.workspace_ready
                      ? "Ready"
                      : config.saved
                        ? config.runtime_available
                          ? "Not running"
                          : "Runtime unavailable"
                        : "Setup needed"}
                  </span>
                  <span>Model</span>
                  <span className="truncate" title={config.config.model}>
                    {config.config.model || "Not configured"}
                  </span>
                  <span>Context</span>
                  <span className="truncate" title={project.name}>
                    {project.name}
                  </span>
                </div>
                {config.workspace_ready && hermes?.ready && !workers.isError ? (
                  <Button className="w-full" onClick={() => setTask(hermes)}>
                    <Terminal size={14} />
                    Work with Hermes
                  </Button>
                ) : config.saved &&
                  config.runtime_available &&
                  config.config.mode !== "matrix" ? (
                  <Button
                    className="w-full"
                    variant="outline"
                    disabled={
                      start.isPending || assistant.isFetching || workers.isError
                    }
                    onClick={() => start.mutate()}
                  >
                    <Play size={14} />
                    {start.isPending ? "Starting Hermes…" : "Start Hermes"}
                  </Button>
                ) : (
                  <Link
                    className="factory-action-link"
                    to="/projects/$projectId/$section"
                    params={{ projectId: project.id, section: "assistant" }}
                    search={{ tab: "hermes" }}
                  >
                    Set up Hermes
                    <ArrowUpRight size={14} />
                  </Link>
                )}
              </>
            )
          )}
          {start.isError && <ErrorNotice error={start.error} />}
          <p className="companion-footnote">
            Private memory and files stay in this project.
          </p>
        </section>
        <section className="companion-section">
          <div className="section-heading">
            <h2>
              <Boxes size={14} />
              Project workers
            </h2>
            <Link
              to="/projects/$projectId/$section"
              params={{ projectId: project.id, section: "workspaces" }}
              aria-label="Manage project workers"
            >
              <ArrowUpRight size={15} />
            </Link>
          </div>
          {workers.isError ? (
            <p className="text-xs text-amber-600 dark:text-amber-400">
              Worker status unavailable.
            </p>
          ) : workers.isPending ? (
            <p className="text-xs text-muted-foreground">Loading workers…</p>
          ) : !workerList.length ? (
            <p className="text-xs leading-5 text-muted-foreground">
              Your first work item can create a worker automatically.
            </p>
          ) : (
            <div className="companion-workers">
              {workerList.slice(0, 4).map((w) => (
                <div key={w.id}>
                  <span className="worker-mini-icon">
                    <Terminal size={13} />
                  </span>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-xs" title={w.name || w.id}>
                      {w.name || w.id}
                    </p>
                    <span className="text-[10px] text-muted-foreground">
                      {w.agent_kind || "Agent"} /{" "}
                      {w.runtime_type || "Runtime unknown"}
                    </span>
                  </div>
                  <State value={w.state} />
                </div>
              ))}
            </div>
          )}
          {workerList.length > 4 && (
            <Link
              className="text-xs text-muted-foreground underline"
              to="/projects/$projectId/$section"
              params={{ projectId: project.id, section: "workspaces" }}
            >
              View all {workerList.length} workers
            </Link>
          )}
        </section>
        <section className="companion-section execution-note">
          <h2>From desktop to delivery</h2>
          <p>T3 Desktop → control plane → workers</p>
          <span>
            Submit work through the control plane. Review the retained results
            here.
          </span>
          <Link to="/infrastructure">
            Inspect infrastructure
            <ArrowUpRight size={12} />
          </Link>
        </section>
      </div>
      {task && (
        <TaskPanel
          project={project}
          workspace={task}
          onClose={() => setTask(undefined)}
        />
      )}
    </>
  );
}
