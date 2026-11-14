import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Monitor,
  Code2,
  ChartNoAxesCombined,
  FileText,
  CheckCheck,
  X,
  Check,
  MessageSquare,
  ArrowUpRight,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";
import { ErrorNotice, State } from "@/shared/components";
import type { Project } from "@/shared/types";
import {
  workApi,
  workOptions,
  workKeys,
  type WorkItem,
} from "@/features/work/api";
import {
  instructionOptions,
  conversationTitle,
} from "@/features/work/conversation-title";
import { useThreadTitles } from "@/features/work/thread-titles";
import { WorkPreview } from "@/features/work/work-preview";
import { userMessage } from "@/features/work/conversation-api";
import { ReviewEvidence } from "./review-viewer";
import type { ReviewMode } from "./review-evidence";
import { workStatus } from "@/features/work/stages";
import { boardKey } from "./board-api";

const modes = [
  { id: "preview", label: "UI preview", icon: Monitor },
  { id: "code", label: "Code review", icon: Code2 },
  { id: "visuals", label: "Visualisations", icon: ChartNoAxesCombined },
  { id: "reports", label: "Reports", icon: FileText },
] as const;
const noop = () => {};
type ReviewDraft = { text: string; key: string };
const emptyDraft: ReviewDraft = { text: "", key: "" };
const draftKey = (item: WorkItem) =>
  `${item.id}:${item.latest_attempt_id || ""}`;

export function Reviews({ project }: { project: Project }) {
  return <ProjectReviews key={project.id} project={project} />;
}

