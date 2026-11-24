import { runOptions } from "./api";

import { useQuery } from "@tanstack/react-query";
import { keys } from "@/shared/query-keys";

import { workspacesApi } from "@/features/workspaces/api";
import { schedulingApi } from "@/features/scheduling/api";
import { date, label } from "@/shared/format";
import { Empty, ErrorNotice, Observation, State } from "@/shared/components";
import type { Project } from "@/shared/types";

export function Runs({ project }: { project: Project }) {
  const query = useQuery(runOptions(project.id));
  const schedules = useQuery({
    queryKey: keys.schedules(project.id),
    queryFn: ({ signal }) => schedulingApi.schedules(project.id, signal),
  });
  const workspaces = useQuery({
    queryKey: keys.workspaces(project.id),
    queryFn: ({ signal }) => workspacesApi.list(project.id, signal),
  });
  return (
    <>
      <div className="page-title">
        <div>
          <h1>Scheduled runs</h1>
        </div>
      </div>
      <div className="list-toolbar">
        <span>{query.data?.length ?? "—"} recorded runs</span>
        <Observation
          at={query.dataUpdatedAt}
          fetching={query.isFetching}
          error={query.isError}
        />
      </div>
      {query.isError && <ErrorNotice error={query.error} />}
      {query.isPending ? (
        <div className="loading" role="status">
          Loading run state…
        </div>
      ) : query.data?.length ? (
        <div className="table-scroll record-table">
          <table>
            <thead>
              <tr>
                <th>State</th>
                <th>Schedule / workspace</th>
                <th>Requested</th>
                <th>Last observation</th>
                <th>Details</th>
              </tr>
            </thead>
            <tbody>
              {query.data.map((run) => (
                <tr key={run.id}>
                  <td data-label="State">
                    <State value={run.state} />
                  </td>
                  <td data-label="Schedule / worker">
                    <strong className="block font-medium">
                      {schedules.data?.find((s) => s.id === run.schedule_id)
                        ?.name || "Manual schedule request"}
                    </strong>
                    <span className="mt-1 block text-[11px] text-muted-foreground">
                      {workspaces.data?.find((w) => w.id === run.workspace_id)
                        ?.name || run.workspace_id}
                    </span>
                  </td>
                  <td data-label="Requested">{date(run.created_at)}</td>
                  <td data-label="Last observation">{date(run.updated_at)}</td>
                  <td data-label="Details">
                    {run.error_code ? (
                      <span className="text-warning">
                        {label(run.error_code)}
                      </span>
                    ) : (
                      <span className="identifier">
                        {run.task_id || "Awaiting task acceptance"}
                      </span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : query.data ? (
        <Empty title="No scheduled runs yet">
          Create a schedule or select Run now on an existing one. Native task
          results stay in the workspace.
        </Empty>
      ) : null}
      <details className="page-help">
        <summary>About scheduled runs</summary>
        <p>
          This view shows scheduled dispatches. Interactive T3 turns and tasks
          submitted directly from a workspace have their own native history.
          Prompts, responses and files are not stored in the platform run
          metadata.
        </p>
      </details>
    </>
  );
}
