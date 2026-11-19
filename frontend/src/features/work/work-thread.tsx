import { WorkspaceSettings } from "@/features/workspaces/settings";
import { Responsibility } from "./responsibility";
import { usePersonalWork, useLayoutPreference } from "./personal-work";
import { AgentSelection, type AgentModel } from "./agent-selection";
import { Link, useSearch, useNavigate } from "@tanstack/react-router";
import { replyOutputs } from "./reply-outputs";
import { LinkedOutputs } from "./linked-outputs";
import { instructionOptions } from "./conversation-title";
import { composerKeyDown } from "./composer-keydown";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
  type Dispatch,
  type SetStateAction,
} from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowDown,
  ArrowUp,
  RotateCcw,
  CircleAlert,
  Server,
  Play,
  FileText,
  Loader2,
  Square,
  Terminal,
  Download,
  PanelRightOpen,
  BarChart3,
  MessageSquare,
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

export function WorkConversation({
  project,
  id,
  startError,
  title,
  navigation,
  message,
  setMessage,
  owner,
}: {
  project: Project;
  id: string;
  startError?: unknown;
  title?: string;
  owner?: string;
  navigation?: ReactNode;
  message: string;
  setMessage: Dispatch<SetStateAction<string>>;
}) {
  const client = useQueryClient();
  const key = useRequestKey();
  const [earlier, setEarlier] = useState(false);
  const [files, setFiles] = useState(false);
  const personal = usePersonalWork();
  const route = useSearch({ strict: false });
  const navigate = useNavigate();
  const [layout, setLayout] = useLayoutPreference<string>("output", "closed");
  // Existing split/output preferences now open the unified workspace.
  const workspaceOpen = layout !== "closed";
  const workspaceVisible = useRef(workspaceOpen);
  workspaceVisible.current = workspaceOpen;
  const [unreadReply, setUnreadReply] = useState(false);
  const observedReplies = useRef(new Map<string, string>());
  const revealPreview = useCallback(() => {}, []);
  const [visuals, setVisuals] = useState(false);
  const [visualsOpen, setVisualsOpen] = useState(false);
  const [visualRequest, setVisualRequest] = useState(0);
  const [reply, setReply] = useState({ attempt: "", content: "" });
  const recordReply = useCallback((attempt: string, content: string) => {
    const previous = observedReplies.current.get(attempt);
    if (
      previous !== undefined &&
      content &&
      content !== previous &&
      workspaceVisible.current
    )
      setUnreadReply(true);
    if (content || previous === undefined)
      observedReplies.current.set(attempt, content);
    setReply((old) =>
      old.attempt === attempt && old.content === content
        ? old
        : { attempt, content },
    );
  }, []);
  const selectElement = useCallback(
    (text: string) => {
      setMessage((old) => (old ? old + "\n" + text : text));
      requestAnimationFrame(() => composer.current?.focus());
    },
    [setMessage],
  );
  const composer = useRef<HTMLTextAreaElement>(null);
  const feed = useRef<HTMLDivElement>(null);
  const attachFeed = useCallback(
    (node: HTMLDivElement | null) => {
      if (node) node.scrollTop = personal.positions.current.get(id) || 0;
      feed.current = node;
    },
    [id, personal.positions],
  );
  const restorePosition = useRef(personal.positions.current.get(id));
  const follow = useRef(!personal.positions.current.has(id) && !route.attempt);
  const [newActivity, setNewActivity] = useState(false);
  const announceActivity = useCallback(() => {
    if (!workspaceVisible.current && !follow.current) setNewActivity(true);
  }, []);
  const showConversation = (latestMessage = false) => {
    setLayout("closed");
    setUnreadReply(false);
    if (latestMessage) follow.current = true;
    requestAnimationFrame(() => {
      const el = feed.current;
      if (el)
        el.scrollTop = latestMessage
          ? el.scrollHeight
          : personal.positions.current.get(id) || 0;
      if (latestMessage) el?.focus({ preventScroll: true });
    });
  };
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
  const outputSources = replyOutputs(
    reply.attempt === latest?.id ? reply.content : "",
  );
  const actions = item && workActions(item.state);
  const running =
    !!item && (actions?.cancel || (!!latest && activeAttempt(latest.state)));
  const worker = workers.data?.find((w) => w.id === item?.workspace_id);
  const [subscriptionSettings, setSubscriptionSettings] = useState(false);
  const [selection, setSelection] = useState<AgentModel>();
  const subscriptionAction = (error: unknown) => {
    if (
      !worker ||
      !error ||
      typeof error !== "object" ||
      !("code" in error) ||
      error.code !== "PROVIDER_LOGIN_REQUIRED"
    )
      return null;
    return (
      <Button
        type="button"
        variant="outline"
        onClick={() => setSubscriptionSettings(true)}
      >
        Open worker subscription settings
      </Button>
    );
  };
  const [actionSequence, setActionSequence] = useState(0);
  const mutation = useMutation({
    mutationFn: (action: "run" | "cancel" | "accept" | "continue" | "wake") => {
      const delivery = key({
        action,
        actionSequence,
        id,
        message,
        selection,
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
        selection,
      );
    },
    onSuccess: async (_, action) => {
      setActionSequence((v) => v + 1);
      if (action === "continue") {
        setMessage("");
        follow.current = true;
        if (route.attempt) {
          await navigate({
            to: "/projects/$projectId/$section",
            params: { projectId: project.id, section: "work" },
            search: { work: id },
            replace: true,
          });
        }
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
      if (workspaceVisible.current) return;
      if (restorePosition.current !== undefined) {
        el.scrollTop = restorePosition.current;
        if (el.scrollHeight - el.clientHeight >= restorePosition.current)
          restorePosition.current = undefined;
      } else if (follow.current) el.scrollTop = el.scrollHeight;
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
      "changes_requested",
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
  const retry = latest && !["planned", "in_progress"].includes(item.state);
  const runAction = actions?.run ? (
    <div className="conversation-message-actions">
      <Button
        variant="outline"
        size="default"
        disabled={mutation.isPending}
        onClick={() => mutation.mutate("run")}
      >
        {mutation.isPending && mutation.variables === "run" ? (
          <Loader2 size={14} className="loading-spinner" />
        ) : retry ? (
          <RotateCcw size={14} />
        ) : (
          <Play size={14} />
        )}
        {retry ? "Retry task" : "Start task"}
      </Button>
      {mutation.isError && mutation.variables === "run" && (
        <>
          <ErrorNotice error={mutation.error} />
          {subscriptionAction(mutation.error)}
        </>
      )}
      {!!startError && item.state === "draft" && (
        <>
          <ErrorNotice error={startError} />
          {subscriptionAction(startError)}
        </>
      )}
    </div>
  ) : null;
  return (
    <section
      className={`conversation-session visual-session unified-thread ${workspaceOpen ? "is-workspace-open" : ""}`}
      aria-label="Work conversation"
    >
      {subscriptionSettings && worker && (
        <WorkspaceSettings
          workspace={worker}
          initialSection="subscriptions"
          onClose={() => setSubscriptionSettings(false)}
          onSaved={() => setSubscriptionSettings(false)}
        />
      )}
      <div className="conversation-main">
        <header className="conversation-thread-heading">
          <div className="conversation-heading-title">
            {navigation}
            <div className="min-w-0">
              <h2 title={title} className="line-clamp-2">
                {title || `${label(item.kind)} conversation`}
              </h2>
              <div className="thread-heading-state">
                <State value={workStatus(item.state)} />
              </div>
            </div>
          </div>
          {latest &&
            attempts.some(
              (a) => a.id !== latest.id && a.review_state === "accepted",
            ) && (
              <span className="text-xs text-muted-foreground">
                Previous output approved · Current work:{" "}
                {workStatus(item.state)}
              </span>
            )}
          <div className="conversation-heading-actions">
            <Responsibility item={item} running={running}>
              <Link
                className="conversation-worker-link"
                to="/projects/$projectId/$section"
                params={{ projectId: project.id, section: "workspaces" }}
                search={{ worker: item.workspace_id }}
                title={item.workspace_id}
              >
                <Server size={14} /> Worker: {worker?.name || "Task worker"}
              </Link>
              <p>Created {date(item.created_at)}</p>
              {owner && <p>Task assignee: {owner}</p>}
            </Responsibility>
            <div
              className="thread-view-switch"
              role="group"
              aria-label="Thread view"
            >
              <Button
                variant="ghost"
                size="sm"
                aria-pressed={!workspaceOpen}
                onClick={() => showConversation(unreadReply)}
              >
                <MessageSquare size={15} /> Conversation
                {unreadReply && (
                  <span
                    className="thread-unread-dot"
                    aria-label="New agent message"
                  />
                )}
              </Button>
              <Button
                variant="ghost"
                size="sm"
                aria-pressed={workspaceOpen}
                onClick={() => setLayout("workspace")}
              >
                <PanelRightOpen size={15} /> Workspace
              </Button>
            </div>
            {actions?.review && (
              <Button asChild variant="outline" size="sm">
                <Link
                  to="/projects/$projectId/$section"
                  params={{ projectId: project.id, section: "reviews" }}
                  search={{ work: id, attempt: item.latest_attempt_id }}
                >
                  Review output
                </Link>
              </Button>
            )}
          </div>
        </header>
        <div
          className="conversation-feed"
          hidden={workspaceOpen}
          onWheel={() => {
            restorePosition.current = undefined;
          }}
          onPointerDown={() => {
            restorePosition.current = undefined;
          }}
          onKeyDown={() => {
            restorePosition.current = undefined;
          }}
          ref={attachFeed}
          onScroll={() => {
            const el = feed.current!;
            if (workspaceVisible.current) return;
            if (restorePosition.current !== undefined) return;
            personal.positions.current.set(id, el.scrollTop);
            follow.current =
              el.scrollHeight - el.scrollTop - el.clientHeight < 100;
            if (follow.current) setNewActivity(false);
          }}
          aria-label="Conversation and agent activity"
          role="region"
          tabIndex={0}
        >
          <div className="conversation-timeline">
            {attemptQuery.isError && <ErrorNotice error={attemptQuery.error} />}
            {route.attempt && (
              <p className="review-context">
                Viewing output revision {route.attempt}.{" "}
                <Link
                  to="/projects/$projectId/$section"
                  params={{ projectId: project.id, section: "work" }}
                  search={{ work: id }}
                >
                  Return to current conversation
                </Link>
              </p>
            )}
            {!attempts.length &&
              !attemptQuery.isPending &&
              !attemptQuery.isError && (
                <DraftMessage
                  project={project.id}
                  item={item}
                  action={runAction}
                />
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
              <Button
                size="sm"
                variant="ghost"
                onClick={() => setEarlier(false)}
              >
                Hide earlier conversation
              </Button>
            )}
            {(route.attempt
              ? attempts.filter((a) => a.id === route.attempt)
              : earlier
                ? attempts
                : attempts.slice(-1)
            ).map((a) => (
              <RunConversation
                key={a.id}
                project={project.id}
                attempt={a}
                action={a.id === latest?.id ? runAction : null}
                onReply={a.id === latest?.id ? recordReply : undefined}
                onAsk={selectElement}
                onActivity={announceActivity}
                onOlder={() => {
                  follow.current = false;
                }}
              />
            ))}
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
        </div>
        {newActivity && !workspaceOpen && (
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
        <section
          className="thread-workspace"
          aria-label="Task workspace"
          hidden={!workspaceOpen}
        >
          <div
            className="thread-workspace-tools"
            role="group"
            aria-label="Workspace tools"
          >
            <Button
              variant="ghost"
              size="sm"
              aria-pressed={!visualsOpen}
              onClick={() => setVisualsOpen(false)}
            >
              <PanelRightOpen size={15} /> Preview
            </Button>
            <Button variant="ghost" size="sm" onClick={() => setFiles(true)}>
              <FileText size={15} /> Files
            </Button>
            <Button
              variant="ghost"
              size="sm"
              aria-pressed={visualsOpen}
              onClick={() => {
                setVisuals(true);
                setVisualsOpen(true);
                setVisualRequest((v) => v + 1);
              }}
            >
              <BarChart3 size={15} /> Visualize data
            </Button>
          </div>
          <div className="thread-workspace-preview" hidden={visualsOpen}>
            <WorkPreview
              project={project.id}
              workspace={item.workspace_id}
              revision={
                latest && !activeAttempt(latest.state) ? latest.id : undefined
              }
              collapsible={false}
              onCollapse={() => showConversation()}
              onReady={revealPreview}
              onSelection={selectElement}
            />
          </div>
          {visuals && (
            <section
              className="conversation-visualization"
              aria-label="Conversation visualization"
              hidden={!visualsOpen}
            >
              <WorkVisuals
                key={latest?.id || "draft"}
                project={project.id}
                onAsk={selectElement}
                artifacts={latest?.artifacts || []}
                initialEditing={visuals}
                openRequest={visualRequest}
                sources={outputSources}
                attempt={latest}
                onDismiss={() => setVisualsOpen(false)}
              />
            </section>
          )}
        </section>
        {workspaceOpen && unreadReply && (
          <div className="thread-message-notice" role="status">
            <MessageSquare size={16} />
            <span>New agent message</span>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => showConversation(true)}
            >
              View message
            </Button>
          </div>
        )}
        {workspaceOpen && runAction && (
          <div className="thread-workspace-recovery">{runAction}</div>
        )}
        <div className="conversation-composer">
          {mutation.isError && mutation.variables !== "run" && (
            <>
              <ErrorNotice error={mutation.error} />
              {subscriptionAction(mutation.error)}
            </>
          )}
          <Form
            onSubmit={() => {
              if (canSend) mutation.mutate("continue");
            }}
          >
            <label className="sr-only" htmlFor="work-followup">
              Message to your agent
            </label>
            <div className="conversation-compose-input">
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
                rows={1}
                value={message}
                onChange={(e) => setMessage(e.target.value)}
                onKeyDown={(e) => composerKeyDown(e, canSend)}
                placeholder={
                  running
                    ? "Draft your next instruction…"
                    : "Tell the agent what to do next…"
                }
                disabled={!item.latest_attempt_id || mutation.isPending}
              />
              <div className="thread-composer-footer">
                <AgentSelection
                  compact
                  workspace={item.workspace_id}
                  provider={selection?.provider ?? item.provider ?? ""}
                  model={selection?.model ?? item.model ?? ""}
                  reasoning={
                    selection
                      ? selection.reasoning_effort || ""
                      : item.reasoning_effort || ""
                  }
                  onReasoning={(effort, provider, model) =>
                    setSelection({ provider, model, reasoning_effort: effort })
                  }
                  disabled={running || mutation.isPending}
                  onProvider={(provider) =>
                    setSelection(provider ? { provider, model: "" } : undefined)
                  }
                  onModel={(model) =>
                    setSelection((value) =>
                      value ? { ...value, model } : undefined,
                    )
                  }
                />
                <div className="conversation-compose-actions">
                  {running ? (
                    <Button
                      type="button"
                      variant="outline"
                      size="default"
                      aria-label="Stop task"
                      disabled={
                        mutation.isPending ||
                        latest?.state === "cancel_requested"
                      }
                      onClick={() => mutation.mutate("cancel")}
                    >
                      <Square size={14} />
                      {latest?.state === "cancel_requested"
                        ? "Stopping…"
                        : "Stop"}
                    </Button>
                  ) : (
                    <Button
                      disabled={!canSend}
                      size="default"
                      aria-label="Send"
                    >
                      {mutation.isPending ? (
                        <Loader2 size={16} className="loading-spinner" />
                      ) : (
                        <ArrowUp size={16} />
                      )}
                      Send
                    </Button>
                  )}
                </div>
              </div>
            </div>
            {running && (
              <p className="thread-composer-hint">
                Stop the run to send new instructions.
              </p>
            )}
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
      {files && (
        <Modal title="Task files" wide onClose={() => setFiles(false)}>
          <div className="conversation-file-drawer">
            {!!latest?.artifacts?.length && (
              <div className="conversation-results-heading">
                <h3>Saved files</h3>
              </div>
            )}
            {latest &&
            (latest.artifacts?.length || !outputSources.some((s) => s.path)) ? (
              <Outputs
                key={latest.id}
                project={project.id}
                work={id}
                attempt={latest}
              />
            ) : !latest ? (
              <p className="conversation-muted">No files yet.</p>
            ) : null}
            {latest && (
              <LinkedOutputs
                key={latest.id}
                project={project.id}
                attempt={latest}
                sources={outputSources}
              />
            )}
            <details className="conversation-technical">
              <summary>Technical details</summary>
              <dl>
                <dt>Work</dt>
                <dd>{item.id}</dd>
                <dt>Worker</dt>
                <dd>{item.workspace_id}</dd>
                <dt>Agent</dt>
                <dd>{item.provider || "Worker default"}</dd>
                <dt>Model</dt>
                <dd>{item.model || "Configured default"}</dd>
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

function DraftMessage({
  project,
  item,
  action,
}: {
  project: string;
  item: WorkItem;
  action?: ReactNode;
}) {
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
      {action}
    </article>
  );
}

function RunConversation({
  project,
  attempt,
  onOlder,
  onAsk,
  onActivity,
  action,
  onReply,
}: {
  project: string;
  attempt: Attempt;
  action?: ReactNode;
  onReply?: (attempt: string, content: string) => void;
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
  const replyContent =
    records
      .filter((r) => r.kind === "assistant.message")
      .map((r) => r.content || "")
      .join("\n\n") ||
    result.data?.content ||
    "";
  useEffect(() => {
    onReply?.(attempt.id, replyContent);
  }, [onReply, attempt.id, replyContent]);
  const lastMessage = records
    .filter((r) => r.kind === "user.message" || r.kind === "assistant.message")
    .at(-1);
  const next = cursor ?? history.data?.page.next_before;
  return (
    <section className="conversation-run" aria-label="Task activity">
      <div className="conversation-run-heading">
        <time>{date(attempt.created_at)}</time>
      </div>
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
                {!attempt.error_code &&
                  !result.data?.content &&
                  record.id === lastMessage?.id &&
                  action}
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
          {!attempt.error_code && action}
          {result.data.next_offset !== -1 && (
            <p className="conversation-muted">
              This view shows the first 64 KiB of the reply.
            </p>
          )}
        </article>
      )}
      {attempt.error_code && (
        <article
          className="conversation-notice conversation-failure"
          role="alert"
        >
          <div className="conversation-failure-heading">
            <CircleAlert size={16} aria-hidden="true" />
            <strong>{label(attempt.error_code)}</strong>
          </div>
          <p>{new APIError(attempt.error_code, 409).message}</p>
          {action}
        </article>
      )}
      {result.isError && <ErrorNotice error={result.error} />}
      {!attempt.error_code &&
        !lastMessage &&
        !result.data?.content &&
        action && (
          <article className="conversation-message is-agent">{action}</article>
        )}
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