function ProjectReviews({ project }: { project: Project }) {
  const items = useQuery(workOptions(project.id));
  // Private comments and their delivery keys survive task switches only within
  // this mounted project view. They are never written to browser storage.
  const [drafts, setDrafts] = useState<Record<string, ReviewDraft>>({});
  const [selected, setSelected] = useState("");
  const [search, setSearch] = useState("");
  const [history, setHistory] = useState(false);
  const [visibleLimit, setVisibleLimit] = useState(20);
  const [revealOpen, setRevealOpen] = useState(false);
  const queueRef = useRef<HTMLElement>(null);
  const term = search.trim().toLowerCase();
  const candidates = (items.data || []).filter((i) =>
    history
      ? ["completed", "closed", "failed", "cancelled"].includes(i.state)
      : i.state === "awaiting_review",
  );
  // Search changes the queue, never the review or comment being edited.
  const active =
    candidates.find((item) => item.id === selected) || candidates[0];
  const activeId = active?.id || "";
  useEffect(() => {
    if (selected !== activeId) setSelected(activeId);
  }, [selected, activeId]);
  const titleItems = [
    ...(active ? [active] : []),
    ...(term ? candidates : candidates.slice(0, visibleLimit)).filter(
      (item) => item.id !== activeId,
    ),
  ];
  const titles = useThreadTitles(project.id, titleItems);
  const queue = candidates
    .map((item) => ({
      item,
      title: titles.titles.get(item.id) || conversationTitle(item),
    }))
    .filter(({ item, title }) =>
      `${title} ${item.id} ${item.kind}`.toLowerCase().includes(term),
    );
  const visibleQueue = queue.slice(0, visibleLimit);
  const current = active && {
    item: active,
    title: titles.titles.get(active.id) || conversationTitle(active),
  };
  const currentOutsideList =
    current && !visibleQueue.some(({ item }) => item.id === current.item.id);
  useEffect(() => {
    if (!revealOpen) return;
    const button = queueRef.current?.querySelector<HTMLButtonElement>(
      '.review-queue-item[aria-pressed="true"]',
    );
    button?.focus({ preventScroll: true });
    button?.scrollIntoView({ block: "nearest", behavior: "instant" });
    setRevealOpen(false);
  }, [revealOpen]);
  return (
    <div className="reviews-page">
      <div className="page-title">
        <div>
          <h1>Reviews</h1>
        </div>
        <span className="review-count">
          <CheckCheck size={16} />
          {items.isPending || items.isError ? "—" : candidates.length}{" "}
          {history ? "finished tasks" : "awaiting review"}
        </span>
      </div>
      {items.isError && (
        <ErrorNotice
          error={items.error}
          onRetry={() => void items.refetch()}
          retrying={items.isFetching}
        />
      )}
      <div className="review-workbench">
        <aside
          ref={queueRef}
          className="review-queue"
          aria-label="Review queue"
        >
          <div className="review-queue-controls">
            <div className="review-queue-switch">
              <button
                aria-pressed={!history}
                onClick={() => {
                  setHistory(false);
                  setSelected("");
                  setVisibleLimit(20);
                }}
              >
                To review
              </button>
              <button
                aria-pressed={history}
                onClick={() => {
                  setHistory(true);
                  setSelected("");
                  setVisibleLimit(20);
                }}
              >
                History
              </button>
            </div>
            <Input
              className="h-8"
              aria-label="Filter reviews"
              placeholder="Find work…"
              value={search}
              onChange={(e) => {
                setSearch(e.target.value);
                setVisibleLimit(20);
              }}
            />
          </div>
          {items.isPending && (
            <p className="review-muted" role="status">
              Loading reviews…
            </p>
          )}
          {titles.pending > 0 && (
            <p className="review-muted" role="status">
              {term ? "Searching task titles…" : "Loading task titles…"}
            </p>
          )}
          {titles.failed > 0 && (
            <div className="review-queue-notice" role="status">
              <p>
                Some task titles could not be loaded.{" "}
                {term ? "Search results may be incomplete. " : ""}You can still
                find tasks by ID or type.
              </p>
              <Button
                variant="outline"
                size="sm"
                disabled={titles.retrying}
                onClick={() => void titles.retry()}
              >
                {titles.retrying ? "Retrying titles…" : "Retry titles"}
              </Button>
            </div>
          )}
          {visibleQueue.map(({ item, title }) => (
            <button
              className="review-queue-item"
              key={item.id}
              aria-pressed={current?.item.id === item.id}
              aria-description={
                drafts[draftKey(item)]?.text.trim()
                  ? "Unsent changes"
                  : undefined
              }
              onClick={() => setSelected(item.id)}
            >
              <strong>{title}</strong>
              <span>
                {history && <State value={workStatus(item.state)} />}
                <small>{item.kind}</small>
                {drafts[draftKey(item)]?.text.trim() && <small>Draft</small>}
                <time dateTime={item.updated_at}>
                  {new Date(item.updated_at).toLocaleDateString(undefined, {
                    month: "short",
                    day: "numeric",
                  })}
                </time>
              </span>
            </button>
          ))}
          {currentOutsideList && (
            <div className="review-queue-notice">
              <p>
                Your open review is outside this list. Its comment stays here.
              </p>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setRevealOpen(true);
                  setSearch("");
                  setVisibleLimit(
                    Math.max(
                      20,
                      candidates.findIndex((item) => item.id === activeId) + 1,
                    ),
                  );
                }}
              >
                Show open review
              </Button>
            </div>
          )}
          {queue.length > 0 && (
            <div className="review-queue-notice">
              <p>
                Showing {visibleQueue.length} of {queue.length}
                {term
                  ? ` ${titles.pending || titles.failed ? "available " : ""}${queue.length === 1 ? "match" : "matches"}`
                  : ` ${queue.length === 1 ? "task" : "tasks"}`}
              </p>
              {visibleQueue.length < queue.length && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setVisibleLimit((count) => count + 20)}
                >
                  Show more tasks
                </Button>
              )}
            </div>
          )}
          {!items.isPending &&
            !items.isError &&
            !queue.length &&
            !(term && titles.pending) && (
              <p className="review-muted">
                {term
                  ? titles.failed
                    ? "No matches in the titles available so far. Retry titles or search by ID or type."
                    : "No matching tasks."
                  : history
                    ? "No finished tasks yet."
                    : "Nothing waiting for review."}
              </p>
            )}
        </aside>
        {current ? (
          <ReviewDetail
            key={current.item.id + current.item.latest_attempt_id}
            project={project.id}
            item={current.item}
            title={current.title}
            statusUnavailable={items.isError}
            draft={drafts[draftKey(current.item)] || emptyDraft}
            onDraft={(text) =>
              setDrafts((old) => ({
                ...old,
                [draftKey(current.item)]:
                  old[draftKey(current.item)]?.text === text
                    ? old[draftKey(current.item)]
                    : { text, key: crypto.randomUUID() },
              }))
            }
          />
        ) : items.isPending || items.isError ? (
          <div className="review-empty">
            <h2>
              {items.isPending
                ? "Loading review queue…"
                : "Review queue unavailable"}
            </h2>
            <p>
              {items.isPending
                ? "Checking for tasks that need your decision."
                : "We could not check for tasks awaiting review. Try again to load the queue."}
            </p>
          </div>
        ) : (
          <div className="review-empty">
            <CheckCheck size={28} />
            <h2>
              {queue.length
                ? "Select work to review"
                : history
                  ? "Review history"
                  : "Your review queue is clear"}
            </h2>
            <p>
              {queue.length
                ? "Choose an item from the list to open its review."
                : search
                  ? "Try a different search."
                  : "Tasks ready for review appear here with their previews, code and reports."}
            </p>
          </div>
        )}
      </div>
    </div>
  );
}

