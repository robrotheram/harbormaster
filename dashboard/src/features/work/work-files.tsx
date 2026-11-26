import "./work-files.css";
import { useMemo, useState } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { ChevronDown, FileText, Files } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { ErrorNotice } from "@/shared/components";
import { privateContentOptions } from "@/shared/private-content";
import { AgentReview } from "@/features/delivery/agent-review";
import { RecordedChanges } from "@/features/delivery/review-viewer";
import { workApi, type Attempt, type WorkItem } from "./api";
import { activeAttempt } from "./conversation-api";
import type { LineReference } from "./file-code";
import { FileInspector, type ReviewFile } from "./file-inspector";
import { WorkspaceFile } from "./workspace-file";
import { filePatches } from "./file-diff";
import type { ReplyOutput } from "./reply-outputs";

const statusLabels: Record<string, string> = {
  A: "Added",
  D: "Deleted",
  M: "Modified",
  T: "Type changed",
};

export function WorkFiles({
  project,
  item,
  attempt,
  attempts,
  pinnedAttempt,
  sources,
  canReview,
  canRequest,
  onAsk,
}: {
  project: string;
  item: WorkItem;
  attempt?: Attempt;
  attempts: Attempt[];
  pinnedAttempt?: string;
  sources: ReplyOutput[];
  canReview: boolean;
  canRequest: boolean;
  onAsk: (message: string) => void;
}) {
  const completedRuns = attempts
    .filter(
      (run) =>
        run.task_id &&
        !activeAttempt(run.state) &&
        run.workspace_id === item.workspace_id,
    )
    .reverse();
  const completedRun = completedRuns[0]?.id;
  const workspace = useQuery({
    ...privateContentOptions,
    queryKey: [
      "project",
      project,
      "workspace-files",
      item.id,
      item.workspace_id,
      completedRun,
    ],
    queryFn: ({ signal }) => workApi.files(project, item.id, signal),
    enabled: !!item.workspace_id,
    placeholderData: keepPreviousData,
  });
  const running = !!pinnedAttempt && !!attempt && activeAttempt(attempt.state);
  const candidates = pinnedAttempt
    ? attempt?.task_id && !running
      ? [attempt]
      : []
    : completedRuns;
  const changes = useQuery({
    ...privateContentOptions,
    queryKey: [
      "project",
      project,
      "work-file-changes",
      item.id,
      item.workspace_id,
      pinnedAttempt,
      candidates.map((run) => run.id),
    ],
    queryFn: async ({ signal }) => {
      for (const run of candidates) {
        const captured = await workApi.changes(
          project,
          item.id,
          run.id,
          signal,
        );
        if (pinnedAttempt || captured?.files.length) return { run, captured };
      }
      return null;
    },
    enabled: candidates.length > 0,
    placeholderData: (previous, query) =>
      !pinnedAttempt && query?.queryKey[5] === undefined ? previous : undefined,
  });
  const captured = changes.data?.captured;
  const bothListsFailed = !running && changes.isError && workspace.isError;
  const diffRun = changes.data?.run;
  const [filter, setFilter] = useState("");
  const [selection, setSelection] = useState<{ path: string; group: string }>();
  const [panel, setPanel] = useState<"files" | "edits" | "agent" | "details">(
    "files",
  );
  const term = filter.trim().toLowerCase();
  const groups = useMemo(() => {
    const files = new Map<string, ReviewFile>();
    for (const file of captured?.files ?? []) files.set(file.path, { ...file });
    for (const artifact of attempt?.artifacts ?? []) {
      const path =
        artifact.source_path || artifact.name || artifact.kind || "Output";
      files.set(path, { ...files.get(path), path, artifact });
    }
    for (const { path } of sources) {
      if (path) files.set(path, { ...files.get(path), path, linked: true });
    }
    const all = [...files.values()];
    return [
      { title: "Changed files", files: all.filter((file) => file.status) },
      {
        title: "Workspace files",
        files: (workspace.data?.paths ?? []).map((path): ReviewFile => ({
          path,
        })),
      },
      {
        title: "Saved files",
        files: all.filter((file) => !file.status && file.artifact),
      },
      {
        title: "From this reply",
        files: all.filter((file) => !file.status && !file.artifact),
      },
    ];
  }, [captured, workspace.data, attempt?.artifacts, sources]);
  const patches = useMemo(
    () =>
      filePatches(
        captured?.diff ?? "",
        captured?.files.map((file) => file.path) ?? [],
      ),
    [captured],
  );
  const matches = groups.map((group) => ({
    ...group,
    files: group.files.filter((file) => file.path.toLowerCase().includes(term)),
  }));
  const visible = matches.flatMap((group) =>
    group.files.map((file) => ({ file, group: group.title })),
  );
  const current =
    visible.find(
      ({ file, group }) =>
        file.path === selection?.path && group === selection.group,
    ) ?? visible[0];
  const selected = current?.file;
  const selectedAttempt = selected?.status ? diffRun : attempt;
  const ask =
    canRequest && selected && selectedAttempt
      ? (reference?: LineReference) => {
          const location = reference
            ? `:${Math.min(reference.start, reference.end)}${reference.start === reference.end ? "" : `-${Math.max(reference.start, reference.end)}`}`
            : "";
          const side =
            selected.status && reference?.side === "before"
              ? ", before this change"
              : "";
          onAsk(
            `Explain ${JSON.stringify(selected.path + location)} (revision ${selectedAttempt.id}${side}): `,
          );
        }
      : undefined;
  const editPanel = panel === "edits" && attempt?.task_id;
  const agentPanel = panel === "agent" && canReview && !!attempt;
  return (
    <section className="work-files" aria-label="Task files">
      <aside className="file-explorer" aria-label="File explorer">
        <div className="file-explorer-heading">
          <h3>Files</h3>
          <Input
            type="search"
            aria-label="Filter files"
            placeholder="Filter files…"
            value={filter}
            onChange={(event) => {
              setFilter(event.target.value);
              setPanel("files");
            }}
          />
        </div>
        <div className="file-explorer-list">
          {bothListsFailed && (
            <ErrorNotice
              error={workspace.error}
              message="Could not load changed files or workspace files."
              onRetry={() =>
                void Promise.all([changes.refetch(), workspace.refetch()])
              }
              retrying={changes.isFetching || workspace.isFetching}
            />
          )}
          {matches.map(
            (group, i) =>
              (i < 2 || group.files.length > 0) && (
                <section key={group.title} aria-label={group.title}>
                  <h4>
                    {group.title} <span>{groups[i].files.length}</span>
                  </h4>
                  {i === 0 &&
                    (running ? (
                      <p>Changes appear after this run finishes.</p>
                    ) : bothListsFailed ? null : changes.isError ? (
                      <ErrorNotice
                        error={changes.error}
                        onRetry={() => void changes.refetch()}
                        retrying={changes.isFetching}
                      />
                    ) : changes.isLoading ? (
                      <p role="status">Loading changes…</p>
                    ) : !pinnedAttempt && !captured?.files.length ? (
                      <p>No file changes have been recorded.</p>
                    ) : !captured ? (
                      <p>No changes were captured for this run.</p>
                    ) : !captured.files.length ? (
                      <p>No project files changed in this run.</p>
                    ) : diffRun && diffRun.id !== attempt?.id ? (
                      <p>
                        Changes from run{" "}
                        {attempts.findIndex((run) => run.id === diffRun.id) + 1}
                        .
                      </p>
                    ) : null)}
                  {i === 1 &&
                    !bothListsFailed &&
                    (workspace.isError ? (
                      <ErrorNotice
                        error={workspace.error}
                        onRetry={() => void workspace.refetch()}
                        retrying={workspace.isFetching}
                      />
                    ) : workspace.isLoading ? (
                      <p role="status">Loading workspace files…</p>
                    ) : workspace.data?.truncated ? (
                      <p>Showing the first 1,000 project files.</p>
                    ) : !groups[i].files.length ? (
                      <p>No workspace files to show.</p>
                    ) : null)}
                  {!!group.files.length && (
                    <ul>
                      {group.files.map((file) => (
                        <li key={file.path}>
                          <button
                            type="button"
                            aria-label={
                              file.status
                                ? `${file.path}, ${statusLabels[file.status] || "Changed"}`
                                : undefined
                            }
                            aria-current={
                              panel === "files" &&
                              file === selected &&
                              current.group === group.title
                                ? "true"
                                : undefined
                            }
                            onClick={() => {
                              setSelection({
                                path: file.path,
                                group: group.title,
                              });
                              setPanel("files");
                            }}
                            title={file.path}
                          >
                            <FileText size={14} aria-hidden="true" />
                            <span>{file.path}</span>
                            {file.status && (
                              <abbr
                                className={`file-status status-${file.status}`}
                                title={statusLabels[file.status] || "Changed"}
                              >
                                {file.status}
                              </abbr>
                            )}
                          </button>
                        </li>
                      ))}
                    </ul>
                  )}
                </section>
              ),
          )}
          {term && !visible.length && (
            <p role="status">No files match this filter.</p>
          )}
        </div>
        <div className="file-explorer-tools">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button type="button" size="sm" variant="ghost">
                Review tools <ChevronDown />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              {attempt?.task_id && (
                <DropdownMenuItem onSelect={() => setPanel("edits")}>
                  Recorded edits
                </DropdownMenuItem>
              )}
              {canReview && (
                <DropdownMenuItem onSelect={() => setPanel("agent")}>
                  Agent review
                </DropdownMenuItem>
              )}
              <DropdownMenuItem onSelect={() => setPanel("details")}>
                Technical details
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </aside>
      <div className="file-inspector">
        {canReview && attempt && (
          <div className="file-inspector-body" hidden={!agentPanel}>
            <AgentReview
              project={project}
              item={item}
              attempt={attempt}
              disabled={!canRequest}
              onFindings={onAsk}
            />
          </div>
        )}
        {agentPanel ? null : editPanel && attempt ? (
          <div className="file-inspector-body">
            <RecordedChanges project={project} attempt={attempt} />
          </div>
        ) : panel === "details" ? (
          <div className="file-inspector-body">
            <h3>Technical details</h3>
            <dl className="file-technical-details">
              <dt>Work</dt>
              <dd>{item.id}</dd>
              <dt>Worker</dt>
              <dd>{item.workspace_id}</dd>
              <dt>Agent</dt>
              <dd>{item.provider || "Worker default"}</dd>
              <dt>Model</dt>
              <dd>{item.model || "Configured default"}</dd>
              <dt>Attempt</dt>
              <dd>{attempt?.id || "Not started"}</dd>
              <dt>Task</dt>
              <dd>{attempt?.task_id || "Not accepted yet"}</dd>
              <dt>Specification</dt>
              <dd>{attempt?.specification_path || item.specification_path}</dd>
            </dl>
          </div>
        ) : selected && current.group === "Workspace files" ? (
          <WorkspaceFile
            key={selected.path}
            project={project}
            work={item.id}
            workspace={item.workspace_id}
            path={selected.path}
            revision={completedRun}
            changed={
              diffRun?.workspace_id === item.workspace_id &&
              !!captured?.files.some((file) => file.path === selected.path)
            }
            patch={patches.get(selected.path)}
            truncated={!!captured?.truncated}
          />
        ) : selected && selectedAttempt ? (
          <FileInspector
            key={selected.path}
            file={selected}
            patch={patches.get(selected.path)}
            truncated={!!captured?.truncated && !!selected.status}
            project={project}
            attempt={selectedAttempt}
            onAsk={ask}
            onAskAboutContent={canRequest ? onAsk : undefined}
          />
        ) : (
          <div className="file-empty">
            <Files size={24} aria-hidden="true" />
            <p>
              {term
                ? "Clear or change the filter to find a file."
                : changes.isLoading || workspace.isLoading
                  ? "Loading files…"
                  : "No files to show."}
            </p>
          </div>
        )}
      </div>
    </section>
  );
}
