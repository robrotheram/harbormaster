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
        </div>
      </div>
      <div className="list-toolbar">
        <span>
          {homes.data?.length ?? "—"} agent{" "}
          {homes.data?.length === 1 ? "home" : "homes"}
        </span>
        <Observation
          at={homes.dataUpdatedAt}
          fetching={homes.isFetching}
          error={homes.isError}
        />
      </div>
      {homes.isError && (
        <ErrorNotice
          error={homes.error}
          onRetry={() => void homes.refetch()}
          retrying={homes.isFetching}
        />
      )}
      {workspaces.isError && !!homes.data?.length && (
        <div>
          <p className="mb-2 text-sm text-muted-foreground">
            Worker details are unavailable. Home records remain available below,
            using the last available names or worker IDs.
          </p>
          <ErrorNotice
            error={workspaces.error}
            onRetry={() => void workspaces.refetch()}
            retrying={workspaces.isFetching}
          />
        </div>
      )}
      {homes.isPending ? (
        <div className="loading" role="status">
          Loading agent identities…
        </div>
      ) : homes.data?.length ? (
        <div className="table-scroll record-table">
          <table>
            <thead>
              <tr>
                <th>Worker</th>
                <th>Home ID</th>
                <th>Agent</th>
                <th>Created</th>
              </tr>
            </thead>
            <tbody>
              {homes.data.map((home) => (
                <tr key={home.id}>
                  <td data-label="Worker">
                    <Fingerprint className="mr-2 inline size-3.5 text-muted-foreground" />
                    {workspaces.data?.find((w) => w.id === home.workspace_id)
                      ?.name || home.workspace_id}
                  </td>
                  <td data-label="Home ID">
                    <span className="identifier">{home.id}</span>
                  </td>
                  <td data-label="Agent">
                    {home.kind === "t3" ? "T3 with Codex" : label(home.kind)}
                  </td>
                  <td data-label="Created">{date(home.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : homes.data ? (
        <Empty title="Homes are created with workers">
          Create a worker to give its agent a persistent home in this project.
        </Empty>
      ) : null}
      <details className="page-help">
        <summary>How agent homes work</summary>
        <p>
          {homes.data?.some((home) => home.shared) ? (
            <>
              All projects use this same agent home. Conversations, memory and
              tool state are shared with the workspace.
            </>
          ) : (
            <>
              A home belongs to one worker in {project.name}. Its conversations,
              memory, credentials and tool state stay separate. Branched workers
              inherit project files through an explicit copy and receive their
              own home.
            </>
          )}
        </p>
      </details>
    </>
  );
}
