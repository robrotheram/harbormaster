import "./work-files.css";
import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
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
  sources,
  canReview,
  canRequest,
  onAsk,
}: {
  project: string;
  item: WorkItem;
  attempt?: Attempt;
  sources: ReplyOutput[];
  canReview: boolean;
  canRequest: boolean;
  onAsk: (message: string) => void;
}) {
  const running = !!attempt && activeAttempt(attempt.state);
  const changes = useQuery({
    ...privateContentOptions,
    queryKey: ["project", project, "task-changes", item.id, attempt?.id],
    queryFn: ({ signal }) =>
      workApi.changes(project, item.id, attempt!.id, signal),
    enabled: !!attempt?.task_id && !running,
  });
  const [filter, setFilter] = useState("");
  const [path, setPath] = useState<string>();
  const [panel, setPanel] = useState<"files" | "edits" | "agent" | "details">(
    "files",
  );
  const term = filter.trim().toLowerCase();
  const groups = useMemo(() => {
    const files = new Map<string, ReviewFile>();
    for (const file of changes.data?.files ?? [])
      files.set(file.path, { ...file });
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
        title: "Saved files",
        files: all.filter((file) => !file.status && file.artifact),
      },
      {
        title: "From this reply",
        files: all.filter((file) => !file.status && !file.artifact),
      },
    ];
  }, [changes.data, attempt?.artifacts, sources]);
  const patches = useMemo(
    () =>
      filePatches(
        changes.data?.diff ?? "",
        changes.data?.files.map((file) => file.path) ?? [],
      ),
    [changes.data],
  );
  const matches = groups.map((group) => ({
    ...group,
    files: group.files.filter((file) => file.path.toLowerCase().includes(term)),
  }));
  const visible = matches.flatMap((group) => group.files);
  const selected = visible.find((file) => file.path === path) || visible[0];
  const ask =
    canRequest && selected && attempt
      ? (reference?: LineReference) => {
          const location = reference
            ? `:${Math.min(reference.start, reference.end)}${reference.start === reference.end ? "" : `-${Math.max(reference.start, reference.end)}`}`
            : "";
          const side =
            selected.status && reference?.side === "before"
              ? ", before this change"
              : "";
          onAsk(
            `Explain ${JSON.stringify(selected.path + location)} (revision ${attempt.id}${side}): `,
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
          {matches.map(
            (group, i) =>
              (i === 0 || group.files.length > 0) && (
                <section key={group.title} aria-label={group.title}>
                  <h4>
                    {group.title} <span>{groups[i].files.length}</span>
                  </h4>
                  {i === 0 &&
                    (running ? (
                      <p>Changes appear after the run finishes.</p>
                    ) : changes.isError ? (
                      <ErrorNotice
                        error={changes.error}
                        onRetry={() => void changes.refetch()}
                        retrying={changes.isFetching}
                      />
                    ) : changes.isLoading ? (
                      <p role="status">Loading changes…</p>
                    ) : !changes.data ? (
                      <p>No changes were captured.</p>
                    ) : !changes.data.files.length ? (
                      <p>No project files changed.</p>
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
                              panel === "files" && file === selected
                                ? "true"
                                : undefined
                            }
                            onClick={() => {
                              setPath(file.path);
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
        ) : selected && attempt ? (
          <FileInspector
            key={selected.path}
            file={selected}
            patch={patches.get(selected.path)}
            truncated={!!changes.data?.truncated && !!selected.status}
            project={project}
            attempt={attempt}
            onAsk={ask}
            onAskAboutContent={canRequest ? onAsk : undefined}
          />
        ) : (
          <div className="file-empty">
            <Files size={24} aria-hidden="true" />
            <p>
              {term
                ? "Clear or change the filter to find a file."
                : changes.isLoading
                  ? "Loading files…"
                  : "No files to show."}
            </p>
          </div>
        )}
      </div>
    </section>
  );
}
