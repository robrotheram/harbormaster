import { Fingerprint } from "lucide-react";

import { useQuery } from "@tanstack/react-query";
import { keys } from "@/shared/query-keys";

import { workspacesApi } from "@/features/workspaces/api";

import { date, label } from "@/shared/format";
import { Empty, ErrorNotice, Observation } from "@/shared/components";
import type { Project } from "@/shared/types";

export function AgentHomes({ project }: { project: Project }) {
  const homes = useQuery({
    queryKey: keys.homes(project.id),
    queryFn: ({ signal }) => workspacesApi.homes(project.id, signal),
    refetchInterval: 15000,
  });
  const workspaces = useQuery({
    queryKey: keys.workspaces(project.id),
    queryFn: ({ signal }) => workspacesApi.list(project.id, signal),
  });
  return (
    <>
      <div className="page-title">
        <div>
          <h1>Agent homes</h1>
          <p>
            Persistent identities and private state, independent of running
            compute.
          </p>
        </div>
      </div>
      <div className="list-toolbar">
        <span>{homes.data?.length ?? "—"} agent homes</span>
        <Observation
          at={homes.dataUpdatedAt}
          fetching={homes.isFetching}
          error={homes.isError}
        />
      </div>
      {homes.isError && <ErrorNotice error={homes.error} />}
      {homes.isPending ? (
        <div className="loading" role="status">
          Loading agent identities…
        </div>
      ) : homes.data?.length ? (
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                <th>Home</th>
                <th>Workspace</th>
                <th>Agent</th>
                <th>Created</th>
              </tr>
            </thead>
            <tbody>
              {homes.data.map((home) => (
                <tr key={home.id}>
                  <td>
                    <Fingerprint className="mr-2 inline size-3.5 text-muted-foreground" />
                    <span className="identifier prominent">{home.id}</span>
                  </td>
                  <td>
                    {workspaces.data?.find((w) => w.id === home.workspace_id)
                      ?.name || home.workspace_id}
                  </td>
                  <td>
                    {home.kind === "t3" ? "T3 with Codex" : label(home.kind)}
                  </td>
                  <td>{date(home.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : homes.data ? (
        <Empty title="Homes are created with workspaces">
          Create a workspace to give its agent a persistent home in this
          project.
        </Empty>
      ) : null}
      <div className="explanation">
        <h2>Private by default</h2>
        <p>
          A home belongs to one workspace in {project.name}. Its conversations,
          memory, credentials and tool state stay separate. Branch workspaces
          inherit project files through an explicit copy and receive their own
          home.
        </p>
      </div>
    </>
  );
}
