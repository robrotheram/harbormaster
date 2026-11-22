import "@/features/delivery/review-feedback.css";
import {
  AreaComments,
  ReviewPreviewMarkup,
  renderReviewAreas,
  type ReviewArea,
} from "@/features/delivery/review-preview-markup";
import { ReviewScreenshots } from "@/features/delivery/review-screenshots";
import {
  ReviewScreenshotEditor,
  type ReviewScreenshot,
} from "@/features/delivery/review-screenshot-editor";
import {
  maxReviewScreenshots,
  readScreenshot,
} from "@/features/delivery/review-screenshot-image";
import { boardKey } from "@/features/delivery/board-api";
import { useUserSession } from "@/features/auth/session";
import { WorkspaceSettings } from "@/features/workspaces/settings";
import { Responsibility, memberOptions } from "./responsibility";
import { usePersonalWork, useLayoutPreference } from "./personal-work";
import { AgentSelection, type AgentModel } from "./agent-selection";
import { Link, useSearch, useNavigate } from "@tanstack/react-router";
import { replyOutputs } from "./reply-outputs";
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
  PanelRightOpen,
  MessageSquare,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/native-select";
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
import { date, label } from "@/shared/format";
import type { Project } from "@/shared/types";
import { workspacesApi, workspaceOptions } from "@/features/workspaces/api";
import type { HistoryRecord, Snapshot } from "@/features/history/history-api";
import { resultOptions } from "@/features/tasks/api";
import {
  workActions,
  workApi,
  workKeys,
  type Attempt,
  type WorkItem,
} from "./api";
import {
  activeAttempt,
  runHistory,
  refreshRunHistory,
  userMessage,
} from "./conversation-api";
import { WorkFiles } from "./work-files";
import { WorkOutput, retainedVisuals } from "./work-output";
import { workStatus } from "./stages";
import {
  reviewImageNote,
  reviewMessageText,
  submittedReviewImage,
} from "./review-image";
import { WorkPreview, type PreviewView } from "./work-preview";
import { MessageContent, WorkspaceImage } from "./message-content";

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
  const [previewView, setPreviewView] = useState<PreviewView>();
  const personal = usePersonalWork();
  const route = useSearch({ strict: false });
  const navigate = useNavigate();
  const [layout, setLayout] = useLayoutPreference<string>("output", "closed");
  const filesOpen = route.view === "files" && route.work === id;
  useEffect(() => {
    if (route.view) setLayout("workspace");
  }, [route.view, id]);
  const workspaceOpen = layout !== "closed" || !!route.view;
  const workspaceVisible = useRef(workspaceOpen);
  workspaceVisible.current = workspaceOpen;
  const [unreadReply, setUnreadReply] = useState(false);
  const observedReplies = useRef(new Map<string, string>());
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
    if (route.view)
      void navigate({
        to: "/projects/$projectId/$section",
        params: { projectId: project.id, section: "work" },
        search: { work: id, attempt: route.attempt },
        replace: true,
      });
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
  const fileAttempt = route.attempt
    ? attempts.find((a) => a.id === route.attempt)
    : latest;
  const output = reply.attempt === fileAttempt?.id ? reply.content : "";
  const outputSources = replyOutputs(output);
  const visualFiles = fileAttempt
    ? retainedVisuals(fileAttempt, outputSources)
    : [];
  const hasOutput = outputSources.length > 0 || visualFiles.length > 0;
  const actions = item && workActions(item.state);
  const running =
    !!item && (actions?.cancel || (!!latest && activeAttempt(latest.state)));
  const worker = workers.data?.find((w) => w.id === item?.workspace_id);
  const [subscriptionSettings, setSubscriptionSettings] = useState(false);
  const [selection, setSelection] = useState<AgentModel>();
  const [move, setMove] = useState<{ work: string; worker: string }>();
  const destination = move?.work === id ? move.worker : undefined;
  const alternateWorkers =
    workers.data?.filter(
      (candidate) =>
        candidate.id !== item?.workspace_id &&
        candidate.project_id === project.id &&
        !candidate.shared &&
        !["STOPPING", "UNKNOWN"].includes(candidate.state),
    ) || [];
  const executionWorker =
    alternateWorkers.find((candidate) => candidate.id === destination) ||
    worker;
  const subscriptionAction = (error: unknown) => {
    if (
      !executionWorker ||
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
  const session = useUserSession();
  const roster = useQuery(memberOptions(project.id));
  const local = session.status === "local";
  const user = roster.data?.user_id;
  const controlsAgent = local || (!!user && item?.controller_id === user);
  const canDecide =
    local || (!!user && (!item?.reviewer_id || item.reviewer_id === user));
  const canReview =
    !!actions?.review &&
    !!latest &&
    fileAttempt?.id === latest.id &&
    !latest.review_state &&
    !attemptQuery.isError &&
    canDecide;
  const draftID = `${project.id}:${id}:${item?.latest_attempt_id}`;
  const areaDraft = personal.reviewAreas[draftID];
  const screenshotDraft = personal.reviewScreenshots[draftID];
  const areas = areaDraft?.areas || [];
  const shots = screenshotDraft?.shots || [];
  const screenshotLimit = maxReviewScreenshots - (areas.length ? 1 : 0);
  const [capturingArea, setCapturingArea] = useState(false);
  const [readingImage, setReadingImage] = useState(false);
  const [imageError, setImageError] = useState("");
  const [screenshotEditor, setScreenshotEditor] = useState<{
    index: number;
    trigger: HTMLElement;
  }>();
  const compiledImage = useRef<{ key: string; content: string }>(null);
  const instructionText = [
    message,
    areas.length
      ? "Apply the numbered changes in the attached annotated preview image.\n" +
        areas
          .map(
            ({ comment, path }, i) =>
              `${i + 1}. ${comment.trim()} (Preview page ${path})`,
          )
          .join("\n")
      : "",
  ]
    .filter(Boolean)
    .join("\n\n");
  const messageTooLong =
    new TextEncoder().encode(instructionText).length > 16000;
  function updateAreas(next: ReviewArea[]) {
    personal.setReviewAreas((old) => ({
      ...old,
      [draftID]: { areas: next, key: crypto.randomUUID() },
    }));
  }
  function updateShots(next: ReviewScreenshot[]) {
    personal.setReviewScreenshots((old) => ({
      ...old,
      [draftID]: { shots: next, key: crypto.randomUUID() },
    }));
  }
  async function attachImage(file: File) {
    setReadingImage(true);
    setImageError("");
    try {
      const url = await readScreenshot(file);
      updateShots([...shots, { url, note: "" }]);
    } catch (e) {
      setImageError(e instanceof Error ? e.message : "Unable to attach image.");
    } finally {
      setReadingImage(false);
    }
  }
  const [changeRequest, setChangeRequest] = useState<{
    attempt: string;
    feedback: string;
  }>();
  const [actionSequence, setActionSequence] = useState(0);
  const mutation = useMutation({
    mutationFn: async (
      action:
        "run" | "cancel" | "accept" | "request-changes" | "continue" | "wake",
    ) => {
      const delivery = key({
        action,
        actionSequence,
        id,
        message,
        selection,
        destination,
        areas: areaDraft?.key,
        screenshots: screenshotDraft?.key,
        attempt: item?.latest_attempt_id,
      });
      if (action === "run")
        return workApi.run(project.id, id, delivery, selection);
      if (action === "cancel") return workApi.cancel(project.id, id);
      if (action === "accept")
        return workApi.review(project.id, id, true, item?.latest_attempt_id);
      if (action === "request-changes")
        return workApi.review(
          project.id,
          id,
          false,
          changeRequest?.attempt,
          changeRequest?.feedback.trim(),
        );
      if (action === "wake")
        return workspacesApi.start(item!.workspace_id, delivery);
      const screenshots = shots.map(({ url, note }) => ({
        content: url.split(",")[1],
        note,
      }));
      if (areaDraft && areas.length) {
        if (compiledImage.current?.key !== areaDraft.key) {
          const image = await renderReviewAreas(areas);
          compiledImage.current = {
            key: areaDraft.key,
            content: image.split(",")[1],
          };
        }
        screenshots.unshift({
          content: compiledImage.current.content,
          note: reviewImageNote,
        });
      }
      return workApi.continue(
        project.id,
        id,
        instructionText,
        item!.latest_attempt_id!,
        delivery,
        selection,
        screenshots,
        destination,
      );
    },
    onMutate: () => draftID,
    onSuccess: async (_, action, submittedDraft) => {
      setActionSequence((v) => v + 1);
      if (action === "continue") {
        setMove(undefined);
        setMessage("");
        personal.setReviewAreas((old) => {
          const next = { ...old };
          delete next[submittedDraft];
          return next;
        });
        personal.setReviewScreenshots((old) => {
          const next = { ...old };
          delete next[submittedDraft];
          return next;
        });
        compiledImage.current = null;
        follow.current = true;
        if (route.attempt) {
          await navigate({
            to: "/projects/$projectId/$section",
            params: { projectId: project.id, section: "work" },
            search: { work: id, view: route.view },
            replace: true,
          });
        }
      }
      if (action === "request-changes") setChangeRequest(undefined);
      await Promise.all([
        client.invalidateQueries({ queryKey: workKeys.list(project.id) }),
        client.invalidateQueries({ queryKey: boardKey(project.id) }),
      ]);
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
    (!destination || alternateWorkers.some((w) => w.id === destination)) &&
    !capturingArea &&
    !readingImage &&
    !areas.some(({ comment }) => !comment.trim()) &&
    shots.length <= screenshotLimit &&
    !!instructionText.trim() &&
    !messageTooLong;
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
      {subscriptionSettings && executionWorker && (
        <WorkspaceSettings
          workspace={executionWorker}
          initialSection="subscriptions"
          onClose={() => setSubscriptionSettings(false)}
          onSaved={() => setSubscriptionSettings(false)}
        />
      )}
      {changeRequest && (
        <Modal
          title="Request changes"
          onClose={() => setChangeRequest(undefined)}
          closeDisabled={mutation.isPending}
        >
          <form
            onSubmit={(event) => {
              event.preventDefault();
              if (changeRequest.feedback.trim())
                mutation.mutate("request-changes");
            }}
          >
            <label>
              What needs to change?
              <Textarea
                autoFocus
                required
                maxLength={1000}
                value={changeRequest.feedback}
                onChange={(event) =>
                  setChangeRequest({
                    ...changeRequest,
                    feedback: event.target.value,
                  })
                }
                disabled={mutation.isPending}
              />
            </label>
            {mutation.error && <ErrorNotice error={mutation.error} />}
            <Button
              type="submit"
              disabled={mutation.isPending || !changeRequest.feedback.trim()}
            >
              Send change request
            </Button>
          </form>
        </Modal>
      )}
      <div className="conversation-main">
        <header className="conversation-thread-heading">
          <div className="conversation-heading-title">
            {navigation}
            <div className="min-w-0">
              <h2 title={title} className="line-clamp-2">
                {title || `${label(item.kind)} conversation`}
              </h2>
              {item.state !== "closed" && (
                <div className="thread-heading-state">
                  <State value={workStatus(item.state)} />
                </div>
              )}
            </div>
          </div>
          {workspaceOpen &&
            (fileAttempt?.review_state === "accepted" ||
              fileAttempt?.review_state === "changes_requested") && (
              <span className="text-xs text-muted-foreground">
                {fileAttempt.review_state === "accepted"
                  ? "This revision was approved."
                  : "Changes requested. The controller can prepare the next revision."}
              </span>
            )}
          {latest &&
            attempts.some(
              (a) => a.id !== latest.id && a.review_state === "accepted",
            ) && (
              <span className="text-xs text-muted-foreground">
                Previous output approved
              </span>
            )}
          <div className="conversation-heading-actions">
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
            {workspaceOpen && canReview && (
              <>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={mutation.isPending}
                  onClick={() => mutation.mutate("accept")}
                >
                  Approve this revision
                </Button>
                {!controlsAgent && (
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={mutation.isPending}
                    onClick={() => {
                      if (!latest) return;
                      mutation.reset();
                      setChangeRequest({ attempt: latest.id, feedback: "" });
                    }}
                  >
                    Request changes
                  </Button>
                )}
              </>
            )}
          </div>
        </header>
        {(workspaceOpen ? fileAttempt : latest)?.review_feedback && (
          <div className="task-guidance whitespace-pre-wrap">
            <strong>Requested changes</strong>
            <div>{(workspaceOpen ? fileAttempt : latest)?.review_feedback}</div>
          </div>
        )}
        <div
          className="thread-workspace-tools"
          role="group"
          aria-label={workspaceOpen ? "Workspace tools" : "Conversation tools"}
        >
          {workspaceOpen && (
            <>
              <Button
                variant="ghost"
                size="sm"
                aria-pressed={!filesOpen}
                onClick={() => {
                  if (filesOpen) {
                    void navigate({
                      to: "/projects/$projectId/$section",
                      params: { projectId: project.id, section: "work" },
                      search: {
                        work: id,
                        attempt: route.attempt,
                        view: "workspace",
                      },
                    });
                  } else setPreviewView("web");
                }}
              >
                <PanelRightOpen size={15} /> Preview
              </Button>
              <Button
                variant="ghost"
                size="sm"
                aria-pressed={filesOpen}
                onClick={() =>
                  void navigate({
                    to: "/projects/$projectId/$section",
                    params: { projectId: project.id, section: "work" },
                    search: { work: id, attempt: route.attempt, view: "files" },
                  })
                }
              >
                <FileText size={15} /> Files
              </Button>
            </>
          )}
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
        </div>
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
                  onLoad={
                    worker?.state === "STOPPED"
                      ? () => mutation.mutate("wake")
                      : undefined
                  }
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
                onLoad={
                  worker?.state === "STOPPED" && a.workspace_id === worker.id
                    ? () => mutation.mutate("wake")
                    : undefined
                }
                action={a.id === latest?.id ? runAction : null}
                onReply={a.id === fileAttempt?.id ? recordReply : undefined}
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
          hidden={!workspaceOpen || filesOpen}
        >
          <WorkPreview
            project={project.id}
            workspace={item.workspace_id}
            revision={
              latest && !activeAttempt(latest.state) ? latest.id : undefined
            }
            view={previewView}
            onViewChange={setPreviewView}
            onSelection={selectElement}
            output={
              workspaceOpen && hasOutput && fileAttempt ? (
                <WorkOutput
                  key={fileAttempt.id}
                  project={project.id}
                  attempt={fileAttempt}
                  sources={outputSources}
                  files={visualFiles}
                  onAsk={selectElement}
                />
              ) : undefined
            }
            renderReview={
              workspaceOpen &&
              !filesOpen &&
              controlsAgent &&
              !!latest &&
              fileAttempt?.id === latest.id &&
              !running &&
              !attemptQuery.isError
                ? ({ frame, origin, service }) => (
                    <ReviewPreviewMarkup
                      key={origin + service}
                      frame={frame}
                      origin={origin}
                      service={service}
                      areas={areas}
                      disabled={mutation.isPending}
                      onArea={(area) => updateAreas([...areas, area])}
                      onBusy={setCapturingArea}
                    />
                  )
                : undefined
            }
          />
        </section>
        {filesOpen && (
          <div className="thread-workspace">
            <WorkFiles
              key={fileAttempt?.id}
              project={project.id}
              item={item}
              attempt={fileAttempt}
              sources={fileAttempt?.id === latest?.id ? outputSources : []}
              canReview={canReview}
              canRequest={
                controlsAgent &&
                canContinue &&
                !mutation.isPending &&
                !attemptQuery.isError
              }
              onAsk={selectElement}
            />
          </div>
        )}
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
          {(areas.length > 0 ||
            (controlsAgent &&
              !!latest &&
              (workspaceOpen || shots.length > 0))) && (
            <div className="workspace-feedback-draft">
              {areas.length > 0 && (
                <AreaComments
                  areas={areas}
                  disabled={running || capturingArea || mutation.isPending}
                  onChange={updateAreas}
                />
              )}
              {controlsAgent && (
                <details>
                  <summary>
                    Attach screenshots{shots.length ? ` (${shots.length})` : ""}
                  </summary>
                  <ReviewScreenshots
                    shots={shots}
                    limit={screenshotLimit}
                    disabled={running || mutation.isPending || readingImage}
                    onChange={updateShots}
                    onAttach={(file) => void attachImage(file)}
                    onEdit={(index, trigger) =>
                      setScreenshotEditor({ index, trigger })
                    }
                  />
                </details>
              )}
              {imageError && (
                <p role="alert" className="text-destructive">
                  {imageError}
                </p>
              )}
              {shots.length > screenshotLimit && (
                <p role="alert">
                  Remove a screenshot to make room for the annotated preview.
                </p>
              )}
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
            <div className="conversation-compose-input">
              <Textarea
                ref={composer}
                id="work-followup"
                aria-invalid={messageTooLong || undefined}
                aria-describedby={
                  messageTooLong ? "work-followup-error" : undefined
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
              {canContinue && alternateWorkers.length > 0 && (
                <div className="flex flex-wrap items-center gap-2 px-2 pb-2">
                  <NativeSelect
                    aria-label="Worker for next message"
                    value={destination || ""}
                    disabled={running || mutation.isPending}
                    onChange={(event) => {
                      setMove(
                        event.target.value
                          ? { work: id, worker: event.target.value }
                          : undefined,
                      );
                      setSelection(undefined);
                    }}
                  >
                    <option value="">{worker?.name || "Current worker"}</option>
                    {alternateWorkers.map((candidate) => (
                      <option value={candidate.id} key={candidate.id}>
                        {candidate.name || candidate.id}
                      </option>
                    ))}
                  </NativeSelect>
                  {destination && (
                    <span className="text-xs text-muted-foreground">
                      The next worker gets the recorded conversation. Files and
                      running services stay on their worker.
                    </span>
                  )}
                </div>
              )}
              <div className="thread-composer-footer">
                <AgentSelection
                  compact
                  workspace={destination || item.workspace_id}
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
            {messageTooLong && (
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
      {screenshotEditor && shots[screenshotEditor.index] && (
        <ReviewScreenshotEditor
          shot={shots[screenshotEditor.index]}
          disabled={mutation.isPending}
          returnFocus={screenshotEditor.trigger}
          onClose={() => setScreenshotEditor(undefined)}
          onSave={(shot) => {
            updateShots(
              shots.map((previous, index) =>
                index === screenshotEditor.index ? shot : previous,
              ),
            );
            setScreenshotEditor(undefined);
          }}
        />
      )}
    </section>
  );
}

function DraftMessage({
  project,
  item,
  action,
  onLoad,
}: {
  project: string;
  item: WorkItem;
  action?: ReactNode;
  onLoad?: () => void;
}) {
  const query = useQuery(instructionOptions(project, item));
  if (query.isError)
    return <HistoryReadError error={query.error} onLoad={onLoad} />;
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

function HistoryReadError({
  error,
  onLoad,
}: {
  error: unknown;
  onLoad?: () => void;
}) {
  return (
    <>
      <ErrorNotice error={error} />
      {onLoad &&
        error instanceof APIError &&
        error.code === "HISTORY_WORKSPACE_STOPPED" && (
          <Button type="button" variant="outline" onClick={onLoad}>
            Start worker to load history
          </Button>
        )}
    </>
  );
}

function RunConversation({
  project,
  attempt,
  onOlder,
  onAsk,
  onActivity,
  action,
  onLoad,
  onReply,
}: {
  project: string;
  attempt: Attempt;
  action?: ReactNode;
  onLoad?: () => void;
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
        attempt,
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
      runHistory(project, attempt, cursor ?? history.data!.page.next_before),
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
      [...older, ...(history.data?.page.records || [])].map((r) => [r.id, r]),
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
        history.isSuccess &&
        !history.data?.cache,
      true,
    ),
  );
  const replyContent =
    records
      .filter((r) => r.kind === "assistant.message" && !r.redacted)
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
      {history.isError && (
        <HistoryReadError error={history.error} onLoad={onLoad} />
      )}
      {history.data?.cache && (
        <p className="conversation-muted">
          {records.length ? "Cached history" : "No cached records for this run"}{" "}
          as of {date(history.data.cache.synced_at)}.
          {history.data.cache.cached_through <
            history.data.cache.source_high_watermark &&
            " Some recorded activity has yet to sync."}
        </p>
      )}
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
          const reviewImage =
            record.kind === "user.message"
              ? submittedReviewImage(record.content || "")
              : undefined;
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
                    <>
                      {reviewMessageText(
                        userMessage(record.content || "", attempt),
                      )}
                      {reviewImage && (
                        <WorkspaceImage
                          src={reviewImage}
                          alt="Sent annotated preview"
                          project={project}
                          attempt={attempt}
                          availableWhileRunning
                        />
                      )}
                    </>
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
