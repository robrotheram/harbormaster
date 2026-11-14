import { useState } from "react";
import { useQuery, useMutation } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { ErrorNotice } from "@/shared/components";
import { privateContentOptions } from "@/shared/private-content";
import { evidenceURL, type Artifact, type Attempt } from "@/features/work/api";
import { runHistory } from "@/features/work/conversation-api";
import type { HistoryRecord } from "@/features/history/history-api";
import { resultOptions } from "@/features/tasks/api";
import {
  MessageContent,
  WorkspaceImage,
} from "@/features/work/message-content";
import { ChartContent } from "@/features/work/chart-content";
import { readOutput } from "@/features/work/read-output";
import {
  artifactMode,
  recordedEdits,
  visualSections,
  type ReviewMode,
} from "./review-evidence";

export function ReviewEvidence({
  project,
  attempt,
  mode,
}: {
  project: string;
  attempt: Attempt;
  mode: Exclude<ReviewMode, "preview">;
}) {
  const [older, setOlder] = useState<HistoryRecord[]>([]);
  const [cursor, setCursor] = useState<number>();
  const [selection, setSelection] = useState(mode === "code" ? "" : "summary");
  const history = useQuery({
    ...privateContentOptions,
    queryKey: [
      "project",
      project,
      "review-history",
      attempt.id,
      attempt.workspace_id,
      attempt.task_id,
    ],
    queryFn: ({ signal }) =>
      runHistory(project, attempt.workspace_id, attempt.task_id!, 0, signal),
    enabled: !!attempt.workspace_id && !!attempt.task_id,
    retry: false,
  });
  const more = useMutation({
    mutationFn: () =>
      runHistory(
        project,
        attempt.workspace_id,
        attempt.task_id!,
        cursor ?? history.data!.page.next_before,
      ),
    onSuccess: (data) => {
      setOlder((old) => [...data.page.records, ...old]);
      setCursor(data.page.next_before);
    },
  });
  const records = [
    ...new Map(
      [...older, ...(history.data?.page.records || [])].map((r) => [r.id, r]),
    ).values(),
  ].sort((a, b) => a.seq - b.seq);
  const messages = records.filter(
    (r) => r.kind === "assistant.message" && r.content && !r.redacted,
  );
  const result = useQuery(
    resultOptions(
      project,
      attempt.workspace_id,
      attempt.task_id,
      !!attempt.workspace_id &&
        !!attempt.task_id &&
        mode !== "code" &&
        !history.isFetching &&
        !messages.length,
    ),
  );
  const edits = recordedEdits(records);
  const files = (attempt.artifacts || []).filter(
    (a) => artifactMode(a) === mode,
  );
  const selectedFile = files.find(
    (a) => (a.id || a.source_path || a.name) === selection,
  );
  const selectedEdit = edits.find((e) => e.id === selection);
  const report = result.data?.content || messages.at(-1)?.content || "";
  const resultFailed = result.isError && !messages.length;
  const visuals =
    messages
      .map((m) => visualSections(m.content || ""))
      .filter(Boolean)
      .join("\n\n") || visualSections(report);
  const next = cursor ?? history.data?.page.next_before;
  const file = selectedFile || (!selectedEdit && !selection && files[0]);
  return (
    <div className="review-evidence">
      <div className="review-evidence-toolbar">
        <span>
          {mode === "code"
            ? "Retained files and recorded edits"
            : mode === "visuals"
              ? "Delivered charts, images and data"
              : "Task report and documents"}
        </span>
        {!!next && (
          <Button
            size="sm"
            variant="outline"
            disabled={more.isPending}
            onClick={() => more.mutate()}
          >
            Load earlier evidence
          </Button>
        )}
      </div>
      {history.isError && (
        <ErrorNotice
          error={history.error}
          onRetry={() => void history.refetch()}
          retrying={history.isFetching}
        />
      )}
      {more.isError && (
        <ErrorNotice
          error={more.error}
          onRetry={() => more.mutate()}
          retrying={more.isPending}
        />
      )}
      {resultFailed && mode !== "code" && (
        <ErrorNotice
          error={result.error}
          onRetry={() => void result.refetch()}
          retrying={result.isFetching}
        />
      )}
      {((history.isError && records.length > 0) ||
        (resultFailed && report)) && (
        <p className="review-context" role="status">
          Showing previously loaded evidence. Retry the failed read to check for
          updates.
        </p>
      )}
      {history.data?.page.capture_limited && (
        <p className="review-context">
          Activity capture was limited. The recorded evidence may be incomplete.
        </p>
      )}
      {history.isFetching && (
        <p className="review-muted" role="status">
          Loading recorded evidence…
        </p>
      )}
      <div className="review-evidence-layout">
        {(files.length > 0 || (mode === "code" && edits.length > 0)) && (
          <nav className="review-files" aria-label="Evidence files">
            {mode !== "code" && (
              <button
                aria-current={!file ? "true" : undefined}
                onClick={() => setSelection("summary")}
              >
                {mode === "reports" ? "Worker report" : "Inline visualisations"}
              </button>
            )}
            {files.map((a, n) => (
              <button
                key={a.id || n}
                aria-current={a === file ? "true" : undefined}
                onClick={() =>
                  setSelection(a.id || a.source_path || a.name || "")
                }
              >
                <span>{a.source_path || a.name || "Output"}</span>
                <small>Retained file</small>
              </button>
            ))}
            {mode === "code" &&
              edits.map((e, n) => (
                <button
                  key={e.id}
                  aria-current={
                    (selectedEdit || (!file ? edits[0] : undefined))?.id ===
                    e.id
                      ? "true"
                      : undefined
                  }
                  onClick={() => setSelection(e.id)}
                >
                  <span>Edit {n + 1}</span>
                  <small>{e.title}</small>
                </button>
              ))}
          </nav>
        )}
        <div className="review-document">
          {file ? (
            <ArtifactViewer
              key={file.id || file.content_url}
              artifact={file}
              project={project}
              attempt={attempt}
              mode={mode}
            />
          ) : mode === "code" ? (
            edits.length ? (
              <CodeEvidence edit={selectedEdit || edits[0]} />
            ) : history.isFetching ? (
              <p role="status">Loading code evidence…</p>
            ) : history.isError ? (
              <EmptyEvidence
                title="Code evidence unavailable"
                text="Retry loading recorded evidence above, or open the conversation to check this task."
              />
            ) : (
              <EmptyEvidence
                title="No code evidence in this view"
                text={
                  next
                    ? "Load earlier evidence to check previous file edits for this task. Reports contains the delivery summary."
                    : "No source files or captured file edits are available. Open Reports for the delivery summary, or the conversation for other tool activity."
                }
              />
            )
          ) : mode === "visuals" ? (
            visuals ? (
              <MessageContent
                content={visuals}
                embedData
                project={project}
                attempt={attempt}
              />
            ) : history.isFetching || result.isFetching ? (
              <p role="status">Loading visualisations…</p>
            ) : history.isError || resultFailed ? (
              <EmptyEvidence
                title="Visualisations unavailable"
                text="Retry the failed evidence read, or open the conversation to check this task."
              />
            ) : (
              <EmptyEvidence
                title="No visualisations in this delivery"
                text="Charts, images and CSV or JSON data appear here when the worker includes them in its response or retained outputs."
              />
            )
          ) : (
            <>
              {report ? (
                <>
                  <div className="review-document-heading">
                    <h3>Worker report</h3>
                    <span>Task result</span>
                  </div>
                  <MessageContent
                    content={report}
                    project={project}
                    attempt={attempt}
                  />
                  {((result.data && result.data.next_offset !== -1) ||
                    messages.at(-1)?.truncated) && (
                    <p className="review-context">
                      This report is truncated. Open the conversation for more
                      context.
                    </p>
                  )}
                </>
              ) : !history.isFetching &&
                !result.isFetching &&
                (history.isError || resultFailed) ? (
                <EmptyEvidence
                  title="Report unavailable"
                  text="Retry the failed evidence read above, or open the conversation to check this task."
                />
              ) : !history.isFetching && !result.isFetching ? (
                <EmptyEvidence
                  title="No report available"
                  text="This task has no readable response or retained report. Check its conversation before making a review decision."
                />
              ) : (
                <p role="status">Loading report…</p>
              )}
              {messages.length > 1 && (
                <details className="review-earlier">
                  <summary>Earlier responses ({messages.length - 1})</summary>
                  {messages.slice(0, -1).map((m) => (
                    <article key={m.id}>
                      <MessageContent
                        content={m.content!}
                        project={project}
                        attempt={attempt}
                      />
                    </article>
                  ))}
                </details>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}
function EmptyEvidence({ title, text }: { title: string; text: string }) {
  return (
    <div className="review-empty">
      <h3>{title}</h3>
      <p>{text}</p>
    </div>
  );
}
function CodeEvidence({
  edit,
}: {
  edit: ReturnType<typeof recordedEdits>[number];
}) {
  return (
    <>
      <div className="review-document-heading">
        <h3>{edit.title}</h3>
      </div>
      <p className="review-context">
        Captured activity for this task, not a complete repository diff. Edit
        requests may have failed; check the report and conversation for the
        outcome.
      </p>
      {edit.truncated && (
        <p className="review-context">This recorded edit is truncated.</p>
      )}
      <CodeContent content={edit.content} />
    </>
  );
}
function CodeContent({ content }: { content: string }) {
  return (
    <pre className="review-code">
      {content.split("\n").map((line, n) => (
        <span
          className={
            line.startsWith("+")
              ? "is-added"
              : line.startsWith("-")
                ? "is-removed"
                : line.startsWith("@@")
                  ? "is-hunk"
                  : ""
          }
          key={n}
        >
          <i aria-hidden="true">{n + 1}</i>
          <code>{line || " "}</code>
        </span>
      ))}
    </pre>
  );
}
function ArtifactViewer({
  artifact,
  project,
  attempt,
  mode,
}: {
  artifact: Artifact;
  project: string;
  attempt: Attempt;
  mode: ReviewMode;
}) {
  const path = artifact.source_path || artifact.name || "Output";
  const url = evidenceURL(artifact.content_url);
  const isImage = /\.(png|jpe?g|gif|webp|svg)$/i.test(path);
  const isDownload = /\.(pdf|html?|zip|gz|exe|wasm|bin)$/i.test(path);
  const text = useQuery({
    ...privateContentOptions,
    queryKey: [
      "project",
      project,
      "review-artifact",
      attempt.id,
      artifact.id,
      url,
    ],
    queryFn: ({ signal }) => readOutput(url!, signal),
    enabled: !!url?.startsWith("/ui-api/") && !isImage && !isDownload,
    retry: false,
  });
  return (
    <>
      <div className="review-document-heading">
        <h3>{path}</h3>
        {url && (
          <a href={url} target="_blank" rel="noopener noreferrer">
            Open original
          </a>
        )}
      </div>
      {isImage ? (
        <WorkspaceImage
          src={path}
          alt={path}
          project={project}
          attempt={attempt}
        />
      ) : isDownload || !url?.startsWith("/ui-api/") ? (
        <EmptyEvidence
          title="Open the original file to review"
          text="This format is available through the original file link above when retained."
        />
      ) : text.isError ? (
        <ErrorNotice
          error={text.error}
          onRetry={() => void text.refetch()}
          retrying={text.isFetching}
        />
      ) : text.isPending ? (
        <p role="status">Loading file…</p>
      ) : mode === "code" ? (
        <CodeContent content={text.data} />
      ) : mode === "visuals" ? (
        <ChartContent source={text.data} title={path} />
      ) : (
        <MessageContent
          content={text.data}
          project={project}
          attempt={attempt}
        />
      )}
    </>
  );
}
