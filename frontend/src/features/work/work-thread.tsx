import { instructionOptions } from "./conversation-title";
import { composerKeyDown } from "./composer-keydown";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
  type Dispatch,
  type SetStateAction,
} from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowDown,
  ArrowUp,
  Check,
  FileText,
  Loader2,
  Square,
  Terminal,
  Download,
  PanelRightOpen,
  BarChart3,
  MoreHorizontal,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import {
  ErrorNotice,
  Form,
  Modal,
  State,
  useRequestKey,
} from "@/shared/components";
import { APIError } from "@/shared/http";
import { privateContentOptions } from "@/shared/private-content";
import { date, label, bytes } from "@/shared/format";
import type { Project } from "@/shared/types";
import { workspacesApi, workspaceOptions } from "@/features/workspaces/api";
import type { HistoryRecord, Snapshot } from "@/features/history/history-api";
import { resultOptions } from "@/features/tasks/api";
import {
  workActions,
  workApi,
  workKeys,
  evidenceURL,
  type Attempt,
  type Artifact,
  type WorkItem,
} from "./api";
import {
  activeAttempt,
  runHistory,
  refreshRunHistory,
  userMessage,
} from "./conversation-api";
import { readOutput } from "./read-output";
import { WorkVisuals } from "./work-visuals";
import { workStatus } from "./stages";
import { WorkPreview } from "./work-preview";
import { MessageContent } from "./message-content";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";

