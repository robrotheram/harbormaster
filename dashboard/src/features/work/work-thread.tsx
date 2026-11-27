import type { VisualizationRevision } from "@/features/analysis/api";
import {
  reportReference,
  reportReferences,
} from "@/features/analysis/report-model";
import {
  AnalysisVisualizations,
  type AnalysisReference,
} from "@/features/analysis/analysis-visualizations";
import { catalogKey } from "@/features/analysis/use-worker-visualizations";
import { useWorkerDatasets } from "@/features/analysis/use-worker-datasets";
import { AnalysisExplorer } from "@/features/analysis/analysis-explorer";
import { WorkerDatasets } from "@/features/analysis/worker-datasets";
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
import { useCompletedReports } from "./completed-report";
import { replyOutputs } from "./reply-outputs";
import { instructionOptions } from "./conversation-title";
import { composerKeyDown } from "./composer-keydown";
import {
  lazy,
  Suspense,
  useCallback,
  useEffect,
  useMemo,
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
  Database,
  FileText,
  Loader2,
  Square,
  Terminal,
  PanelRightOpen,
  MessageSquare,
  X,
  Paperclip,
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
  canContinueWork,
  workActions,
  workApi,
  workKeys,
  workOptions,
  type Attempt,
  type WorkItem,
} from "./api";
import {
  activeAttempt,
  historyAfter,
  mergeRunHistory,
  runHistory,
  refreshRunHistory,
  userMessage,
  watchRunHistory,
} from "./conversation-api";
import { WorkFiles } from "./work-files";
import { WorkOutput, retainedVisuals } from "./work-output";
import { workStage, workStatus } from "./stages";
import {
  reviewImageNote,
  reviewMessageText,
  submittedReviewImage,
} from "./review-image";
import { WorkPreview, type PreviewView } from "./work-preview";
import { MessageContent, WorkspaceImage } from "./message-content";

const activeRunPollInterval = 1000;
const WorkReport = lazy(() =>
  import("./work-report").then(({ WorkReport }) => ({
    default: WorkReport,
  })),
);