function ReviewDetail({
  project,
  item,
  title,
  draft,
  onDraft,
  statusUnavailable,
}: {
  project: string;
  item: WorkItem;
  title: string;
  draft: ReviewDraft;
  onDraft: (text: string) => void;
  statusUnavailable: boolean;
}) {
  const client = useQueryClient();
  const [mode, setMode] = useState<ReviewMode>(
    item.kind === "software" ? "preview" : "reports",
  );
  const feedback = draft.text;
  const setFeedback = onDraft;
  const feedbackTooLong =
    new TextEncoder().encode(feedback.trim()).length > 16000;
  const [commenting, setCommenting] = useState(!!feedback.trim());
  const attempts = useQuery({
    queryKey: workKeys.attempts(project, item.id),
    queryFn: ({ signal }) => workApi.attempts(project, item.id, signal),
    refetchInterval: 5000,
  });
  const instruction = useQuery(instructionOptions(project, item));
  const attempt = attempts.data?.find((a) => a.id === item.latest_attempt_id);
  const canReview =
    !!attempt && item.state === "awaiting_review" && !attempt.review_state;
  const statusKnown = !statusUnavailable && !attempts.isError;
  const selectElement = useCallback((text: string) => {
    setFeedback(text);
    setCommenting(true);
  }, []);
  const decision = useMutation({
    mutationFn: (accepted: boolean | "closed") =>
      accepted === "closed"
        ? workApi.close(project, item.id, attempt!.id)
        : workApi.review(project, item.id, accepted, attempt!.id),
    onSuccess: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey: workKeys.list(project) }),
        client.invalidateQueries({ queryKey: boardKey(project) }),
      ]);
    },
  });
  const followup = useMutation({
    mutationFn: () =>
      workApi.continue(
        project,
        item.id,
        feedback.trim(),
        attempt!.id,
        draft.key,
      ),
    onSuccess: async () => {
      setFeedback("");
      await Promise.all([
        client.invalidateQueries({ queryKey: workKeys.list(project) }),
        client.invalidateQueries({ queryKey: boardKey(project) }),
      ]);
    },
  });
  return (
    <section className="review-detail" aria-label="Task review">
      <header className="review-detail-heading">
        <div>
          <h2>{title}</h2>
          <div className="review-task-meta">
            <State value={workStatus(item.state)} />
            <span>{item.kind}</span>
            <details className="review-request">
              <summary>Task description</summary>
              {instruction.isError ? (
                <ErrorNotice
                  error={instruction.error}
                  onRetry={() => void instruction.refetch()}
                  retrying={instruction.isFetching}
                />
              ) : (
                <pre>
                  {instruction.data
                    ? userMessage(instruction.data, {
                        parent_attempt_id: "followup",
                      })
                    : "Loading request…"}
                </pre>
              )}
            </details>
          </div>
        </div>
        <Link
          className="review-conversation-link"
          to="/projects/$projectId/$section"
          params={{ projectId: project, section: "work" }}
          search={{ work: item.id }}
        >
          Open conversation <ArrowUpRight size={14} />
        </Link>
      </header>
      <div className="review-modes" role="tablist" aria-label="Review format">
        {modes.map(({ id, label, icon: Icon }) => (
          <button
            role="tab"
            key={id}
            id={`review-tab-${id}`}
            aria-selected={mode === id}
            aria-controls="review-content"
            tabIndex={mode === id ? 0 : -1}
            onKeyDown={(event) => {
              const index = modes.findIndex((tab) => tab.id === mode);
              const next =
                event.key === "ArrowRight"
                  ? (index + 1) % modes.length
                  : event.key === "ArrowLeft"
                    ? (index + modes.length - 1) % modes.length
                    : event.key === "Home"
                      ? 0
                      : event.key === "End"
                        ? modes.length - 1
                        : -1;
              if (next < 0) return;
              event.preventDefault();
              setMode(modes[next].id);
              document.getElementById(`review-tab-${modes[next].id}`)?.focus();
            }}
            onClick={() => setMode(id)}
          >
            <Icon size={16} />
            {label}
          </button>
        ))}
      </div>
      {attempts.isError && (
        <ErrorNotice
          error={attempts.error}
          onRetry={() => void attempts.refetch()}
          retrying={attempts.isFetching}
        />
      )}
      {attempts.isPending && (
        <p className="review-muted" role="status">
          Loading review…
        </p>
      )}
      {attempt && (
        <div
          id="review-content"
          role="tabpanel"
          aria-labelledby={`review-tab-${mode}`}
          className="review-content"
        >
          {mode === "preview" ? (
            <WorkPreview
              project={project}
              workspace={attempt.workspace_id || item.workspace_id}
              revision={attempt.id}
              collapsible={false}
              reviewMode
              onReady={noop}
              onCollapse={noop}
              onSelection={selectElement}
            />
          ) : (
            <ReviewEvidence
              key={attempt.id + mode}
              project={project}
              attempt={attempt}
              mode={mode}
            />
          )}
        </div>
      )}
      {!attempt && !attempts.isPending && !attempts.isError && (
        <div className="review-empty" role="status">
          <h3>The result is not available yet</h3>
          <p>It will appear here when the task is ready.</p>
        </div>
      )}
      {attempt && (
        <footer className="review-decision">
          {!statusKnown && (
            <p className="review-context" role="status">
              Review status is unavailable. Your draft is kept here. Retry the
              failed status check before sending a decision.
            </p>
          )}
          {canReview ? (
            <>
              {commenting && (
                <div className="review-comment">
                  <label htmlFor="review-feedback">Changes needed</label>
                  <Textarea
                    id="review-feedback"
                    aria-label="Changes needed"
                    aria-invalid={feedbackTooLong || undefined}
                    aria-describedby={
                      feedbackTooLong ? "review-feedback-error" : undefined
                    }
                    placeholder="Describe the changes for the agent…"
                    autoFocus
                    value={feedback}
                    onChange={(e) => setFeedback(e.target.value)}
                    disabled={decision.isPending || followup.isPending}
                  />
                  <p>Your comment goes to the agent to make the changes.</p>
                  {feedbackTooLong && (
                    <p
                      id="review-feedback-error"
                      role="alert"
                      className="text-destructive"
                    >
                      This follow-up exceeds 16 kB. Shorten it or split it into
                      separate messages.
                    </p>
                  )}
                </div>
              )}
              <div className="review-decision-actions">
                {commenting ? (
                  <>
                    <Button
                      disabled={
                        !feedback.trim() ||
                        feedbackTooLong ||
                        !statusKnown ||
                        decision.isPending ||
                        followup.isPending
                      }
                      onClick={() => followup.mutate()}
                    >
                      <MessageSquare size={15} />
                      {followup.isPending ? "Sending…" : "Send changes"}
                    </Button>
                    <Button
                      variant="ghost"
                      disabled={decision.isPending || followup.isPending}
                      onClick={() => setCommenting(false)}
                    >
                      Cancel
                    </Button>
                  </>
                ) : (
                  <>
                    <Button
                      disabled={
                        !statusKnown || decision.isPending || followup.isPending
                      }
                      onClick={() => decision.mutate(true)}
                    >
                      <Check size={16} /> Approve
                    </Button>
                    <Button
                      variant="outline"
                      disabled={
                        !statusKnown || decision.isPending || followup.isPending
                      }
                      onClick={() => setCommenting(true)}
                    >
                      <MessageSquare size={15} /> Request changes
                    </Button>
                    <Button
                      className="review-close"
                      variant="ghost"
                      title="Close without approval. No more work needed."
                      disabled={
                        !statusKnown || decision.isPending || followup.isPending
                      }
                      onClick={() => decision.mutate("closed")}
                    >
                      <X size={16} /> Close
                    </Button>
                  </>
                )}
              </div>
            </>
          ) : (
            <p className="review-outcome">
              {item.state === "completed" || attempt.review_state === "accepted"
                ? "Approved. This task is done."
                : item.state === "closed" || attempt.review_state === "closed"
                  ? "Closed without approval. No more work needed."
                  : "This task is not waiting for review."}
            </p>
          )}
          {followup.isError && <ErrorNotice error={followup.error} />}
          {decision.isError && <ErrorNotice error={decision.error} />}
        </footer>
      )}
    </section>
  );
}