export function WorkConversation({
  project,
  id,
  startError,
  title,
  navigation,
  message,
  setMessage,
}: {
  project: Project;
  id: string;
  startError?: unknown;
  title?: string;
  navigation?: ReactNode;
  message: string;
  setMessage: Dispatch<SetStateAction<string>>;
}) {
  const client = useQueryClient();
  const key = useRequestKey();
  const [earlier, setEarlier] = useState(false);
  const [files, setFiles] = useState(false);
  const [previewOpen, setPreviewOpen] = useState(true);
  const collapsePreview = useCallback(() => {
    setPreviewOpen(false);
    setMobileView("chat");
  }, []);
  const revealPreview = useCallback(() => setPreviewOpen(true), []);
  const [visuals, setVisuals] = useState(false);
  const [mobileView, setMobileView] = useState("chat");
  const [ratio, setRatio] = useState(47);
  const split = useRef<HTMLElement>(null);
  const selectElement = useCallback(
    (text: string) => {
      setMessage((old) => (old ? old + "\n" + text : text));
      setMobileView("chat");
      requestAnimationFrame(() => composer.current?.focus());
    },
    [setMessage],
  );
  const composer = useRef<HTMLTextAreaElement>(null);
  const feed = useRef<HTMLDivElement>(null);
  const follow = useRef(true);
  const [newActivity, setNewActivity] = useState(false);
  const announceActivity = useCallback(() => {
    if (!follow.current) setNewActivity(true);
  }, []);
  const itemQuery = useQuery({
    queryKey: workKeys.detail(project.id, id),
    queryFn: ({ signal }) => workApi.detail(project.id, id, signal),
    refetchInterval: 3000,
  });
  const attemptQuery = useQuery({
    queryKey: workKeys.attempts(project.id, id),
    queryFn: ({ signal }) => workApi.attempts(project.id, id, signal),
    enabled: !!itemQuery.data && !itemQuery.isError,
    refetchInterval: 3000,
  });
  const workers = useQuery(workspaceOptions(project.id));
  const item = itemQuery.isError ? undefined : itemQuery.data;
  const attempts = attemptQuery.isError
    ? []
    : [...(attemptQuery.data || [])].sort((a, b) =>
        a.created_at.localeCompare(b.created_at),
      );
  const latest = attempts.find((a) => a.id === item?.latest_attempt_id);
  const actions = item && workActions(item.state);
  const running =
    !!item && (actions?.cancel || (!!latest && activeAttempt(latest.state)));
  const worker = workers.data?.find((w) => w.id === item?.workspace_id);
  const [actionSequence, setActionSequence] = useState(0);
  const mutation = useMutation({
    mutationFn: (action: "run" | "cancel" | "accept" | "continue" | "wake") => {
      const delivery = key({
        action,
        actionSequence,
        id,
        message,
        attempt: item?.latest_attempt_id,
      });
      if (action === "run") return workApi.run(project.id, id, delivery);
      if (action === "cancel") return workApi.cancel(project.id, id);
      if (action === "accept")
        return workApi.review(project.id, id, true, item?.latest_attempt_id);
      if (action === "wake")
        return workspacesApi.start(item!.workspace_id, delivery);
      return workApi.continue(
        project.id,
        id,
        message,
        item!.latest_attempt_id!,
        delivery,
      );
    },
    onSuccess: async (_, action) => {
      setActionSequence((v) => v + 1);
      if (action === "continue") {
        setMessage("");
        follow.current = true;
      }
      await client.invalidateQueries({ queryKey: workKeys.list(project.id) });
      if (action === "wake")
        await client.invalidateQueries({ queryKey: ["project", project.id] });
    },
  });
  // Follow new content only while the reader is already at the bottom.
  useEffect(() => {
    const el = feed.current;
    if (!el) return;
    const observer = new MutationObserver(() => {
      if (follow.current) el.scrollTop = el.scrollHeight;
    });
    observer.observe(el, {
      childList: true,
      subtree: true,
      characterData: true,
    });
    return () => observer.disconnect();
  }, [!!item]);
  if (itemQuery.isError)
    return (
      <section className="conversation-main">
        {navigation}
        <ErrorNotice
          error={itemQuery.error}
          onRetry={() => void itemQuery.refetch()}
          retrying={itemQuery.isFetching}
        />
      </section>
    );
  if (!item)
    return (
      <section className="conversation-main" role="status">
        {navigation}
        Loading conversation…
      </section>
    );
  const canContinue =
    !!item.latest_attempt_id &&
    [
      "planned",
      "in_progress",
      "completed",
      "accepted",
      "awaiting_review",
      "failed",
      "cancelled",
    ].includes(item.state);
  const canSend =
    !running &&
    !mutation.isPending &&
    canContinue &&
    !!message.trim() &&
    new TextEncoder().encode(message).length <= 16000;
  return (
    <section
      ref={split}
      className={`conversation-session visual-session show-${mobileView} ${previewOpen ? "" : "is-preview-collapsed"}`}
      style={{ "--conversation-ratio": `${ratio}%` } as CSSProperties}
      aria-label="Work conversation"
    >
      <div className="conversation-mobile-switch">
        {navigation}
        <button
          aria-pressed={mobileView === "chat"}
          onClick={() => setMobileView("chat")}
        >
          Conversation
        </button>
        <button
          aria-pressed={mobileView === "preview"}
          onClick={() => {
            setPreviewOpen(true);
            setMobileView("preview");
          }}
        >
          Preview
        </button>
      </div>
      <div className="conversation-main">
        <header className="conversation-thread-heading">
          <div className="conversation-heading-title">
            <div className="min-w-0">
              <h2 title={title} className="line-clamp-2">
                {title || `${label(item.kind)} conversation`}
              </h2>
              <p>
                {worker?.name || "Assigned worker"}
                <span>Started {date(item.created_at)}</span>
              </p>
            </div>
          </div>
          <div className="conversation-heading-actions">
            {!previewOpen && (
              <Button
                className="conversation-preview-toggle"
                variant="ghost"
                size="sm"
                onClick={() => setPreviewOpen(true)}
              >
                <PanelRightOpen size={14} />
                Show preview
              </Button>
            )}
            <span className="conversation-thread-status">
              <State value={workStatus(item.state)} />
            </span>
            {actions?.review && (
              <Button
                variant="outline"
                size="sm"
                disabled={mutation.isPending || itemQuery.isFetching}
                onClick={() => mutation.mutate("accept")}
                aria-label="Accept result"
              >
                <Check size={14} />{" "}
                <span className="hidden min-[480px]:inline">Accept result</span>
                <span className="min-[480px]:hidden">Accept</span>
              </Button>
            )}
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label="Conversation options"
                >
                  <MoreHorizontal size={16} />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onSelect={() => setFiles(true)}>
                  <FileText size={14} /> Files
                </DropdownMenuItem>
                <DropdownMenuItem
                  onSelect={() => {
                    setVisuals(true);
                    follow.current = true;
                  }}
                >
                  <BarChart3 size={14} /> Visualize data
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </header>
        {!!startError && item.state === "draft" && (
          <div className="conversation-notice">
            <p>Your request is saved. Starting the task failed; retry below.</p>
            <ErrorNotice error={startError} />
          </div>
        )}
        <div
          className="conversation-feed"
          ref={feed}
          onScroll={() => {
            const el = feed.current!;
            follow.current =
              el.scrollHeight - el.scrollTop - el.clientHeight < 100;
            if (follow.current) setNewActivity(false);
          }}
          aria-label="Conversation and agent activity"
          role="region"
          tabIndex={0}
        >
          {attemptQuery.isError && <ErrorNotice error={attemptQuery.error} />}
          {!attempts.length &&
            !attemptQuery.isPending &&
            !attemptQuery.isError && (
              <DraftMessage project={project.id} item={item} />
            )}
          {!earlier && attempts.length > 1 && (
            <Button
              variant="ghost"
              onClick={() => {
                follow.current = false;
                setEarlier(true);
              }}
            >
              Show earlier conversation
            </Button>
          )}
          {earlier && (
            <Button size="sm" variant="ghost" onClick={() => setEarlier(false)}>
              Hide earlier conversation
            </Button>
          )}
          {(earlier ? attempts : attempts.slice(-1)).map((a) => (
            <RunConversation
              key={a.id}
              project={project.id}
              attempt={a}
              onAsk={selectElement}
              onActivity={announceActivity}
              onOlder={() => {
                follow.current = false;
              }}
            />
          ))}
          {visuals && (
            <section
              className="conversation-visualization"
              aria-label="Conversation visualization"
            >
              <WorkVisuals
                key={latest?.id || "draft"}
                project={project.id}
                onAsk={selectElement}
                artifacts={latest?.artifacts || []}
                initialEditing={visuals}
                onDismiss={() => setVisuals(false)}
              />
            </section>
          )}
          {running && (
            <div className="conversation-live" role="status">
              <Loader2 size={14} className="animate-spin" />
              <span>
                {latest?.state === "cancel_requested"
                  ? "Stopping the task…"
                  : latest?.task_id
                    ? "Agent is working. Recorded activity updates here."
                    : "Preparing the worker and waiting for task acceptance…"}
              </span>
            </div>
          )}
          {worker?.state === "STOPPED" && (
            <div className="conversation-notice">
              <p>
                The worker is stopped. Start it here to read the private
                conversation.
              </p>
              <Button
                variant="outline"
                disabled={mutation.isPending}
                onClick={() => mutation.mutate("wake")}
              >
                Load conversation
              </Button>
            </div>
          )}
        </div>
        {newActivity && (
          <Button
            className="conversation-new-activity"
            size="sm"
            variant="outline"
            onClick={() => {
              follow.current = true;
              feed.current?.scrollTo({
                top: feed.current.scrollHeight,
                behavior: window.matchMedia("(prefers-reduced-motion: reduce)")
                  .matches
                  ? "instant"
                  : "smooth",
              });
              setNewActivity(false);
            }}
          >
            <ArrowDown size={14} /> New activity
          </Button>
        )}
        <div className="conversation-composer">
          {mutation.isError && <ErrorNotice error={mutation.error} />}
          {actions?.run && (
            <div className="conversation-run-retry">
              <span>
                {item.state === "planned" || item.state === "in_progress"
                  ? "Reopened. Start the agent or send a new request to continue."
                  : latest
                    ? "This task did not finish. Retry it or send a revised request below."
                    : "Your draft is ready to start."}
              </span>
              <Button
                disabled={mutation.isPending}
                onClick={() => mutation.mutate("run")}
              >
                {latest &&
                item.state !== "planned" &&
                item.state !== "in_progress"
                  ? "Retry task"
                  : "Start task"}
              </Button>
            </div>
          )}
          <Form
            onSubmit={() => {
              if (canSend) mutation.mutate("continue");
            }}
          >
            <label className="sr-only" htmlFor="work-followup">
              Message to your agent
            </label>
            <Textarea
              ref={composer}
              id="work-followup"
              aria-invalid={
                new TextEncoder().encode(message).length > 16000 || undefined
              }
              aria-describedby={
                new TextEncoder().encode(message).length > 16000
                  ? "work-followup-error"
                  : undefined
              }
              rows={2}
              value={message}
              onChange={(e) => setMessage(e.target.value)}
              onKeyDown={(e) => composerKeyDown(e, canSend)}
              placeholder={
                running
                  ? "You can draft your next message while the agent works…"
                  : "Ask for a change or a new task…"
              }
              disabled={!item.latest_attempt_id || mutation.isPending}
            />
            <div className="conversation-compose-actions">
              <span>
                {running
                  ? "Wait for the agent to finish, or stop it before sending."
                  : "Enter to send · Shift+Enter for a new line"}
              </span>
              {running ? (
                <Button
                  type="button"
                  variant="outline"
                  disabled={
                    mutation.isPending || latest?.state === "cancel_requested"
                  }
                  onClick={() => mutation.mutate("cancel")}
                >
                  <Square size={13} /> Stop task
                </Button>
              ) : (
                <Button disabled={!canSend}>
                  <ArrowUp size={15} />
                  {mutation.isPending ? "Sending…" : "Send"}
                </Button>
              )}
            </div>
            {new TextEncoder().encode(message).length > 16000 && (
              <p
                id="work-followup-error"
                className="text-sm text-destructive"
                role="alert"
              >
                This follow-up exceeds 16 kB. Shorten it or split it into
                separate messages.
              </p>
            )}
          </Form>
        </div>
      </div>
      <div
        className="conversation-divider"
        role="separator"
        aria-label="Resize conversation and preview"
        aria-orientation="vertical"
        aria-valuemin={30}
        aria-valuemax={70}
        aria-valuenow={ratio}
        tabIndex={0}
        onKeyDown={(e) => {
          if (e.key === "ArrowLeft" || e.key === "ArrowRight") {
            e.preventDefault();
            setRatio((r) =>
              Math.max(30, Math.min(70, r + (e.key === "ArrowLeft" ? -2 : 2))),
            );
          }
        }}
        onPointerDown={(e) => {
          e.preventDefault();
          e.currentTarget.setPointerCapture(e.pointerId);
        }}
        onPointerMove={(e) => {
          if (!e.currentTarget.hasPointerCapture(e.pointerId)) return;
          const rect = split.current!.getBoundingClientRect();
          setRatio(
            Math.max(
              30,
              Math.min(70, ((e.clientX - rect.left) / rect.width) * 100),
            ),
          );
        }}
        onPointerUp={(e) => e.currentTarget.releasePointerCapture(e.pointerId)}
      />
      <WorkPreview
        project={project.id}
        workspace={item.workspace_id}
        revision={
          latest && !activeAttempt(latest.state) ? latest.id : undefined
        }
        onCollapse={collapsePreview}
        onReady={revealPreview}
        onSelection={selectElement}
      />
      {files && (
        <Modal title="Task files" wide onClose={() => setFiles(false)}>
          <div className="conversation-file-drawer">
            <div className="conversation-results-heading">
              <h3>Saved files</h3>
            </div>
            {latest ? (
              <Outputs
                key={latest.id}
                project={project.id}
                work={id}
                attempt={latest}
              />
            ) : (
              <p className="conversation-muted">
                Files you choose to retain will appear here when the agent
                finishes.
              </p>
            )}
            <details className="conversation-technical">
              <summary>Technical details</summary>
              <dl>
                <dt>Work</dt>
                <dd>{item.id}</dd>
                <dt>Worker</dt>
                <dd>{item.workspace_id}</dd>
                <dt>Attempt</dt>
                <dd>{latest?.id || "Not started"}</dd>
                <dt>Task</dt>
                <dd>{latest?.task_id || "Not accepted yet"}</dd>
                <dt>Specification</dt>
                <dd>{latest?.specification_path || item.specification_path}</dd>
              </dl>
            </details>
          </div>
        </Modal>
      )}
    </section>
  );
}