export function WorkConversation({
  project,
  id,
  title,
  navigation,
  message,
  setMessage,
  owner,
}: {
  project: Project;
  id: string;
  title?: string;
  owner?: string;
  navigation?: ReactNode;
  message: string;
  setMessage: Dispatch<SetStateAction<string>>;
}) {
  const client = useQueryClient();
  const attachHeader = useCallback((header: HTMLElement | null) => {
    const workbench = header?.closest<HTMLElement>(".conversation-workbench");
    if (!header || !workbench) return;
    const alignRail = () =>
      workbench.style.setProperty(
        "--thread-header-height",
        `${header.getBoundingClientRect().height}px`,
      );
    alignRail();
    const observer = new ResizeObserver(alignRail);
    observer.observe(header);
    return () => {
      observer.disconnect();
      workbench.style.removeProperty("--thread-header-height");
    };
  }, []);
  const key = useRequestKey();
  const [earlier, setEarlier] = useState(false);
  const [previewView, setPreviewView] = useState<PreviewView>();
  const personal = usePersonalWork();
  const revision = personal.editRevisions[id];
  const { focusComposer } = personal;
  const route = useSearch({ strict: false });
  const lastWorkspaceTool = useRef<
    Pick<typeof route, "view" | "artifact" | "dataset"> | undefined
  >(undefined);
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
  const [reply, setReply] = useState({
    attempt: "",
    content: "",
    loading: true,
  });
  const recordReply = useCallback(
    (attempt: string, content: string, loading: boolean) => {
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
        old.attempt === attempt &&
        old.content === content &&
        old.loading === loading
          ? old
          : { attempt, content, loading },
      );
    },
    [],
  );
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
    if (workspaceOpen)
      lastWorkspaceTool.current = route.view
        ? { view: route.view, artifact: route.artifact, dataset: route.dataset }
        : undefined;
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
  const reopenWorkspace = () => {
    if (workspaceOpen) return;
    if (lastWorkspaceTool.current) {
      void navigate({
        to: "/projects/$projectId/$section",
        params: { projectId: project.id, section: "work" },
        search: {
          work: id,
          attempt: route.attempt,
          ...lastWorkspaceTool.current,
        },
        replace: true,
      });
    } else setLayout("workspace");
  };
  const showWorkspace = (
    view: "workspace" | "datasets" | "files" | "report",
    latestRevision = false,
  ) =>
    navigate({
      to: "/projects/$projectId/$section",
      params: { projectId: project.id, section: "work" },
      search: {
        work: id,
        attempt: latestRevision ? undefined : route.attempt,
        view,
      },
    });
  const itemQuery = useQuery({
    queryKey: workKeys.detail(project.id, id),
    queryFn: ({ signal }) => workApi.detail(project.id, id, signal),
    refetchInterval: ({ state }) =>
      state.data && activeAttempt(state.data.state)
        ? activeRunPollInterval
        : 3000,
  });
  const attemptQuery = useQuery({
    queryKey: workKeys.attempts(project.id, id),
    queryFn: ({ signal }) => workApi.attempts(project.id, id, signal),
    enabled: !!itemQuery.data && !itemQuery.isError,
    refetchInterval:
      itemQuery.data && activeAttempt(itemQuery.data.state)
        ? activeRunPollInterval
        : 3000,
  });
  const workers = useQuery(workspaceOptions(project.id));
  const item = itemQuery.isError ? undefined : itemQuery.data;
  const dataScience = item?.kind === "data-science";
  const desktop = item?.kind === "desktop";
  const datasetsOpen =
    dataScience && route.view === "datasets" && route.work === id;
  const reportOpen =
    dataScience && route.view === "report" && route.work === id;
  const analysisOpen =
    dataScience && workspaceOpen && !filesOpen && !datasetsOpen && !reportOpen;
  const projectWork = useQuery({
    ...workOptions(project.id),
    enabled: !!analysisOpen,
    refetchInterval: false,
  });
  useEffect(() => {
    if (
      item?.id === id &&
      !datasetsOpen &&
      !reportOpen &&
      focusComposer.current === id
    ) {
      focusComposer.current = "";
      composer.current?.focus();
    }
  }, [datasetsOpen, focusComposer, id, item?.id, reportOpen]);
  const attempts = attemptQuery.isError
    ? []
    : [...(attemptQuery.data || [])].sort((a, b) =>
        a.created_at.localeCompare(b.created_at),
      );
  const latest = attempts.find((a) => a.id === item?.latest_attempt_id);
  const fileAttempt = route.attempt
    ? attempts.find((a) => a.id === route.attempt)
    : latest;
  const displayedAttempt = workspaceOpen ? fileAttempt : latest;
  const output = reply.attempt === fileAttempt?.id ? reply.content : "";
  const outputSources = replyOutputs(output);
  const visualFiles = fileAttempt
    ? retainedVisuals(fileAttempt, outputSources)
    : [];
  const { library: analysisReports } = useCompletedReports({
    project: project.id,
    work: item,
    attempts,
    pinnedAttempt: route.attempt,
    reply: output,
    replyAttempt: fileAttempt?.id,
    enabled: !!analysisOpen && !!fileAttempt && !attemptQuery.isError,
  });
  const references = new Map<string, AnalysisReference>();
  if (fileAttempt) {
    for (const { artifact } of outputSources) {
      if (!artifact) continue;
      const ref = {
        worker_id: fileAttempt.workspace_id,
        artifact_id: artifact,
      };
      references.set(reportReference(ref), ref);
    }
  }
  if (!analysisReports.isPlaceholderData) {
    for (const { markdown, worker, path } of analysisReports.data ?? []) {
      for (const ref of reportReferences(markdown, worker)) {
        const key = reportReference(ref);
        const previous = references.get(key)?.reportPaths ?? [];
        references.set(key, {
          ...ref,
          usedInReport: true,
          reportPaths:
            ref.worker_id === worker ? [...previous, path] : previous,
        });
      }
    }
  }
  const analysisWorker = fileAttempt?.workspace_id || item?.workspace_id;
  const finishedAttempt =
    latest && !activeAttempt(latest.state) ? latest.id : undefined;
  useEffect(() => {
    if (dataScience && analysisWorker && finishedAttempt)
      void client.invalidateQueries({
        queryKey: catalogKey(project.id, analysisWorker),
      });
  }, [client, dataScience, project.id, analysisWorker, finishedAttempt]);
  const otherOutput = outputSources.filter(({ artifact }) => !artifact);
  const hasOutput = outputSources.length > 0 || visualFiles.length > 0;
  const actions = item && workActions(item.state);
  const running =
    !!item && (actions?.cancel || (!!latest && activeAttempt(latest.state)));
  const worker = workers.data?.find((w) => w.id === item?.workspace_id);
  const [datasetSelection, setDatasetSelection] = useState<{
    worker: string;
    id: string;
  }>();
  const analysisDatasets = useWorkerDatasets(
    project.id,
    item?.workspace_id ?? "",
    !!analysisOpen && !route.attempt && !!worker?.ready,
  );
  const datasetChoices = analysisDatasets.data?.items ?? [];
  const analysisDataset =
    datasetSelection && datasetSelection.worker === item?.workspace_id
      ? datasetChoices.find(
          (dataset) => dataset.dataset_id === datasetSelection.id,
        )
      : datasetChoices.length === 1
        ? datasetChoices[0]
        : undefined;
  const [subscriptionSettings, setSubscriptionSettings] = useState(false);
  const [selection, setSelection] = useState<AgentModel>();
  const [move, setMove] = useState<{ work: string; worker: string }>();
  const destination = !revision && move?.work === id ? move.worker : undefined;
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
  const attachmentInput = useRef<HTMLInputElement>(null);
  const [readingImage, setReadingImage] = useState(false);
  const [imageError, setImageError] = useState("");
  const [screenshotEditor, setScreenshotEditor] = useState<{
    index: number;
    trigger: HTMLElement;
  }>();
  const compiledImage = useRef<{ key: string; content: string }>(null);
  const revisionInstruction = revision
    ? [
        `Revise visualization ${revision.artifact_id}. Read its specification with workbench-analysis artifact ${revision.artifact_id}. Apply the requested changes, register a new artifact ID on this worker, and embed the revised artifact in your reply.`,
        revision.report_paths?.length &&
          `Update the existing Markdown reports at ${JSON.stringify(revision.report_paths)} in place. Replace only the embedded references to ${revision.worker_id}/${revision.artifact_id}, including references that omit the worker ID, with references to the new artifact. Keep unrelated visualizations and report text. Link the updated report files in your reply.`,
      ]
        .filter(Boolean)
        .join(" ")
    : "";
  const datasetContext =
    analysisOpen &&
    !route.attempt &&
    !revision &&
    !destination &&
    message.trim() &&
    analysisDataset &&
    !message.includes(analysisDataset.dataset_id)
      ? `Selected dataset for this request: ${analysisDataset.dataset_id}.`
      : "";
  const instructionText = [
    revisionInstruction,
    datasetContext,
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
  function clearEditRevision() {
    personal.setEditRevisions((old) => {
      if (!old[id]) return old;
      const next = { ...old };
      delete next[id];
      return next;
    });
  }
  function draftAnalysis(prompt?: string) {
    clearEditRevision();
    setMove(undefined);
    if (prompt) {
      setMessage((current) =>
        current.trim() ? `${current}\n\n${prompt}` : prompt,
      );
    }
    composer.current?.focus();
  }
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
        revision: revision?.artifact_id,
        reportPaths: revision?.report_paths,
        datasetContext,
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
        item!.latest_attempt_id ?? "",
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
        clearEditRevision();
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
  const emptyThread =
    item.state === "draft" &&
    !item.latest_attempt_id &&
    !item.specification_path &&
    !!item.workspace_id;
  const canContinue = (emptyThread && controlsAgent) || canContinueWork(item);
  const showWorkerPicker =
    canContinue && !emptyThread && !revision && alternateWorkers.length > 0;
  const analysisLoading =
    (!!analysisOpen &&
      analysisReports.isPending &&
      analysisReports.isFetching) ||
    (attemptQuery.isPending && !!(route.attempt || item.latest_attempt_id)) ||
    (!!fileAttempt?.task_id &&
      (reply.attempt !== fileAttempt.id || reply.loading));
  let analysisTitle = "No visualizations yet";
  let analysisMessage =
    canContinue && controlsAgent
      ? "Ask your agent for a chart, map, or image in the message below."
      : "Check Files or Report for other results from this run.";
  if (route.attempt) {
    analysisTitle = fileAttempt
      ? "No visualizations in this revision"
      : "Revision unavailable";
    analysisMessage = fileAttempt
      ? "Check Files or Report for other results from this revision."
      : "Open the conversation to see the current run.";
  }
  if (analysisLoading) {
    analysisTitle = "Loading analysis results…";
    analysisMessage = "Checking the run for charts, maps, and images.";
  }
  const analysisRunning =
    running && (!route.attempt || route.attempt === latest?.id);
  if (analysisRunning) {
    analysisTitle =
      latest?.state === "cancel_requested"
        ? "Stopping the run…"
        : latest?.task_id
          ? "Agent is working"
          : "Preparing the worker";
    analysisMessage =
      "Any charts, maps, or images the agent shares will appear here as they become available.";
  }
  const revisionOnWorker =
    !revision || item.workspace_id === revision.worker_id;
  function startEditRevision(edit: VisualizationRevision) {
    const sourceWorker = workers.data?.find(
      (candidate) =>
        candidate.id === edit.worker_id &&
        candidate.project_id === project.id &&
        !["STOPPING", "UNKNOWN"].includes(candidate.state),
    );
    if (!sourceWorker) return false;
    const target =
      canContinue &&
      controlsAgent &&
      !mutation.isPending &&
      !attemptQuery.isError &&
      item?.workspace_id === sourceWorker.id
        ? item
        : !edit.report_paths?.length &&
          projectWork.data?.find(
            (candidate) =>
              candidate.id !== id &&
              candidate.workspace_id === sourceWorker.id &&
              candidate.kind === "data-science" &&
              canContinueWork(candidate) &&
              (local || (!!user && candidate.controller_id === user)),
          );
    if (!target) return false;
    personal.setEditRevisions((old) => ({
      ...old,
      [target.id]: edit,
    }));
    focusComposer.current = target.id;
    if (target.id === id) {
      setMove(undefined);
      focusComposer.current = "";
      composer.current?.focus();
      if (route.attempt) void showWorkspace("workspace", true);
    } else {
      void navigate({
        to: "/projects/$projectId/$section",
        params: { projectId: project.id, section: "work" },
        search: { work: target.id, view: "workspace" },
      });
    }
    return true;
  }
  const canSend =
    !running &&
    !mutation.isPending &&
    canContinue &&
    revisionOnWorker &&
    (!revision || (controlsAgent && !!message.trim())) &&
    (!destination || alternateWorkers.some((w) => w.id === destination)) &&
    !capturingArea &&
    !readingImage &&
    !areas.some(({ comment }) => !comment.trim()) &&
    shots.length <= screenshotLimit &&
    !!instructionText.trim() &&
    !messageTooLong;
  const retry = latest && !["planned", "in_progress"].includes(item.state);
  const runAction =
    actions?.run && !emptyThread ? (
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
      </div>
    ) : null;
  const viewSwitch = (
    <div className="thread-view-switch" role="group" aria-label="Thread view">
      <Button
        type="button"
        variant="ghost"
        size="sm"
        aria-pressed={!workspaceOpen}
        onClick={() => showConversation(unreadReply)}
      >
        <MessageSquare size={15} />{" "}
        <span className="thread-view-label">Conversation</span>
        <span
          className="thread-unread-dot"
          data-unread={unreadReply}
          aria-hidden="true"
        />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        aria-pressed={workspaceOpen}
        onClick={reopenWorkspace}
      >
        <PanelRightOpen size={15} />{" "}
        <span className="thread-view-label">Workspace</span>
      </Button>
      <span className="sr-only" role="status">
        {unreadReply ? "New agent message" : ""}
      </span>
    </div>
  );
  return (
    <section
      className="conversation-session visual-session unified-thread"
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
      <div className="conversation-main">
        <header ref={attachHeader} className="conversation-thread-heading">
          <div className="conversation-heading-title">
            {navigation}
            <div className="thread-heading-summary min-w-0">
              <h2 title={title} className="line-clamp-2">
                {title || `${label(item.kind)} conversation`}
              </h2>
              {item.state !== "closed" &&
                workStage(item.state) !== "review" && (
                  <div className="thread-heading-state">
                    <State value={workStatus(item.state)} />
                  </div>
                )}
            </div>
          </div>
          <div className="thread-header-controls">
            {viewSwitch}
            <Responsibility
              item={item}
              running={running}
              reviewActions={
                workspaceOpen && canReview ? (
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
                          setChangeRequest({
                            attempt: latest.id,
                            feedback: "",
                          });
                        }}
                      >
                        Request changes
                      </Button>
                    )}

                    {mutation.isError && mutation.variables === "accept" && (
                      <ErrorNotice error={mutation.error} />
                    )}
                    {changeRequest?.attempt === latest?.id && changeRequest && (
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
                          {mutation.error && (
                            <ErrorNotice error={mutation.error} />
                          )}
                          <Button
                            type="submit"
                            disabled={
                              mutation.isPending ||
                              !changeRequest.feedback.trim()
                            }
                          >
                            Send change request
                          </Button>
                        </form>
                      </Modal>
                    )}
                  </>
                ) : undefined
              }
            >
              {item.workspace_id ? (
                <Link
                  className="conversation-worker-link"
                  to="/projects/$projectId/workspaces/$workerId"
                  params={{
                    projectId: project.id,
                    workerId: item.workspace_id,
                  }}
                  title={item.workspace_id}
                >
                  <Server size={14} /> Worker: {worker?.name || "Task worker"}
                </Link>
              ) : (
                <Link
                  className="conversation-worker-link"
                  to="/projects/$projectId/$section"
                  params={{ projectId: project.id, section: "workspaces" }}
                >
                  <Server size={14} /> Project workers
                </Link>
              )}
              {showWorkerPicker && (
                <div className="space-y-2">
                  <label
                    htmlFor="work-followup-worker"
                    className="text-xs font-medium"
                  >
                    Worker for next message
                  </label>
                  <NativeSelect
                    id="work-followup-worker"
                    aria-describedby={
                      destination ? "work-followup-worker-hint" : undefined
                    }
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
                    <p id="work-followup-worker-hint">
                      The next worker gets the recorded conversation. Files and
                      running services stay on their worker.
                    </p>
                  )}
                </div>
              )}
              <p>Created {date(item.created_at)}</p>
              {owner && <p>Task assignee: {owner}</p>}
            </Responsibility>
          </div>
        </header>
        {workspaceOpen && (
          <div
            className="thread-workspace-tools"
            role="group"
            aria-label="Workspace tools"
          >
            <Button
              variant="ghost"
              size="sm"
              aria-pressed={!filesOpen && !datasetsOpen && !reportOpen}
              onClick={() => {
                if (filesOpen || datasetsOpen || reportOpen) {
                  void showWorkspace("workspace");
                } else if (!dataScience && !desktop) setPreviewView("web");
              }}
            >
              <PanelRightOpen size={15} />{" "}
              {dataScience ? "Analysis" : "Preview"}
            </Button>
            {dataScience && (
              <>
                <Button
                  variant="ghost"
                  size="sm"
                  aria-pressed={datasetsOpen}
                  onClick={() => void showWorkspace("datasets")}
                >
                  <Database size={15} /> Datasets
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  aria-pressed={reportOpen}
                  onClick={() => void showWorkspace("report")}
                >
                  <FileText size={15} /> Report
                </Button>
              </>
            )}
            <Button
              variant="ghost"
              size="sm"
              aria-pressed={filesOpen}
              onClick={() => void showWorkspace("files")}
            >
              <FileText size={15} /> Files
            </Button>
            {filesOpen && attempts.length > 1 && latest && (
              <NativeSelect
                aria-label="File changes by run"
                size="sm"
                value={route.attempt ?? ""}
                onChange={(event) =>
                  void navigate({
                    to: "/projects/$projectId/$section",
                    params: { projectId: project.id, section: "work" },
                    search: {
                      work: id,
                      attempt: event.target.value || undefined,
                      view: "files",
                    },
                  })
                }
              >
                <option value="">Latest changes</option>
                {attempts
                  .map((run, index) => (
                    <option key={run.id} value={run.id}>
                      Run {index + 1}
                      {run.id === latest.id ? " (latest)" : ""}
                    </option>
                  ))
                  .reverse()}
              </NativeSelect>
            )}
          </div>
        )}
        {((workspaceOpen &&
          fileAttempt?.review_state === "changes_requested") ||
          displayedAttempt?.review_feedback) && (
          <div className="task-guidance whitespace-pre-wrap">
            <strong>Requested changes</strong>
            <div>
              {displayedAttempt?.review_feedback ||
                "The controller can prepare the next revision."}
            </div>
          </div>
        )}
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
            {!emptyThread &&
              !attempts.length &&
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
                editVisualizations={dataScience}
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
          hidden={!workspaceOpen || filesOpen || datasetsOpen || reportOpen}
        >
          {dataScience ? (
            analysisOpen && (
              <>
                {!route.attempt && (
                  <AnalysisExplorer
                    key={item.workspace_id}
                    project={project.id}
                    worker={item.workspace_id}
                    datasets={analysisDatasets}
                    dataset={analysisDataset}
                    onSelect={(id) =>
                      setDatasetSelection({ worker: item.workspace_id, id })
                    }
                    onBrowse={() =>
                      void navigate({
                        to: "/projects/$projectId/$section",
                        params: { projectId: project.id, section: "work" },
                        search: { work: id, view: "datasets" },
                      })
                    }
                    onDraft={
                      controlsAgent && canContinue && !mutation.isPending
                        ? draftAnalysis
                        : undefined
                    }
                  />
                )}
                <AnalysisVisualizations
                  key={`${analysisWorker ?? ""}/${route.attempt ?? ""}/${route.artifact ?? ""}`}
                  project={project.id}
                  worker={analysisWorker ?? ""}
                  references={[...references.values()]}
                  initialArtifact={route.artifact}
                  pinned={!!route.attempt}
                  status={
                    analysisLoading ||
                    analysisRunning ||
                    (route.attempt && !fileAttempt)
                      ? analysisTitle
                      : undefined
                  }
                  emptyMessage={analysisMessage}
                  canDelete={controlsAgent && !route.attempt}
                  onRevise={startEditRevision}
                  onAsk={
                    controlsAgent &&
                    canContinue &&
                    !mutation.isPending &&
                    !route.attempt &&
                    analysisWorker === item.workspace_id
                      ? ({ artifact_id }) =>
                          draftAnalysis(
                            `Explain visualization ${artifact_id}. Read its specification with workbench-analysis artifact ${artifact_id}. Summarize what the result shows, how the data was aggregated, and what caveats matter before drawing a conclusion.`,
                          )
                      : undefined
                  }
                />
                {attemptQuery.isError && (
                  <div className="px-4 pb-4 sm:px-6">
                    <ErrorNotice
                      error={attemptQuery.error}
                      message="Workbench could not load the run history for this analysis."
                      onRetry={() => void attemptQuery.refetch()}
                      retrying={attemptQuery.isFetching}
                    />
                  </div>
                )}
                {fileAttempt &&
                  (otherOutput.length > 0 || visualFiles.length > 0) && (
                    <div className="analysis-results min-w-0 px-4 pb-5 sm:px-6">
                      <WorkOutput
                        key={fileAttempt.id}
                        project={project.id}
                        attempt={fileAttempt}
                        sources={otherOutput}
                        files={visualFiles}
                        onAsk={selectElement}
                      />
                    </div>
                  )}
              </>
            )
          ) : (
            <WorkPreview
              project={project.id}
              workspace={item.workspace_id}
              revision={
                latest && !activeAttempt(latest.state) ? latest.id : undefined
              }
              mode={desktop ? "desktop" : "web"}
              view={previewView}
              onViewChange={setPreviewView}
              onSelection={selectElement}
              output={
                !desktop && workspaceOpen && hasOutput && fileAttempt ? (
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
          )}
        </section>
        {datasetsOpen && dataScience && (
          <section className="thread-workspace" aria-label="Datasets page">
            <div className="min-w-0 space-y-4 px-4 py-5 sm:px-6">
              <h2 className="text-lg font-semibold">Datasets</h2>
              {worker ? (
                <WorkerDatasets
                  key={`${worker.id}/${route.dataset ?? ""}`}
                  worker={worker}
                  project={project.id}
                  initialDataset={route.dataset}
                  onExplore={
                    controlsAgent && canContinue && !route.attempt
                      ? (dataset) => {
                          setDatasetSelection({
                            worker: worker.id,
                            id: dataset,
                          });
                          clearEditRevision();
                          setMove(undefined);
                          focusComposer.current = id;
                          void showWorkspace("workspace");
                        }
                      : undefined
                  }
                />
              ) : (
                <p role="status">The task worker is unavailable.</p>
              )}
            </div>
          </section>
        )}
        {reportOpen && (
          <section className="thread-workspace thread-report" aria-label="Report">
            <Suspense
              fallback={
                <p role="status" className="p-4 text-sm">
                  Loading reports…
                </p>
              }
            >
              <WorkReport
                project={project.id}
                work={item}
                attempts={attempts}
                pinnedAttempt={route.attempt}
                loadingAttempts={
                  attemptQuery.isPending && !!item.latest_attempt_id
                }
                reply={output}
                replyAttempt={fileAttempt?.id}
                key={`${id}:${item.workspace_id}:${route.attempt ?? ""}`}
                canEdit={controlsAgent && !running}
              />
            </Suspense>
          </section>
        )}
        {filesOpen && (
          <div className="thread-workspace">
            <WorkFiles
              key={project.id + ":" + item.id + ":" + item.workspace_id}
              project={project.id}
              item={item}
              attempt={fileAttempt}
              attempts={attempts}
              pinnedAttempt={route.attempt}
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
        {workspaceOpen && runAction && (
          <div className="thread-workspace-recovery">{runAction}</div>
        )}
        <footer className="thread-footer" hidden={datasetsOpen || reportOpen}>
          <div className="conversation-composer">
            {mutation.isError &&
              mutation.variables !== "run" &&
              mutation.variables !== "accept" &&
              mutation.variables !== "request-changes" && (
                <>
                  <ErrorNotice error={mutation.error} />
                  {subscriptionAction(mutation.error)}
                </>
              )}
            {areas.length > 0 && (
              <div className="workspace-feedback-draft">
                <AreaComments
                  areas={areas}
                  disabled={running || capturingArea || mutation.isPending}
                  onChange={updateAreas}
                />
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
                {controlsAgent && shots.length > 0 && (
                  <ReviewScreenshots
                    shots={shots}
                    disabled={running || mutation.isPending || readingImage}
                    onRemove={(index) =>
                      updateShots(shots.filter((_, i) => i !== index))
                    }
                    onEdit={(index, trigger) =>
                      setScreenshotEditor({ index, trigger })
                    }
                  />
                )}
                {(imageError || shots.length > screenshotLimit) && (
                  <p
                    role="alert"
                    className="px-3 pt-2 text-xs text-destructive"
                  >
                    {imageError ||
                      "Remove a screenshot to make room for the annotated preview."}
                  </p>
                )}
                {analysisOpen &&
                  !route.attempt &&
                  !revision &&
                  !destination &&
                  analysisDataset && (
                    <p className="truncate px-3 pt-2 text-xs text-muted-foreground">
                      Dataset: {analysisDataset.name}
                    </p>
                  )}
                {revision && (
                  <div className="px-3 pt-2">
                    <div className="flex w-fit max-w-full items-center gap-1 rounded-md border px-2 text-xs">
                      <span className="min-w-0 truncate">
                        Editing {revision.title}
                      </span>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-xs"
                        aria-label="Clear visualization edit"
                        onClick={clearEditRevision}
                        disabled={mutation.isPending}
                      >
                        <X />
                      </Button>
                    </div>
                    {!revisionOnWorker && (
                      <p
                        role="status"
                        className="pt-1 text-xs text-muted-foreground"
                      >
                        Open a task on the original worker to request this edit.
                      </p>
                    )}
                  </div>
                )}
                <div className="thread-message-field">
                  {controlsAgent && latest && (
                    <>
                      <input
                        ref={attachmentInput}
                        hidden
                        type="file"
                        accept="image/png,image/jpeg"
                        onChange={(event) => {
                          const file = event.currentTarget.files?.[0];
                          event.currentTarget.value = "";
                          if (file) void attachImage(file);
                        }}
                      />
                      <Button
                        type="button"
                        className="thread-attach"
                        size="icon"
                        variant="ghost"
                        aria-label="Attach screenshot"
                        title={
                          shots.length >= screenshotLimit
                            ? "Screenshot limit reached"
                            : "Attach screenshot"
                        }
                        disabled={
                          running ||
                          mutation.isPending ||
                          readingImage ||
                          shots.length >= screenshotLimit
                        }
                        onClick={() => attachmentInput.current?.click()}
                      >
                        <Paperclip />
                      </Button>
                    </>
                  )}
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
                        : revision
                          ? "Describe what to change…"
                          : dataScience
                            ? "Ask a question about your data or describe a chart…"
                            : "Tell the agent what to do next…"
                    }
                    disabled={
                      (!emptyThread && !item.latest_attempt_id) ||
                      mutation.isPending
                    }
                  />
                </div>
                <div className="thread-composer-footer">
                  <div className="thread-message-controls">
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
                        setSelection({
                          provider,
                          model,
                          reasoning_effort: effort,
                        })
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
                          size="icon"
                          aria-label="Send"
                          title="Send"
                        >
                          {mutation.isPending ? (
                            <Loader2 size={16} className="loading-spinner" />
                          ) : (
                            <ArrowUp size={16} />
                          )}
                        </Button>
                      )}
                    </div>
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
        </footer>
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
  editVisualizations,
}: {
  project: string;
  attempt: Attempt;
  action?: ReactNode;
  onLoad?: () => void;
  onReply?: (attempt: string, content: string, loading: boolean) => void;
  onOlder: () => void;
  onAsk: (message: string) => void;
  onActivity: () => void;
  editVisualizations: boolean;
}) {
  const [older, setOlder] = useState<HistoryRecord[]>([]);
  const [cursor, setCursor] = useState<number>();
  const [live, setLive] = useState(false);
  const observed = useRef<Snapshot | undefined>(undefined);
  const { id, work_item_id, workspace_id, task_id, state } = attempt;
  const run = useMemo(
    () => ({ id, work_item_id, workspace_id, task_id }),
    [id, work_item_id, workspace_id, task_id],
  );
  const client = useQueryClient();
  const historyKey = useMemo(
    () => ["project", project, "work-conversation", workspace_id, id, state],
    [project, workspace_id, id, state],
  );
  const active = activeAttempt(state);
  const history = useQuery({
    ...privateContentOptions,
    queryKey: historyKey,
    queryFn: async ({ signal }) => {
      const previous = observed.current;
      const value = await refreshRunHistory(project, run, previous, signal);
      if (signal.aborted) return value;
      observed.current =
        previous === observed.current
          ? value
          : mergeRunHistory(observed.current, value);
      return observed.current;
    },
    enabled: !!task_id,
    refetchInterval: (query) =>
      !live && (active || query.state.data?.page.next_after)
        ? activeRunPollInterval
        : false,
  });
  const canStream =
    active && !!task_id && history.isSuccess && !history.data?.cache;
  useEffect(() => {
    setLive(false);
    if (!canStream || typeof EventSource === "undefined") return;
    return watchRunHistory(
      project,
      run,
      historyAfter(observed.current) ?? 0,
      setLive,
      (page) => {
        observed.current = mergeRunHistory(observed.current, page);
        client.setQueryData(historyKey, observed.current);
      },
    );
  }, [canStream, client, historyKey, project, run]);
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
  const combinedRecords = [
    ...new Map(
      [...older, ...(history.data?.page.records || [])].map((r) => [r.id, r]),
    ).values(),
  ].sort((a, b) => a.seq - b.seq);
  const replyTexts = new Set(
    combinedRecords
      .filter((record) => record.kind === "assistant.message")
      .map((record) => record.content?.trim()),
  );
  const records = combinedRecords.filter(
    (record) =>
      record.kind !== "assistant.progress" ||
      !replyTexts.has(record.content?.trim()),
  );
  const loadResult =
    !!attempt.task_id &&
    !active &&
    !records.some((r) => r.kind === "assistant.message") &&
    history.isSuccess &&
    !history.data?.cache;
  const result = useQuery(
    resultOptions(
      project,
      attempt.workspace_id,
      attempt.task_id,
      loadResult,
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
  const replyLoading =
    (!!task_id && history.isPending) || (loadResult && result.isPending);
  useEffect(() => {
    onReply?.(attempt.id, replyContent, replyLoading);
  }, [onReply, attempt.id, replyContent, replyLoading]);
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
            record.kind === "assistant.message" ||
            record.kind === "assistant.progress"
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
                      editVisualizations={editVisualizations}
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
              editVisualizations={editVisualizations}
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
