import { useThreadTitles } from "@/features/work/thread-titles";
import { memberOptions } from "@/features/work/responsibility";
import { capabilityOptions } from "./capabilities";
import { useState, useEffect, useRef } from "react";
import { Link, useSearch, useNavigate } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Archive,
  ArrowDownToLine,
  AlertTriangle,
  Search,
  Plus,
  Play,
  Loader2,
  GitBranch,
  RefreshCw,
  SlidersHorizontal,
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/native-select";
import {
  Sheet,
  SheetContent,
  SheetClose,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet";
import {
  AgentSelection,
  type AgentModel,
} from "@/features/work/agent-selection";
import { workspaceOptions } from "@/features/workspaces/api";
import { ErrorNotice, Modal, Form } from "@/shared/components";
import { request } from "@/shared/http";
import { label } from "@/shared/format";
import type { Project } from "@/shared/types";
import { workKeys } from "@/features/work/api";
import { boardApi, boardKey, boardOptions, type BoardCard } from "./board-api";
import {
  activityTone,
  teamActivity,
  needsAttention,
  pendingDependencies,
  taskIsForMember,
  inTaskScope,
  assigneeKey,
} from "./team-state";
import { taskReference, PriorityPill } from "./task-controls";
import { TaskDetails, type TaskChange, type TaskEntry } from "./task-details";
import { NewIssue } from "./new-issue";
import { IssueImport } from "./issue-import";

export function IssueBoard({ project }: { project: Project }) {
  return <Board key={project.id} project={project} />;
}
function Board({ project }: { project: Project }) {
  const client = useQueryClient(),
    navigate = useNavigate(),
    route = useSearch({ strict: false });
  const board = useQuery(boardOptions(project.id)),
    capabilities = useQuery(capabilityOptions(project.id));
  const rawCards = board.data?.items || [];
  const { titles } = useThreadTitles(
    project.id,
    rawCards.map((c) => c.work),
  );
  const cards = rawCards.map((c) => ({
    ...c,
    title: c.title || titles.get(c.id),
  }));
  const roster = useQuery(memberOptions(project.id));
  const members = roster.data?.items || [];
  const me = members.find((member) => member.user_id === roster.data?.user_id);
  const assigneeLabel = (card: BoardCard) =>
    card.owner &&
    me &&
    (card.owner_id === me.user_id ||
      assigneeKey(card) === me.email.toLowerCase())
      ? "You"
      : card.owner || "Unassigned";
  const memberLabel = (id: string) =>
    id === me?.user_id
      ? "You"
      : members.find((member) => member.user_id === id)?.email;
  const cardsById = new Map(cards.map((card) => [card.id, card]));
  const requiresReadyDependencies = (card: BoardCard) =>
    !!card.require_dependencies &&
    pendingDependencies(card, cardsById).length > 0;
  const canStartTask = (card: BoardCard) =>
    capabilities.data?.can_start === true &&
    !requiresReadyDependencies(card) &&
    (!card.work.controller_id ||
      capabilities.data?.can_import === true ||
      roster.data?.user_id === card.work.controller_id);
  const startReason = (card: BoardCard) =>
    requiresReadyDependencies(card)
      ? "Finish the required dependencies before starting this task."
      : capabilities.data?.can_start && !canStartTask(card)
        ? `Ask ${memberLabel(card.work.controller_id || "") || "the current controller"} to transfer agent control before you start this task.`
        : capabilities.data?.start_reason;
  const [search, setSearch] = useState(""),
    [owner, setOwner] = useState(""),
    [scope, setScope] = useState("open"),
    [flagged, setFlagged] = useState(false),
    [archived, setArchived] = useState(false),
    [creating, setCreating] = useState(false),
    [importing, setImporting] = useState(false),
    [taskEntry, setTaskEntry] = useState<TaskEntry>("details"),
    [selected, setSelected] = useState(route.work || "");
  const [sort, setSort] = useState("priority");
  const returnFocus = useRef<HTMLElement | null>(null);
  useEffect(() => {
    setSelected(route.work || "");
  }, [route.work]);
  const select = (id: string, entry: TaskEntry = "details") => {
    setTaskEntry(entry);
    setSelected(id);
    void navigate({
      to: "/projects/$projectId/$section",
      params: { projectId: project.id, section: "team" },
      search: id ? { work: id } : {},
      replace: true,
    });
  };
  const refresh = () =>
    Promise.all([
      client.invalidateQueries({ queryKey: boardKey(project.id) }),
      client.invalidateQueries({ queryKey: workKeys.list(project.id) }),
    ]);
  const change = useMutation({
    mutationFn: ({ card, edit }: { card: BoardCard; edit: TaskChange }) =>
      boardApi.save(
        { ...card, revision: edit.revision ?? card.revision },
        edit.stage || card.stage,
        edit.title,
        edit.description,
        {
          ...(edit.owner_id !== undefined ? { owner_id: edit.owner_id } : {}),
          ...(edit.priority ? { priority: edit.priority } : {}),
          ...(edit.require_dependencies !== undefined
            ? { require_dependencies: edit.require_dependencies }
            : {}),
          ...(edit.dependencies ? { dependencies: edit.dependencies } : {}),
        },
      ),
    onSettled: refresh,
  });
  const sync = useMutation({
    mutationFn: ({
      card,
      direction,
      discard,
    }: {
      card: BoardCard;
      direction: "pull" | "push";
      discard?: boolean;
    }) => boardApi.sync(card, direction, discard),
    onSettled: refresh,
  });
  const [startingCard, setStartingCard] = useState<BoardCard>();
  const start = useMutation({
    mutationFn: ({
      card,
      workspace,
      selection,
    }: {
      card: BoardCard;
      workspace?: string;
      selection?: AgentModel;
    }) =>
      request(`/board/${encodeURIComponent(card.id)}/start`, {
        project_id: project.id,
        revision: card.revision,
        workspace_id: workspace,
        selection,
      }),
    onSuccess: async (_, { card }) => {
      setStartingCard(undefined);
      await refresh();
      await navigate({
        to: "/projects/$projectId/$section",
        params: { projectId: project.id, section: "work" },
        search: { work: card.id },
      });
    },
    onError: refresh,
  });
  const startWork = (card: BoardCard) => {
    if (capabilities.data?.requires_agent_selection) {
      start.reset();
      setStartingCard(card);
    } else start.mutate({ card });
  };
  const current = cards.find((c) => c.id === selected);
  const scopes = ["open", "active", "planned", "completed"];
  if (me?.user_id) scopes.splice(1, 0, "for-me");
  const matchesScope = (card: BoardCard, value: string) =>
    value === "for-me"
      ? taskIsForMember(card, me?.user_id || "", me?.email || "")
      : inTaskScope(card, value);
  const priorityRank = (c: BoardCard) =>
    ({ high: 0, normal: 1, low: 2 })[c.priority || "normal"] ?? 1;
  const matches = cards.filter(
    (c) =>
      (archived ? c.stage === "archived" : c.stage !== "archived") &&
      (!owner || assigneeKey(c) === owner) &&
      (!flagged ||
        needsAttention(c, pendingDependencies(c, cardsById).length)) &&
      `${c.title || ""} ${taskReference(c)} ${c.id} ${c.owner || "unassigned"} ${c.source?.repository || ""} ${c.source?.number || ""}`
        .toLowerCase()
        .includes(search.toLowerCase()),
  );
  const visible = matches
    .filter((c) => archived || matchesScope(c, scope))
    .sort((a, b) =>
      sort === "priority"
        ? priorityRank(a) - priorityRank(b) || (a.number || 0) - (b.number || 0)
        : b.work.updated_at.localeCompare(a.work.updated_at),
    );
  const taskLink = (card: BoardCard) => (
    <Link
      className="task-reference-link"
      to="/projects/$projectId/$section"
      params={{ projectId: project.id, section: "team" }}
      search={{ work: card.id }}
    >
      {taskReference(card)}
    </Link>
  );
  const error = change.error || sync.error || start.error;
  const busy = change.isPending || sync.isPending || start.isPending;
  return (
    <div className="issue-board-page overview-page">
      <header className="board-heading">
        <div>
          <h1>Overview</h1>
          <p>Tasks, owners, and the next step.</p>
        </div>
        <div className="overview-header-actions">
          <Button
            variant="ghost"
            disabled={!cards.some((card) => card.stage !== "archived")}
            onClick={() => {
              const first =
                visible.find((card) => card.stage !== "archived") ||
                cards.find((card) => card.stage !== "archived");
              if (first) select(first.id, "project");
            }}
          >
            <GitBranch size={15} /> Activity
          </Button>
          <Button variant="ghost" onClick={() => setArchived(!archived)}>
            <Archive size={15} />
            {archived ? "Back to overview" : "Archive"}
          </Button>
          <Button variant="outline" onClick={() => setImporting(!importing)}>
            <ArrowDownToLine size={15} />
            Import
          </Button>
          <Button onClick={() => setCreating(true)}>
            <Plus size={16} />
            Create task
          </Button>
        </div>
      </header>
      <div className="overview-toolbar">
        <div className="board-search">
          <Search size={16} />
          <Input
            aria-label="Find work"
            placeholder="Search tasks, # or assignee…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
        <NativeSelect
          aria-label="Filter by owner"
          value={owner}
          onChange={(e) => setOwner(e.target.value)}
        >
          <option value="">All assignees</option>
          <option value="unassigned">Unassigned</option>
          {[
            ...new Map(
              cards
                .filter((c) => c.owner)
                .map((c) => [assigneeKey(c), c.owner!]),
            ).entries(),
          ].map(([id, name]) => (
            <option key={id} value={id}>
              {name}
            </option>
          ))}
        </NativeSelect>
        <Button
          variant={flagged ? "secondary" : "ghost"}
          aria-pressed={flagged}
          onClick={() => setFlagged(!flagged)}
        >
          <AlertTriangle size={14} />
          Needs attention
        </Button>
        <div className="overview-sort">
          <SlidersHorizontal size={14} />
          <NativeSelect
            aria-label="Sort tasks"
            value={sort}
            onChange={(e) => setSort(e.target.value)}
          >
            <option value="priority">Priority first</option>
            <option value="updated">Recently updated</option>
          </NativeSelect>
        </div>
      </div>
      {importing && (
        <IssueImport project={project.id} onClose={() => setImporting(false)} />
      )}
      {creating && (
        <NewIssue
          project={project.id}
          onClose={() => setCreating(false)}
          onCreated={(id) => {
            setArchived(false);
            setSearch("");
            setFlagged(false);
            select(id);
          }}
        />
      )}
      {error && !current && <ErrorNotice error={error} />}
      {capabilities.isError && (
        <div className="overview-action-notice" role="status">
          <AlertTriangle size={16} />
          <div>
            <strong>Task actions are temporarily unavailable</strong>
            <p>
              Your tasks are still available to view and edit. Retry to check
              whether you can start work.
            </p>
          </div>
          <Button
            variant="outline"
            size="sm"
            disabled={capabilities.isFetching}
            onClick={() => void capabilities.refetch()}
          >
            <RefreshCw size={14} />
            {capabilities.isFetching ? "Checking…" : "Retry access check"}
          </Button>
        </div>
      )}
      {capabilities.data && !capabilities.data.can_start && (
        <div className="overview-action-notice" role="status">
          <AlertTriangle size={16} />
          <div>
            <strong>Agent execution needs setup</strong>
            <p>
              {capabilities.data.start_reason} You can still plan and assign
              tasks.
            </p>
          </div>
        </div>
      )}
      {board.isError && (
        <ErrorNotice error={board.error} onRetry={() => void board.refetch()} />
      )}
      {board.isPending ? (
        <p role="status">Loading tasks…</p>
      ) : (
        <>
          <div className="overview-scope-bar">
            {archived ? (
              <h2>Archived tasks</h2>
            ) : (
              <nav className="team-scopes" aria-label="Work scope">
                {scopes.map((value) => (
                  <Button
                    key={value}
                    variant="ghost"
                    aria-pressed={scope === value}
                    onClick={() => setScope(value)}
                  >
                    {value === "for-me" ? "For me" : label(value)}
                    <span className="scope-count">
                      {
                        cards.filter(
                          (c) =>
                            c.stage !== "archived" && matchesScope(c, value),
                        ).length
                      }
                    </span>
                  </Button>
                ))}
              </nav>
            )}
            <span className="board-item-count">
              {visible.length} {visible.length === 1 ? "task" : "tasks"}
            </span>
          </div>
          <div className="team-table-scroll">
            <table className="team-table">
              <thead>
                <tr>
                  <th>Task ID</th>
                  <th>Task</th>
                  <th>Priority</th>
                  <th>Assignee</th>
                  <th>Activity</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {visible.map((card) => {
                  const awaitingReview = card.work.state === "awaiting_review";
                  const revising = card.work.state === "changes_requested";
                  const reviewer = card.work.reviewer_id;
                  const controller = card.work.controller_id;
                  const pending = pendingDependencies(card, cardsById).length;
                  const firstRun =
                    !card.work.latest_attempt_id &&
                    ["planned", "draft", "in_progress"].includes(
                      card.work.state,
                    ) &&
                    card.stage !== "archived";
                  return (
                    <tr
                      key={card.id}
                      className="task-clickable-row"
                      data-priority={card.priority || "normal"}
                      onClick={(event) => {
                        if (
                          (event.target as HTMLElement).closest(
                            "a, button, input, select",
                          ) ||
                          window.getSelection()?.toString()
                        )
                          return;
                        event.currentTarget
                          .querySelector<HTMLButtonElement>(".team-work-title")
                          ?.focus();
                        select(card.id);
                      }}
                    >
                      <td className="task-id-cell">{taskLink(card)}</td>
                      <td className="task-title-cell">
                        <div className="task-row-title">
                          <button
                            className="team-work-title"
                            aria-haspopup="dialog"
                            onClick={() => select(card.id)}
                          >
                            {card.title || "Untitled task"}
                          </button>
                        </div>
                        <small>
                          Updated{" "}
                          {new Date(card.work.updated_at).toLocaleDateString(
                            undefined,
                            { month: "short", day: "numeric" },
                          )}
                          {card.source
                            ? ` · ${card.source.repository} #${card.source.number}`
                            : ""}
                        </small>
                      </td>
                      <td data-label="Priority">
                        <PriorityPill priority={card.priority} />
                      </td>
                      <td data-label="Assignee">
                        <span
                          className={`task-assignee ${card.owner ? "" : "is-unassigned"}`}
                        >
                          <span className="task-avatar" aria-hidden="true">
                            {card.owner
                              ? card.owner.slice(0, 1).toUpperCase()
                              : "—"}
                          </span>
                          <span title={card.owner || undefined}>
                            {assigneeLabel(card)}
                          </span>
                        </span>
                      </td>
                      <td data-label="Activity">
                        <button
                          type="button"
                          className={`task-pill activity-trigger activity-${activityTone(card)}${pending > 0 ? " has-dependency-warning" : ""}`}
                          aria-label={`${teamActivity(card)}${pending > 0 ? ", dependency warning" : ""}. View activity for ${card.title || taskReference(card)}`}
                          aria-haspopup="dialog"
                          onClick={() => select(card.id, "activity")}
                        >
                          <i />
                          {teamActivity(card)}
                          {pending > 0 && (
                            <AlertTriangle size={13} aria-hidden="true" />
                          )}
                        </button>
                      </td>
                      <td className="task-row-actions">
                        <div className="task-action-group">
                          {firstRun ? (
                            <Button
                              size="sm"
                              variant="outline"
                              disabled={busy || !canStartTask(card)}
                              title={
                                canStartTask(card)
                                  ? undefined
                                  : startReason(card)
                              }
                              onClick={() => startWork(card)}
                            >
                              {start.isPending &&
                              start.variables?.card.id === card.id ? (
                                <Loader2
                                  size={14}
                                  className="loading-spinner"
                                />
                              ) : (
                                <Play size={14} />
                              )}
                              {start.isPending &&
                              start.variables?.card.id === card.id
                                ? "Starting…"
                                : "Start work"}
                            </Button>
                          ) : card.work.latest_attempt_id ? (
                            <Button asChild size="sm" variant="outline">
                              <Link
                                to="/projects/$projectId/$section"
                                params={{
                                  projectId: project.id,
                                  section: "work",
                                }}
                                search={{
                                  work: card.id,
                                  view: awaitingReview
                                    ? "workspace"
                                    : undefined,
                                }}
                              >
                                {awaitingReview
                                  ? !reviewer || reviewer === me?.user_id
                                    ? "Review"
                                    : "View review"
                                  : revising &&
                                      controller &&
                                      controller === me?.user_id
                                    ? "Revise"
                                    : "Open thread"}
                              </Link>
                            </Button>
                          ) : null}
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
            {!board.isError && !visible.length && (
              <div className="overview-empty">
                <h2>
                  {archived ? "No archived tasks" : "No tasks in this view"}
                </h2>
                <p>
                  {search || owner || flagged
                    ? "No tasks match these filters."
                    : scope === "completed"
                      ? "Completed tasks will appear here after work is finished."
                      : "Create a task to plan your next piece of work."}
                </p>
                <Button
                  variant="outline"
                  onClick={() => {
                    if (!cards.length) {
                      setCreating(true);
                      return;
                    }
                    setArchived(false);
                    setSearch("");
                    setOwner("");
                    setFlagged(false);
                    setScope("open");
                  }}
                >
                  {cards.length ? "Show open tasks" : "Create task"}
                </Button>
              </div>
            )}
          </div>
        </>
      )}
      <Sheet
        open={!!current}
        onOpenChange={(open) => {
          if (!open && !busy) select("");
        }}
      >
        <SheetContent
          className="task-sheet"
          showCloseButton={false}
          onOpenAutoFocus={(event) => {
            if (taskEntry !== "details") event.preventDefault();
            returnFocus.current =
              document.activeElement instanceof HTMLElement
                ? document.activeElement
                : null;
          }}
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            const previous = returnFocus.current;
            if (previous?.isConnected && previous.closest(".overview-page"))
              previous.focus();
            else
              document
                .querySelector<HTMLInputElement>(
                  '.overview-page input[aria-label="Find work"]',
                )
                ?.focus();
          }}
        >
          <SheetHeader>
            <SheetTitle>Task details</SheetTitle>
            <SheetDescription className="sr-only">
              Description, responsibility, and dependencies.
            </SheetDescription>
            <SheetClose asChild>
              <Button
                variant="ghost"
                size="icon"
                aria-label="Close"
                disabled={busy}
              >
                <X size={18} />
              </Button>
            </SheetClose>
          </SheetHeader>
          {error && (
            <div className="task-panel-error">
              <ErrorNotice error={error} />
            </div>
          )}
          {current && (
            <TaskDetails
              key={current.id}
              card={current}
              cards={cards}
              busy={busy}
              starting={
                start.isPending && start.variables?.card.id === current.id
              }
              canSync={capabilities.data?.can_import === true}
              canStart={canStartTask(current)}
              startReason={startReason(current)}
              entry={taskEntry}
              onOpenTask={(id, projectScope) =>
                select(id, projectScope ? "project" : "graph")
              }
              onStart={() => startWork(current)}
              onSave={(edit) => change.mutateAsync({ card: current, edit })}
              onSync={(direction, discard) =>
                sync.mutate({ card: current, direction, discard })
              }
            />
          )}
        </SheetContent>
      </Sheet>
      {startingCard && (
        <StartWorkDialog
          project={project.id}
          card={startingCard}
          pending={start.isPending}
          error={start.error}
          onClose={() => {
            start.reset();
            setStartingCard(undefined);
          }}
          onStart={(workspace, selection) =>
            start.mutate({ card: startingCard, workspace, selection })
          }
        />
      )}
    </div>
  );
}

function StartWorkDialog({
  project,
  card,
  pending,
  error,
  onClose,
  onStart,
}: {
  project: string;
  card: BoardCard;
  pending: boolean;
  error: unknown;
  onClose: () => void;
  onStart: (workspace: string, selection: AgentModel) => void;
}) {
  const workers = useQuery(workspaceOptions(project));
  const ready = (workers.data || []).filter(
    (worker) =>
      worker.ready &&
      worker.agent_kind !== "hermes" &&
      !worker.retained_preview &&
      (!card.work.workspace_id || worker.id === card.work.workspace_id),
  );
  const [chosenWorker, setChosenWorker] = useState("");
  const [selection, setSelection] = useState<AgentModel>();
  const worker = ready.find((item) => item.id === chosenWorker) || ready[0];
  return (
    <Modal
      title="Start work"
      onClose={() => {
        if (!pending) onClose();
      }}
      closeDisabled={pending}
      closeOnEscape={!pending}
    >
      <Form
        onSubmit={() => {
          if (worker && selection?.model && !pending)
            onStart(worker.id, selection);
        }}
      >
        <label>
          Worker
          <NativeSelect
            value={worker?.id || ""}
            disabled={pending || ready.length < 2}
            onChange={(event) => {
              setChosenWorker(event.target.value);
              setSelection(undefined);
            }}
          >
            {!ready.length && (
              <option value="">
                {workers.isPending ? "Loading workers…" : "No ready workers"}
              </option>
            )}
            {ready.map((item) => (
              <option key={item.id} value={item.id}>
                {item.name || item.id}
              </option>
            ))}
          </NativeSelect>
        </label>
        {worker ? (
          <AgentSelection
            key={worker.id}
            workspace={worker.id}
            provider={selection?.provider || ""}
            model={selection?.model || ""}
            reasoning={selection?.reasoning_effort}
            disabled={pending}
            onReasoning={(effort, provider, model) =>
              setSelection({ provider, model, reasoning_effort: effort })
            }
          />
        ) : (
          !workers.isPending &&
          !workers.isError && (
            <p>Start a worker and load its subscription to choose a model.</p>
          )
        )}
        {workers.isError && (
          <ErrorNotice
            error={workers.error}
            onRetry={() => void workers.refetch()}
          />
        )}
        {!!error && <ErrorNotice error={error} />}
        <Button disabled={pending || !worker || !selection?.model}>
          {pending ? "Starting…" : "Start work"}
        </Button>
      </Form>
    </Modal>
  );
}
