import { useState } from "react";
import { useQueries, useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  Boxes,
  ArrowUpRight,
  Search,
  RefreshCw,
  Cpu,
  MemoryStick,
  ShieldCheck,
  ListChecks,
  Check,
} from "lucide-react";
import { snapshotOptions } from "./api";
import {
  BulkWorkerAction,
  WorkerSelection,
  WorkerCheckbox,
  type WorkerBatch,
} from "./worker-actions";
import { workspaceOptions } from "@/features/workspaces/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/native-select";
import { ErrorNotice, Modal, State } from "@/shared/components";
import { bytes, date } from "@/shared/format";
import type { Project, Workspace } from "@/shared/types";

export function Infrastructure({ projects }: { projects: Project[] }) {
  const snapshot = useQuery(snapshotOptions());
  const queries = useQueries({
    queries: projects.map((p) => workspaceOptions(p.id)),
  });
  const [project, setProject] = useState("");
  const [runtime, setRuntime] = useState("");
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<string>();
  const [bulkMode, setBulkMode] = useState(false);
  const [checked, setChecked] = useState<Set<string>>(new Set());
  const [batch, setBatch] = useState<WorkerBatch>();
  const workers = queries.flatMap((q) => (q.isError ? [] : q.data || []));
  const pending = queries.some((q) => q.isPending);
  const failures = projects.filter((_, i) => queries[i].isError);
  const runtimes = [
    ...new Set(workers.map((w) => w.runtime_type || "unknown")),
  ].sort();
  const visible = workers.filter(
    (w) =>
      (!project || w.project_id === project) &&
      (!runtime || (w.runtime_type || "unknown") === runtime) &&
      [w.name, w.id, w.agent_kind, w.branch].some((v) =>
        v?.toLowerCase().includes(search.toLowerCase()),
      ),
  );
  const inspected = workers.find((w) => w.id === selected);
  const usage = snapshot.isError ? undefined : snapshot.data?.usage;
  const refresh = () => {
    snapshot.refetch();
    queries.forEach((q) => q.refetch());
  };
  const refreshing = snapshot.isFetching || queries.some((q) => q.isFetching);
  return (
    <>
      <div className="page-title">
        <div>
          <h1>Infrastructure</h1>
        </div>
        <Button
          variant="outline"
          size="sm"
          className="h-8"
          onClick={refresh}
          disabled={refreshing}
        >
          <RefreshCw className={refreshing ? "animate-spin" : ""} />
          Refresh
        </Button>
      </div>
      <div className="infra-resources">
        <span>
          <Boxes size={13} />
          Running{" "}
          <strong>
            {usage?.running_workspaces ?? "—"}
            <em> / {usage?.max_running_workspaces ?? "—"}</em>
          </strong>
        </span>
        <span>
          <Cpu size={13} />
          Allocated CPU{" "}
          <strong>
            {usage?.cpu_allocated == null
              ? "—"
              : `${usage.cpu_allocated} ${usage.cpu_allocated === 1 ? "core" : "cores"}`}
          </strong>
        </span>
        <span>
          <MemoryStick size={13} />
          Allocated memory{" "}
          <strong>
            {usage?.memory_allocated_bytes == null
              ? "—"
              : bytes(usage.memory_allocated_bytes)}
          </strong>
        </span>
      </div>
      {snapshot.isError && <ErrorNotice error={snapshot.error} />}
      {failures.length > 0 && (
        <div className="notice error" role="alert">
          Worker inventory unavailable for{" "}
          {failures.map((p) => p.name).join(", ")}. Counts exclude these
          projects.{" "}
          <button className="underline" onClick={refresh}>
            Retry
          </button>
        </div>
      )}
      <section
        className="worker-inventory"
        aria-label="Global worker inventory"
      >
        <div className="queue-heading">
          <h2>
            <Boxes size={14} />
            Workers
            <span>
              {pending ? "…" : workers.length}
              {failures.length ? "+" : ""}
            </span>
          </h2>
          <span className="text-[11px] text-muted-foreground">
            {failures.length || pending
              ? "Partial inventory"
              : `Across ${projects.length} projects`}
          </span>
        </div>
        <div className="infra-filters">
          <div className="compact-search">
            <Search size={13} />
            <Input
              className="h-8 pl-8 text-xs md:text-xs shadow-none"
              aria-label="Search workers"
              placeholder="Search workers…"
              value={search}
              onChange={(e) => {
                setSearch(e.target.value);
                setChecked(new Set());
              }}
            />
          </div>
          <NativeSelect
            size="sm"
            className="text-xs"
            wrapperClassName="infra-filter-select"
            aria-label="Filter workers by project"
            value={project}
            onChange={(e) => {
              setProject(e.target.value);
              setChecked(new Set());
            }}
          >
            <option value="">All projects</option>
            {projects.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </NativeSelect>
          <NativeSelect
            size="sm"
            className="text-xs"
            wrapperClassName="infra-filter-select"
            aria-label="Filter workers by runtime"
            value={runtime}
            onChange={(e) => {
              setRuntime(e.target.value);
              setChecked(new Set());
            }}
          >
            <option value="">All runtimes</option>
            {runtimes.map((r) => (
              <option value={r} key={r}>
                {runtimeName(r)}
              </option>
            ))}
          </NativeSelect>
          {(project || runtime || search) && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setProject("");
                setRuntime("");
                setSearch("");
                setChecked(new Set());
              }}
            >
              Clear filters
            </Button>
          )}
          <Button
            className="bulk-mode-toggle h-8"
            variant="outline"
            size="sm"
            aria-label={
              bulkMode ? "Done with bulk operations" : "Bulk operations"
            }
            aria-expanded={bulkMode}
            aria-controls={bulkMode ? "worker-selection-controls" : undefined}
            disabled={!bulkMode && !visible.length}
            onClick={() => {
              setBulkMode(!bulkMode);
              setChecked(new Set());
            }}
          >
            {bulkMode ? <Check /> : <ListChecks />}
            {bulkMode ? "Done" : "Bulk operations"}
          </Button>
        </div>
        {bulkMode && (
          <WorkerSelection
            visible={visible}
            checked={checked}
            onChange={setChecked}
            onAction={setBatch}
          />
        )}
        <div className="work-table-wrap">
          <table className="work-table infra-table" data-bulk={bulkMode}>
            <thead>
              <tr>
                {bulkMode && (
                  <th className="worker-select-cell">
                    <span className="sr-only">Select worker</span>
                  </th>
                )}
                <th className="infra-worker-cell">Worker</th>
                <th>Project</th>
                <th>Runtime</th>
                <th>Status</th>
                <th>Activity</th>
                <th>Allocation</th>
                <th>
                  <span className="sr-only">Inspect</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {visible.map((w) => (
                <tr key={w.id} data-selected={bulkMode && checked.has(w.id)}>
                  {bulkMode && (
                    <td className="worker-select-cell">
                      <WorkerCheckbox
                        label={`Select worker ${w.name || w.id}`}
                        checked={checked.has(w.id)}
                        onChange={() =>
                          setChecked((current) => {
                            const next = new Set(current);
                            if (next.has(w.id)) next.delete(w.id);
                            else next.add(w.id);
                            return next;
                          })
                        }
                      />
                    </td>
                  )}
                  <td className="infra-worker-cell">
                    <div className="work-name">
                      <div className="min-w-0">
                        <button
                          onClick={() => setSelected(w.id)}
                          title={w.name || w.id}
                        >
                          {w.name || w.id}
                        </button>
                        <span>
                          {w.agent_kind || "Agent"}
                          {w.branch && ` / ${w.branch}`}
                        </span>
                      </div>
                    </div>
                  </td>
                  <td className="infra-project-cell">
                    <Link
                      to="/projects/$projectId/$section"
                      params={{ projectId: w.project_id, section: "work" }}
                      className="worker-project-link"
                    >
                      {projects.find((p) => p.id === w.project_id)?.name ||
                        w.project_id}
                    </Link>
                  </td>
                  <td className="infra-runtime-cell">
                    <span className="runtime-label">
                      {runtimeName(w.runtime_type)}
                    </span>
                  </td>
                  <td className="infra-status-cell">
                    <State value={w.state} />
                  </td>
                  <td className="infra-activity-cell text-muted-foreground">
                    {w.active_runs < 0 || w.background_jobs < 0
                      ? "Unknown"
                      : `${w.active_runs} runs / ${w.background_jobs} jobs`}
                  </td>
                  <td className="infra-allocation-cell text-muted-foreground">
                    {w.cpu_limit} CPU / {bytes(w.memory_limit_bytes)}
                  </td>
                  <td className="infra-inspect-cell">
                    <button
                      className="icon-link"
                      aria-label={`Inspect worker ${w.name || w.id}`}
                      onClick={() => setSelected(w.id)}
                    >
                      <ArrowUpRight size={14} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {!visible.length && (
          <div className="work-empty">
            <Boxes size={25} />
            <h3>
              {pending
                ? "Loading workers…"
                : workers.length
                  ? "No matching workers"
                  : failures.length
                    ? "Worker inventory unavailable"
                    : "No workers provisioned"}
            </h3>
            <p>
              {workers.length
                ? "Try a different project, runtime, or search."
                : "Create a work item in a project to provision its first worker."}
            </p>
            {!workers.length && !pending && !failures.length && (
              <Link to="/" className="factory-action-link">
                Open projects
                <ArrowUpRight size={14} />
              </Link>
            )}
          </div>
        )}
        <div className="queue-footer">
          <span>
            {visible.length} visible
            {pending
              ? " / loading"
              : failures.length
                ? " / partial inventory"
                : ` / ${workers.length} total`}
          </span>
          <span>Resource values are allocations, not live utilization</span>
        </div>
      </section>
      {batch && (
        <BulkWorkerAction
          batch={batch}
          projects={projects}
          onClose={() => setBatch(undefined)}
          onSucceeded={(id) =>
            setChecked((current) => {
              const next = new Set(current);
              next.delete(id);
              return next;
            })
          }
        />
      )}
      {selected && (
        <Modal title="Worker details" onClose={() => setSelected(undefined)}>
          {inspected ? (
            <WorkerDetails
              worker={inspected}
              project={projects.find((p) => p.id === inspected.project_id)}
            />
          ) : (
            <p>Worker unavailable. Refresh the inventory to check its state.</p>
          )}
        </Modal>
      )}
    </>
  );
}
function runtimeName(runtime?: string) {
  if (runtime === "kata") return "Kata Containers";
  if (runtime === "firecracker") return "Firecracker";
  if (runtime === "docker") return "Docker";
  return runtime || "Unknown";
}
function WorkerDetails({
  worker: w,
  project,
}: {
  worker: Workspace;
  project?: Project;
}) {
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="font-medium break-all">{w.name || w.id}</h2>
        <State value={w.state} />
      </div>
      <dl className="worker-detail-facts">
        {[
          ["Project", project?.name || w.project_id],
          ["Worker ID", w.id],
          ["Agent", w.agent_kind || "Unknown"],
          ["Runtime", runtimeName(w.runtime_type)],
          ["Isolation", w.isolation_status || "Unknown"],
          ["Network", w.network_status || "Unknown"],
          ["CPU allocation", `${w.cpu_limit} cores`],
          ["Memory allocation", bytes(w.memory_limit_bytes)],
          [
            "Observed CPU",
            w.cpu_cores == null ? "Unknown" : `${w.cpu_cores} cores`,
          ],
          ["Observed memory", bytes(w.memory_bytes)],
          ["Updated", date(w.updated_at)],
        ].map(([name, value]) => (
          <div key={name}>
            <dt>{name}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
      <p className="flex items-start gap-2 text-xs leading-5 text-muted-foreground">
        <ShieldCheck size={14} className="shrink-0 mt-0.5" />
        Isolation and networking reflect runtime observations. The runtime name
        alone does not verify either.
      </p>
      <Link
        to="/projects/$projectId/$section"
        params={{ projectId: w.project_id, section: "workspaces" }}
        className="factory-action-link"
      >
        Manage worker & desktop connection
        <ArrowUpRight size={14} />
      </Link>
    </div>
  );
}
