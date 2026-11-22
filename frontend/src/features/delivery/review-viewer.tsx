import { useState } from "react";
import { useQuery, useMutation } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { ErrorNotice } from "@/shared/components";
import { privateContentOptions } from "@/shared/private-content";
import { evidenceURL, type Artifact, type Attempt } from "@/features/work/api";
import { runHistory } from "@/features/work/conversation-api";
import type { HistoryRecord } from "@/features/history/history-api";
import {
  MessageContent,
  WorkspaceImage,
} from "@/features/work/message-content";
import { ChartContent } from "@/features/work/chart-content";
import { readOutput } from "@/features/work/read-output";
import { recordedEdits } from "./review-evidence";

export function RecordedChanges({
  project,
  attempt,
}: {
  project: string;
  attempt: Attempt;
}) {
  const [older, setOlder] = useState<HistoryRecord[]>([]);
  const [cursor, setCursor] = useState<number>();
  const [selection, setSelection] = useState("");
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
    queryFn: ({ signal }) => runHistory(project, attempt, 0, signal),
    enabled: !!attempt.workspace_id && !!attempt.task_id,
    retry: false,
  });
  const more = useMutation({
    mutationFn: () =>
      runHistory(project, attempt, cursor ?? history.data!.page.next_before),
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
  const edits = recordedEdits(records);
  const selected = edits.find((edit) => edit.id === selection) || edits[0];
  const next = cursor ?? history.data?.page.next_before;
  return (
    <div className="review-evidence">
      {!!next && (
        <Button
          size="sm"
          variant="outline"
          disabled={more.isPending}
          onClick={() => more.mutate()}
        >
          Load earlier edits
        </Button>
      )}
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
      {history.data?.page.capture_limited && (
        <p className="review-context">
          Activity capture was limited. The recorded evidence may be incomplete.
        </p>
      )}
      {history.isFetching && <p role="status">Loading recorded edits…</p>}
      {edits.length > 0 && (
        <div className="review-evidence-layout">
          <nav className="review-files" aria-label="Recorded edits">
            {edits.map((edit, index) => (
              <button
                key={edit.id}
                aria-current={selected.id === edit.id ? "true" : undefined}
                onClick={() => setSelection(edit.id)}
              >
                <span>Edit {index + 1}</span>
                <small>{edit.title}</small>
              </button>
            ))}
          </nav>
          <div className="review-document">
            <CodeEvidence edit={selected} />
          </div>
        </div>
      )}
      {!edits.length && !history.isFetching && !history.isError && (
        <p className="review-muted">No edits were captured for this attempt.</p>
      )}
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
export function ArtifactViewer({
  artifact,
  project,
  attempt,
  onAsk,
  showChart = false,
}: {
  artifact: Artifact;
  project: string;
  attempt: Attempt;
  onAsk?: (message: string) => void;
  showChart?: boolean;
}) {
  const path = artifact.source_path || artifact.name || "Output";
  const url = evidenceURL(artifact.content_url);
  const isImage = /\.(png|jpe?g|gif|webp|avif|svg)$/i.test(path);
  const isDownload = /\.(pdf|zip|gz|exe|wasm|bin)$/i.test(path);
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
          <a href={url} download>
            Download
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
        <p className="review-muted">Download the original file to view it.</p>
      ) : text.isError ? (
        <ErrorNotice
          error={text.error}
          onRetry={() => void text.refetch()}
          retrying={text.isFetching}
        />
      ) : text.isPending ? (
        <p role="status">Loading file…</p>
      ) : /\.(diff|patch)$/i.test(path) ? (
        <CodeContent content={text.data} />
      ) : showChart ? (
        <ChartContent source={text.data} title={path} onAsk={onAsk} />
      ) : /\.(md|markdown)$/i.test(path) ? (
        <MessageContent
          content={text.data}
          onAsk={onAsk}
          embedData
          project={project}
          attempt={attempt}
        />
      ) : (
        <pre>{text.data}</pre>
      )}
    </>
  );
}
