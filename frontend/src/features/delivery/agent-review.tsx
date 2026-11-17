import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { ErrorNotice } from "@/shared/components";
import {
  workApi,
  workKeys,
  type WorkItem,
  type Attempt,
} from "@/features/work/api";
import { resultOptions } from "@/features/tasks/api";
import { MessageContent } from "@/features/work/message-content";

export function AgentReview({
  project,
  item,
  attempt,
  disabled,
  onFindings,
}: {
  project: string;
  item: WorkItem;
  attempt: Attempt;
  disabled: boolean;
  onFindings: (text: string) => void;
}) {
  const client = useQueryClient();
  const [focus, setFocus] = useState("");
  const [key, setKey] = useState(() => crypto.randomUUID());
  const [review, setReview] = useState<WorkItem>();
  const [dispatchError, setDispatchError] = useState<unknown>();
  const start = useMutation({
    mutationFn: () =>
      workApi.begin(
        {
          project_id: project,
          workspace_id: attempt.workspace_id,
          kind: "research",
          provider: item.provider,
          model: item.model,
          instruction: [
            `Agent review of work ${item.id}, revision ${attempt.id}.`,
            "Review only: inspect the evidence and report findings. Do not edit files, apply fixes, approve work, merge or deploy.",
            "This is a separate review task. Workspace files are live and may differ from the reviewed revision; clearly identify that limitation.",
            `Read the original request at ${JSON.stringify(attempt.specification_path || item.specification_path)}.`,
            `Retained output references: ${JSON.stringify(attempt.artifacts || [])}`,
            "Check correctness, regressions, missing tests and usability where relevant. Return actionable findings with severity, file/line or visual location, evidence, and suggested changes. State any checks you could not perform.",
            focus.trim() ? `Review focus: ${focus.trim()}` : "",
          ]
            .filter(Boolean)
            .join("\n\n"),
        },
        key,
      ),
    onSuccess: async (result) => {
      setReview(result.item);
      setDispatchError(result.error);
      await client.invalidateQueries({ queryKey: workKeys.list(project) });
    },
  });
  const attempts = useQuery({
    queryKey: workKeys.attempts(project, review?.id || ""),
    queryFn: ({ signal }) => workApi.attempts(project, review!.id, signal),
    enabled: !!review,
    refetchInterval: 5000,
  });
  const latest = attempts.data
    ?.slice()
    .sort((a, b) => b.created_at.localeCompare(a.created_at))[0];
  const finished =
    latest &&
    ![
      "queued",
      "running",
      "dispatching",
      "blocked",
      "cancel_requested",
    ].includes(latest.state);
  const result = useQuery(
    resultOptions(project, attempt.workspace_id, latest?.task_id, !!finished),
  );
  return (
    <details className="review-agent">
      <summary>Agent review</summary>
      <p>
        Run a separate review in this workspace. Findings are advisory; you
        choose which changes to send back.
      </p>
      {!review ? (
        <>
          <Textarea
            aria-label="Agent review focus"
            placeholder="Optional focus, e.g. accessibility or regressions"
            maxLength={2000}
            disabled={start.isPending}
            value={focus}
            onChange={(e) => {
              setFocus(e.target.value);
              setKey(crypto.randomUUID());
            }}
          />
          <Button
            variant="outline"
            disabled={disabled || start.isPending}
            onClick={() => start.mutate()}
          >
            {start.isPending ? "Starting review…" : "Start agent review"}
          </Button>
        </>
      ) : (
        <>
          <Link
            to="/projects/$projectId/$section"
            params={{ projectId: project, section: "work" }}
            search={{ work: review.id }}
          >
            Open agent review conversation
          </Link>
          {dispatchError ? (
            <>
              <ErrorNotice error={dispatchError} />
              <p>
                The review was saved. Open its conversation to retry starting
                it.
              </p>
            </>
          ) : (
            <p role="status">
              {latest ? `Review: ${latest.state}` : "Review queued…"}
            </p>
          )}
        </>
      )}
      {start.isError && <ErrorNotice error={start.error} />}
      {attempts.isError && <ErrorNotice error={attempts.error} />}
      {result.isError && <ErrorNotice error={result.error} />}
      {result.data?.content && (
        <>
          <MessageContent
            content={result.data.content}
            project={project}
            attempt={latest!}
          />
          {result.data.next_offset !== -1 && (
            <p>
              Showing part of the review. Open the conversation for the full
              result.
            </p>
          )}
          <Button
            variant="outline"
            disabled={
              disabled ||
              new TextEncoder().encode(result.data.content).length > 14000
            }
            onClick={() =>
              onFindings(
                `Agent review findings to address:\n\n${result.data!.content}`,
              )
            }
          >
            Add findings to change notes
          </Button>
        </>
      )}
    </details>
  );
}