function DraftMessage({ project, item }: { project: string; item: WorkItem }) {
  const query = useQuery(instructionOptions(project, item));
  if (query.isError) return <ErrorNotice error={query.error} />;
  return (
    <article className="conversation-message is-user">
      <div className="conversation-speaker">
        You <span>Saved draft</span>
      </div>
      <div className="conversation-message-body">
        {query.data
          ? userMessage(query.data, {})
          : "Loading your saved request…"}
      </div>
    </article>
  );
}

function RunConversation({
  project,
  attempt,
  onOlder,
  onAsk,
  onActivity,
}: {
  project: string;
  attempt: Attempt;
  onOlder: () => void;
  onAsk: (message: string) => void;
  onActivity: () => void;
}) {
  const [older, setOlder] = useState<HistoryRecord[]>([]);
  const [cursor, setCursor] = useState<number>();
  const observed = useRef<Snapshot | undefined>(undefined);
  const history = useQuery({
    ...privateContentOptions,
    queryKey: [
      "project",
      project,
      "work-conversation",
      attempt.workspace_id,
      attempt.id,
      attempt.state,
    ],
    queryFn: async ({ signal }) => {
      const value = await refreshRunHistory(
        project,
        attempt.workspace_id,
        attempt.task_id!,
        observed.current,
        signal,
      );
      if (!signal.aborted) observed.current = value;
      return value;
    },
    enabled: !!attempt.task_id,
    refetchInterval: (query) =>
      activeAttempt(attempt.state) || query.state.data?.page.next_after
        ? 2000
        : false,
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
  const lastSequence = useRef<number | undefined>(undefined);
  const newestSequence = history.data?.page.records.reduce(
    (last, record) => Math.max(last, record.seq),
    0,
  );
  useEffect(() => {
    if (newestSequence === undefined) return;
    if (
      lastSequence.current !== undefined &&
      newestSequence > lastSequence.current
    )
      onActivity();
    lastSequence.current = newestSequence;
  }, [newestSequence, onActivity]);
  const records = [
    ...new Map(
      [
        ...older,
        ...(history.isError ? [] : history.data?.page.records || []),
      ].map((r) => [r.id, r]),
    ).values(),
  ].sort((a, b) => a.seq - b.seq);
  const result = useQuery(
    resultOptions(
      project,
      attempt.workspace_id,
      attempt.task_id,
      !!attempt.task_id &&
        !activeAttempt(attempt.state) &&
        !records.some((r) => r.kind === "assistant.message") &&
        !history.isPending,
    ),
  );
  const next = cursor ?? history.data?.page.next_before;
  return (
    <section className="conversation-run" aria-label="Task activity">
      <div className="conversation-run-heading">
        <time>{date(attempt.created_at)}</time>
      </div>
      {attempt.error_code && (
        <div className="conversation-notice" role="alert">
          <strong>{label(attempt.error_code)}</strong>
          <p>{new APIError(attempt.error_code, 409).message}</p>
        </div>
      )}
      {!!next && (
        <Button
          size="sm"
          variant="ghost"
          disabled={more.isPending}
          onClick={() => {
            onOlder();
            more.mutate();
          }}
        >
          Load earlier activity
        </Button>
      )}
      {more.isError && <ErrorNotice error={more.error} />}
      {history.isError && <ErrorNotice error={history.error} />}
      {history.data?.page.capture_limited && (
        <p role="alert">
          Recording storage is full. Some activity may be missing.
        </p>
      )}
      {history.isPending && attempt.task_id && (
        <p className="conversation-muted">Loading recorded activity…</p>
      )}
      {groupActivity(records).map((group) => {
        const actionCount =
          group.filter((record) => record.kind === "tool.call").length ||
          group.length;
        const renderRecord = (record: HistoryRecord) => {
          if (
            record.kind === "user.message" ||
            record.kind === "assistant.message"
          )
            return (
              <article
                key={record.id}
                className={`conversation-message ${record.kind === "user.message" ? "is-user" : "is-agent"}`}
              >
                <div className="conversation-speaker">
                  {record.kind === "user.message"
                    ? "You"
                    : label(record.source)}
                  <time title={date(record.at)}>
                    {new Date(record.at).toLocaleTimeString(undefined, {
                      hour: "2-digit",
                      minute: "2-digit",
                    })}
                  </time>
                </div>
                <div className="conversation-message-body">
                  {record.kind === "user.message" ? (
                    userMessage(record.content || "", attempt)
                  ) : (
                    <MessageContent
                      content={record.content || ""}
                      project={project}
                      attempt={attempt}
                      onAsk={onAsk}
                    />
                  )}
                </div>
                {record.truncated && (
                  <p className="conversation-muted">
                    This recorded message was shortened.
                  </p>
                )}
              </article>
            );
          if (record.kind === "tool.call" || record.kind === "tool.result")
            return (
              <details key={record.id} className="conversation-tool">
                <summary>
                  <Terminal size={13} />
                  <span>
                    {label(record.tool || "Tool")}{" "}
                    {record.kind === "tool.result" ? "result" : "call"}
                  </span>
                  {record.status && (
                    <span className="conversation-tool-status">
                      {label(record.status)}
                    </span>
                  )}
                </summary>
                <pre>{record.content || "No output recorded."}</pre>
                {record.truncated && (
                  <p className="conversation-muted">
                    Output shortened in the recording.
                  </p>
                )}
              </details>
            );
          if (record.kind === "capture.gap")
            return (
              <p key={record.id} className="conversation-muted">
                Some activity was not recorded.
              </p>
            );
          return null;
        };
        return group[0].kind.startsWith("tool.") ? (
          <details className="conversation-activity" key={group[0].id}>
            <summary>
              {actionCount} tool {actionCount === 1 ? "action" : "actions"}
              {group.some((r) => r.status === "failed")
                ? " · includes errors"
                : ""}
            </summary>
            {group.map(renderRecord)}
          </details>
        ) : (
          renderRecord(group[0])
        );
      })}
      {result.data?.content && (
        <article className="conversation-message is-agent">
          <div className="conversation-speaker">Agent reply</div>
          <div className="conversation-message-body">
            <MessageContent
              content={result.data.content}
              project={project}
              attempt={attempt}
              onAsk={onAsk}
            />
          </div>
          {result.data.next_offset !== -1 && (
            <p className="conversation-muted">
              This view shows the first 64 KiB of the reply.
            </p>
          )}
        </article>
      )}
      {result.isError && <ErrorNotice error={result.error} />}
    </section>
  );
}

function Outputs({
  project,
  work,
  attempt,
}: {
  project: string;
  work: string;
  attempt: Attempt;
}) {
  const [file, setFile] = useState<Artifact>();
  const url = evidenceURL(file?.content_url);
  const preview = useQuery({
    ...privateContentOptions,
    queryKey: [
      "project",
      project,
      "work-output",
      work,
      attempt.id,
      file?.id || file?.content_url,
    ],
    queryFn: ({ signal }) => readOutput(url!, signal),
    enabled: !!file && !!url?.startsWith("/ui-api/"),
  });
  return (
    <>
      {!attempt.artifacts?.length && (
        <p className="conversation-muted">
          {activeAttempt(attempt.state)
            ? "Saved outputs will appear when the task finishes."
            : "No output files were retained for this task. The agent's reply is in the conversation."}
        </p>
      )}
      <ul className="conversation-files">
        {attempt.artifacts?.map((a, i) => {
          const name = a.source_path || a.name || a.kind || "Output";
          const link = evidenceURL(a.content_url);
          const text =
            /\.(md|txt|json|csv|py|go|js|jsx|ts|tsx|yaml|yml|html|css|log|toml)$/i.test(
              name,
            );
          return (
            <li key={a.id || i}>
              <FileText size={15} />
              <div>
                <span>{name}</span>
                <small>{bytes(a.size_bytes)}</small>
                <div className="conversation-file-actions">
                  {text &&
                    link?.startsWith("/ui-api/") &&
                    (a.size_bytes ?? 0) <= 262144 && (
                      <button onClick={() => setFile(a)}>Inspect</button>
                    )}
                  {link && (
                    <a href={link} download>
                      <Download size={12} /> Download
                    </a>
                  )}
                </div>
              </div>
            </li>
          );
        })}
      </ul>
      {file && (
        <section
          className="conversation-output-preview"
          aria-label="Output preview"
        >
          <div>
            <strong>{file.source_path || file.name}</strong>
            <button onClick={() => setFile(undefined)}>Close preview</button>
          </div>
          {preview.isPending && <p>Loading output…</p>}
          {preview.isError && <p role="alert">{preview.error.message}</p>}
          {preview.data !== undefined && <pre>{preview.data}</pre>}
        </section>
      )}
    </>
  );
}

function groupActivity(records: HistoryRecord[]): HistoryRecord[][] {
  const groups: HistoryRecord[][] = [];
  for (const record of records) {
    const last = groups.at(-1);
    if (record.kind.startsWith("tool.") && last?.[0].kind.startsWith("tool."))
      last.push(record);
    else groups.push([record]);
  }
  return groups;
}
