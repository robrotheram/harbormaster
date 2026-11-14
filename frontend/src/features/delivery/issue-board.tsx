import { useRef, useState } from "react";
import { Link } from "@tanstack/react-router";
import {
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  Archive,
  ArrowDownToLine,
  ArrowUpFromLine,
  AlertTriangle,
  GripVertical,
  MoreHorizontal,
  Search,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/native-select";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet";
import { ErrorNotice } from "@/shared/components";
import { label } from "@/shared/format";
import { workStatus, workFinished } from "@/features/work/stages";
import { workKeys } from "@/features/work/api";
import { evidenceURL } from "@/features/work/api";
import {
  conversationTitle,
  instructionOptions,
} from "@/features/work/conversation-title";
import {
  boardApi,
  boardKey,
  boardOptions,
  columns,
  type BoardCard,
  type BoardStage,
} from "./board-api";
import { TicketDescription } from "./ticket-description";
import { NewIssue } from "./new-issue";
import { IssueImport } from "./issue-import";
import type { Project } from "@/shared/types";

type Action = {
  card: BoardCard;
  op: "move" | "rename" | "pull" | "push";
  stage?: BoardStage;
  title?: string;
  description?: string;
  discard?: boolean;
};
export function IssueBoard({ project }: { project: Project }) {
  return <Board key={project.id} project={project} />;
}
function Board({ project }: { project: Project }) {
  const client = useQueryClient();
  const board = useQuery(boardOptions(project.id));
  const cards = board.data?.items || [];
  const instructions = useQueries({
    queries: cards.map((c) => ({
      ...instructionOptions(project.id, c.work),
      enabled: !c.title,
    })),
  });
  const titles = new Map(
    cards.map((c, i) => [
      c.id,
      c.title || conversationTitle(c.work, instructions[i]?.data),
    ]),
  );
  const [search, setSearch] = useState("");
  const [flagged, setFlagged] = useState(false);
  const [archived, setArchived] = useState(false);
  const [creating, setCreating] = useState(false);
  const [importing, setImporting] = useState(false);
  const [selected, setSelected] = useState("");
  const [dragging, setDragging] = useState("");
  const [over, setOver] = useState<BoardStage>();
  const [notice, setNotice] = useState("");
  const lanes = useRef<HTMLDivElement>(null);
  const jumpToColumn = (stage: BoardStage) => {
    const container = lanes.current;
    const column = container?.querySelector<HTMLElement>(
      `[data-stage="${stage}"]`,
    );
    if (!container || !column) return;
    container.scrollTo({
      left:
        container.scrollLeft +
        column.getBoundingClientRect().left -
        container.getBoundingClientRect().left,
      behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches
        ? "instant"
        : "smooth",
    });
  };
  const action = useMutation({
    mutationFn: (a: Action) =>
      a.op === "push" || a.op === "pull"
        ? boardApi.sync(a.card, a.op, a.discard)
        : boardApi.save(
            a.card,
            a.stage || a.card.stage,
            a.title,
            a.description,
          ),
    onMutate: async (a) => {
      await client.cancelQueries({ queryKey: boardKey(project.id) });
      const previous = client.getQueryData<{ items: BoardCard[] }>(
        boardKey(project.id),
      );
      if (a.op === "move" || a.op === "rename")
        client.setQueryData(boardKey(project.id), {
          items:
            previous?.items.map((c) =>
              c.id === a.card.id
                ? { ...c, stage: a.stage || c.stage, title: a.title ?? c.title }
                : c,
            ) || [],
        });
      return { previous };
    },
    onError: (_error, _a, context) => {
      if (context?.previous)
        client.setQueryData(boardKey(project.id), context.previous);
    },
    onSuccess: (saved, a) => {
      client.setQueryData<{ items: BoardCard[] }>(
        boardKey(project.id),
        (old) => ({
          items: old?.items.map((c) => (c.id === saved.id ? saved : c)) || [],
        }),
      );
      setNotice(
        a.op === "push"
          ? "Changes synced to repository."
          : a.op === "pull"
            ? "Updated from repository."
            : workFinished(a.card.work.state) && !workFinished(saved.work.state)
              ? "Work reopened. The agent has not been started."
              : saved.stage === "archived"
                ? "Issue archived."
                : "Issue updated.",
      );
    },
    onSettled: () =>
      Promise.all([
        client.invalidateQueries({ queryKey: boardKey(project.id) }),
        client.invalidateQueries({ queryKey: workKeys.list(project.id) }),
      ]),
  });
  const move = (card: BoardCard, stage: BoardStage) => {
    if (stage !== card.stage && !action.isPending)
      action.mutate({ card, stage, op: "move" });
  };
  const visible = cards.filter(
    (c) =>
      (archived ? c.stage === "archived" : c.stage !== "archived") &&
      (!flagged || c.attention || !!c.source?.error) &&
      `${titles.get(c.id)} ${c.source?.repository || ""} ${c.source?.number || ""} ${c.id}`
        .toLowerCase()
        .includes(search.toLowerCase()),
  );
  const current = cards.find((c) => c.id === selected);
  const renderCard = (card: BoardCard) => (
    <article
      key={card.id}
      className="issue-card"
      data-attention={card.attention || !!card.source?.error}
      data-dragging={dragging === card.id}
      draggable={!action.isPending}
      onDragStart={(e) => {
        setDragging(card.id);
        e.dataTransfer.effectAllowed = "move";
        e.dataTransfer.setData("text/plain", card.id);
      }}
      onDragEnd={() => {
        setDragging("");
        setOver(undefined);
      }}
    >
      <div className="issue-card-top">
        <span className="issue-card-reference">
          {card.source ? `#${card.source.number}` : card.id.slice(-7)}
        </span>
        <span className="issue-card-kind">{card.work.kind}</span>
        <GripVertical
          size={14}
          className="issue-drag-handle"
          aria-label="Drag to another column"
        />
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="size-7"
              aria-label={`Actions for ${titles.get(card.id)}`}
            >
              <MoreHorizontal size={16} />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuLabel>Move to</DropdownMenuLabel>
            {columns.map((col) => (
              <DropdownMenuItem
                key={col.id}
                disabled={action.isPending || card.stage === col.id}
                onSelect={() => move(card, col.id)}
              >
                {col.label}
              </DropdownMenuItem>
            ))}
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={() => setSelected(card.id)}>
              Edit issue
            </DropdownMenuItem>
            <DropdownMenuItem
              disabled={action.isPending}
              onSelect={() =>
                move(
                  card,
                  card.stage === "archived"
                    ? workFinished(card.work.state)
                      ? "done"
                      : card.archived_from || "planned"
                    : "archived",
                )
              }
            >
              <Archive size={14} />
              {card.stage === "archived" ? "Restore issue" : "Archive issue"}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      <button className="issue-card-title" onClick={() => setSelected(card.id)}>
        {titles.get(card.id)}
      </button>
      {(card.attention || card.source?.error) && (
        <div className="issue-card-alert">
          <AlertTriangle size={13} />
          {card.source?.error
            ? "Sync needs attention"
            : card.work.state === "failed"
              ? "Worker failed"
              : card.work.state === "cancelled"
                ? "Work cancelled"
                : "Work needs attention"}
        </div>
      )}
      <div className="issue-card-footer">
        {card.source && workFinished(card.work.state) && (
          <span>{label(workStatus(card.work.state))}</span>
        )}
        {card.source ? (
          <>
            <span className="truncate" title={card.source.repository}>
              {card.source.repository}
            </span>
            <span
              className="issue-sync"
              data-dirty={card.dirty || !!card.source.error}
            >
              {card.source.error
                ? "Sync failed"
                : card.dirty
                  ? "Not synced"
                  : "Synced"}
            </span>
          </>
        ) : (
          <span>
            {workFinished(card.work.state)
              ? label(workStatus(card.work.state))
              : "Factory work"}
          </span>
        )}
      </div>
    </article>
  );
  return (
    <div className="issue-board-page">
      <div className="board-heading">
        <div>
          <h1>Issue board</h1>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" onClick={() => setArchived(!archived)}>
            <Archive size={15} />
            {archived
              ? "Back to board"
              : `Archive (${cards.filter((c) => c.stage === "archived").length})`}
          </Button>
          <Button onClick={() => setCreating(true)}>New issue</Button>
          <Button variant="outline" onClick={() => setImporting(!importing)}>
            <ArrowDownToLine size={15} />
            Import issues
          </Button>
        </div>
      </div>
      <div className="board-toolbar">
        <div className="board-search">
          <Search size={15} />
          <Input
            className="h-8"
            aria-label="Search issues"
            placeholder="Search issues…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
        <Button
          variant={flagged ? "secondary" : "ghost"}
          aria-pressed={flagged}
          onClick={() => setFlagged(!flagged)}
        >
          <AlertTriangle size={14} />
          Flagged
        </Button>
        <span className="board-item-count">
          {board.isPending || board.isError
            ? "— issues"
            : `${visible.length} ${visible.length === 1 ? "issue" : "issues"}`}
        </span>
        <span className="board-drag-hint">
          Drag cards to move · Use the card menu on touch or keyboard
        </span>
      </div>
      {action.isError && <ErrorNotice error={action.error} />}{" "}
      {notice && (
        <p className="sr-only" role="status">
          {notice}
        </p>
      )}
      {creating && (
        <NewIssue
          project={project.id}
          onClose={() => setCreating(false)}
          onCreated={(id) => {
            setArchived(false);
            setSearch("");
            setFlagged(false);
            setSelected(id);
          }}
        />
      )}
      {importing && (
        <IssueImport project={project.id} onClose={() => setImporting(false)} />
      )}
      {board.isError && (
        <div>
          <ErrorNotice error={board.error} />
          <Button
            variant="outline"
            disabled={board.isFetching}
            onClick={() => void board.refetch()}
          >
            {board.isFetching ? "Retrying…" : "Retry loading board"}
          </Button>
          {board.data && (
            <p className="mt-2 text-sm text-muted-foreground">
              Showing the last loaded issues. Refresh before making further
              changes.
            </p>
          )}
        </div>
      )}
      {board.isPending && <p role="status">Loading issue board…</p>}
      {board.data && (
        <>
          {!archived && (
            <nav aria-label="Board columns" className="board-column-navigation">
              {columns.map((col) => (
                <Button
                  key={col.id}
                  variant="outline"
                  aria-controls={`board-column-${col.id}`}
                  onClick={() => jumpToColumn(col.id)}
                >
                  <span>{col.label}</span>
                  <span className="board-column-count">
                    {visible.filter((c) => c.stage === col.id).length}
                  </span>
                </Button>
              ))}
            </nav>
          )}
          {archived ? (
            <section className="board-archive">
              <h2>Archived issues</h2>
              <p>
                Hidden from the active board. Restore an issue from its menu.
              </p>
              <div className="board-archive-grid">
                {visible.map(renderCard)}
              </div>
              {!visible.length && <p>No archived issues match this view.</p>}
            </section>
          ) : (
            <div
              className="board-lanes"
              ref={lanes}
              tabIndex={0}
              role="region"
              aria-label="Issue board columns"
            >
              {columns.map((col) => {
                const items = visible.filter((c) => c.stage === col.id);
                return (
                  <section
                    key={col.id}
                    id={`board-column-${col.id}`}
                    className="board-lane"
                    data-stage={col.id}
                    data-over={over === col.id}
                    aria-label={`${col.label} column`}
                    onDragOver={(e) => {
                      e.preventDefault();
                      e.dataTransfer.dropEffect = "move";
                      setOver(col.id);
                    }}
                    onDragLeave={(e) => {
                      if (!e.currentTarget.contains(e.relatedTarget as Node))
                        setOver(undefined);
                    }}
                    onDrop={(e) => {
                      e.preventDefault();
                      const id = e.dataTransfer.getData("text/plain");
                      const card = cards.find((c) => c.id === id);
                      if (card) move(card, col.id);
                      setDragging("");
                      setOver(undefined);
                    }}
                  >
                    <header>
                      <span className="board-stage-dot" />
                      <h2>{col.label}</h2>
                      <span className="board-column-count">{items.length}</span>
                    </header>
                    <div className="board-lane-cards">
                      {items.map(renderCard)}
                      {!items.length && (
                        <div className="board-empty-lane">
                          {dragging
                            ? "Drop issue here"
                            : search || flagged
                              ? "No matching issues"
                              : col.id === "planned"
                                ? "Use New issue to plan your next task."
                                : col.id === "running"
                                  ? "Issues being worked on appear here."
                                  : col.id === "review"
                                    ? "Completed work awaiting review appears here."
                                    : "Approved or closed issues appear here."}
                        </div>
                      )}
                    </div>
                  </section>
                );
              })}
            </div>
          )}
        </>
      )}
      <Sheet
        open={!!current}
        onOpenChange={(open) => {
          if (!open) setSelected("");
        }}
      >
        <SheetContent className="overflow-y-auto sm:max-w-5xl">
          <SheetHeader>
            <SheetTitle>Issue details</SheetTitle>
            <SheetDescription>
              Edit the issue, move it, or sync its repository status.
            </SheetDescription>
          </SheetHeader>
          {action.isError && (
            <div className="px-6">
              <ErrorNotice error={action.error} />
            </div>
          )}
          {current && (
            <CardDetails
              key={current.id}
              card={current}
              title={titles.get(current.id) || ""}
              busy={action.isPending}
              onAction={(a) => action.mutate(a)}
              onDescription={(description) =>
                action.mutateAsync({ card: current, op: "rename", description })
              }
            />
          )}
        </SheetContent>
      </Sheet>
    </div>
  );
}
function CardDetails({
  card,
  title,
  busy,
  onAction,
  onDescription,
}: {
  card: BoardCard;
  title: string;
  busy: boolean;
  onAction: (a: Action) => void;
  onDescription: (description: string) => Promise<unknown>;
}) {
  const [draft, setDraft] = useState(title);
  const source = card.source;
  return (
    <div className="board-details ticket-details">
      <div className="ticket-heading">
        <div className="ticket-reference">
          <span className="ticket-state">
            {card.stage === "archived"
              ? "Archived"
              : card.stage === "done"
                ? workFinished(card.work.state)
                  ? label(workStatus(card.work.state))
                  : "Closed"
                : "Open"}
          </span>
          <span>
            {source
              ? `${source.repository} #${source.number}`
              : `Local issue · ${card.id.slice(-7)}`}
          </span>
        </div>
        <h2>{title}</h2>
        <details className="ticket-title-edit">
          <summary>Edit title</summary>
          <label>
            Issue title
            <Input
              aria-label="Issue title"
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              maxLength={256}
            />
          </label>
          <Button
            disabled={busy || !draft.trim() || draft === title}
            onClick={() =>
              onAction({ card, op: "rename", title: draft.trim() })
            }
          >
            Save title
          </Button>
        </details>
      </div>
      <div className="ticket-layout">
        <TicketDescription card={card} busy={busy} onSave={onDescription} />
        <aside className="ticket-properties">
          <label>
            Status
            <NativeSelect
              aria-label="Issue column"
              value={card.stage}
              disabled={busy}
              onChange={(e) =>
                onAction({
                  card,
                  op: "move",
                  stage: e.target.value as BoardStage,
                })
              }
            >
              {columns.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.label}
                </option>
              ))}
              <option value="archived">Archive</option>
            </NativeSelect>
          </label>
          <div className="board-detail-section">
            <h3>Factory work</h3>
            <p>Work status: {label(workStatus(card.work.state))}</p>
            {workFinished(card.work.state) && (
              <p>
                Move to Planned, In progress or Review to reopen this task. The
                agent starts only when you ask.
              </p>
            )}
            <Link
              className="board-text-link"
              to="/projects/$projectId/$section"
              params={{ projectId: card.project_id, section: "work" }}
              search={{ work: card.id }}
            >
              Open work thread
            </Link>
            {card.work.state === "awaiting_review" && (
              <Link
                className="board-text-link"
                to="/projects/$projectId/$section"
                params={{ projectId: card.project_id, section: "reviews" }}
              >
                Review delivery
              </Link>
            )}
          </div>
          {source && (
            <div className="board-detail-section">
              <h3>
                {source.repository} #{source.number}
              </h3>
              <p>
                {source.error
                  ? "Sync needs attention"
                  : card.dirty
                    ? "Local changes waiting to sync"
                    : "Up to date at last sync"}
              </p>
              <p className="text-xs">
                Last synced {new Date(source.synced_at).toLocaleString()}
              </p>
              <p className="text-xs">
                Sync sends the title, description and open/closed status. Done
                closes the issue; Planned, In progress and Review keep it open.
                Archiving stays local.
              </p>
              {evidenceURL(source.url) && (
                <a
                  className="board-text-link"
                  href={evidenceURL(source.url)}
                  target="_blank"
                  rel="noreferrer"
                >
                  Open repository issue
                </a>
              )}
              <div className="flex flex-wrap gap-2">
                <Button
                  disabled={busy}
                  onClick={() => onAction({ card, op: "push" })}
                >
                  <ArrowUpFromLine size={14} />
                  Sync to repository
                </Button>
                <Button
                  variant="outline"
                  disabled={busy}
                  onClick={() => onAction({ card, op: "pull" })}
                >
                  <ArrowDownToLine size={14} />
                  Refresh from repository
                </Button>
              </div>
              {source.error && (
                <>
                  <p className="text-sm text-amber-600 dark:text-amber-300">
                    {source.error.startsWith("BOARD_SYNC")
                      ? "Local and repository changes need a decision. Keep your edits, or replace the local title, description and status with the repository version."
                      : "The last sync failed. Check the account and repository access, then retry."}
                  </p>
                  {source.error.startsWith("BOARD_SYNC") && (
                    <Button
                      variant="outline"
                      disabled={busy}
                      onClick={() =>
                        onAction({ card, op: "pull", discard: true })
                      }
                    >
                      Use repository version
                    </Button>
                  )}
                </>
              )}
            </div>
          )}
        </aside>
      </div>
    </div>
  );
}
