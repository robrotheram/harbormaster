import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  Copy,
  Loader2,
  Pencil,
  Play,
  X,
  UserRound,
  Archive,
  ExternalLink,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { NativeSelect } from "@/components/ui/native-select";
import { ErrorNotice } from "@/shared/components";
import { memberOptions } from "@/features/work/responsibility";
import { instructionOptions } from "@/features/work/conversation-title";
import { userMessage } from "@/features/work/conversation-api";
import { TicketMarkdown } from "./ticket-description";
import { taskReference, PriorityPill, SearchPicker } from "./task-controls";
import { teamActivity, activityTone } from "./team-state";
import { columns } from "./board-api";
import type { BoardCard, BoardStage } from "./board-api";

export type TaskChange = {
  title?: string;
  description?: string;
  stage?: BoardStage;
  owner_id?: string;
  priority?: string;
  dependencies?: string[];
};
export function TaskDetails({
  card,
  cards,
  busy,
  starting,
  canStart,
  canSync,
  startReason,
  onStart,
  onSave,
  onSync,
}: {
  card: BoardCard;
  cards: BoardCard[];
  busy: boolean;
  starting: boolean;
  canStart: boolean;
  canSync: boolean;
  startReason?: string;
  onStart: () => void;
  onSave: (change: TaskChange) => Promise<unknown>;
  onSync: (direction: "pull" | "push", discard?: boolean) => void;
}) {
  const [copyStatus, setCopyStatus] = useState("");
  const roster = useQuery(memberOptions(card.project_id));
  const members = roster.data?.items.filter((m) => m.status === "active") || [];
  const me = members.find((m) => m.user_id === roster.data?.user_id);
  const instruction = useQuery({
    ...instructionOptions(card.project_id, card.work),
    enabled: card.description === undefined && !!card.work.specification_path,
  });
  const content =
    card.description ??
    (instruction.data
      ? userMessage(instruction.data, {
          parent_attempt_id: "followup",
        }).replace(/^# [^\n]+\n\n/, "")
      : "");
  const [editing, setEditing] = useState(false),
    [title, setTitle] = useState(""),
    [description, setDescription] = useState(""),
    [preview, setPreview] = useState(false);
  const save = async (change: TaskChange) => {
    try {
      await onSave(change);
      return true;
    } catch {
      return false;
    }
  };
  const dependencies = (card.dependencies || [])
    .map((id) => cards.find((c) => c.id === id))
    .filter((c): c is BoardCard => !!c);
  const missing = (card.dependencies || []).filter(
    (id) => !cards.some((c) => c.id === id),
  );
  const firstRun =
    !card.work.latest_attempt_id &&
    ["planned", "draft", "in_progress"].includes(card.work.state) &&
    card.stage !== "archived";
  return (
    <div className="task-detail-content">
      <div className="task-detail-top">
        <div className="task-reference-group">
          <span className="task-reference">{taskReference(card)}</span>
          <button
            className="task-copy-link"
            aria-label="Copy task link"
            onClick={async () => {
              try {
                await navigator.clipboard.writeText(window.location.href);
                setCopyStatus("Link copied");
              } catch {
                setCopyStatus(
                  "Copy the address from your browser to share this task.",
                );
              }
            }}
          >
            <Copy size={13} />
          </button>
          <span role="status" className="task-field-hint">
            {copyStatus}
          </span>
        </div>
        <span
          className={`task-pill activity-${card.stage === "archived" ? "planned" : activityTone(card)}`}
        >
          {card.stage === "archived" ? "Archived" : teamActivity(card)}
        </span>
        <PriorityPill priority={card.priority} />
      </div>
      <div className="task-detail-title">
        <h2>{card.title || "Untitled task"}</h2>
        {!editing && (
          <Button
            variant="outline"
            disabled={
              busy || (card.description === undefined && instruction.isFetching)
            }
            onClick={() => {
              setTitle(card.title || "");
              setDescription(content);
              setPreview(false);
              setEditing(true);
            }}
          >
            <Pencil size={14} />
            Edit
          </Button>
        )}
      </div>
      <div className="task-detail-actions">
        {firstRun ? (
          <Button disabled={busy || editing || !canStart} onClick={onStart}>
            {starting ? (
              <Loader2 size={14} className="loading-spinner" />
            ) : (
              <Play size={14} />
            )}
            {starting ? "Starting…" : "Start work"}
          </Button>
        ) : card.work.latest_attempt_id ? (
          <Button asChild>
            <Link
              to="/projects/$projectId/$section"
              params={{
                projectId: card.project_id,
                section: "work",
              }}
              search={{ work: card.id }}
            >
              Open thread
            </Link>
          </Button>
        ) : null}
        <Button
          variant="ghost"
          disabled={busy || editing}
          onClick={() =>
            void save({
              stage:
                card.stage === "archived"
                  ? card.archived_from || "planned"
                  : "archived",
            })
          }
        >
          <Archive size={14} />
          {card.stage === "archived" ? "Restore task" : "Archive"}
        </Button>
      </div>
      {firstRun && !canStart && (
        <p className="task-guidance">
          {startReason || "Checking execution availability…"}
        </p>
      )}
      <div className="task-detail-grid">
        <section className="task-body" aria-label="Task content">
          {editing ? (
            <form
              className="task-edit-form"
              onSubmit={async (e) => {
                e.preventDefault();
                if (await save({ title: title.trim(), description }))
                  setEditing(false);
              }}
            >
              <label>
                Title
                <Input
                  autoFocus
                  aria-label="Task title"
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                  maxLength={256}
                  required
                  disabled={busy}
                />
              </label>
              <div className="task-description-heading">
                <label htmlFor="task-description">Description</label>
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  aria-pressed={preview}
                  onClick={() => setPreview(!preview)}
                >
                  {preview ? "Write" : "Preview"}
                </Button>
              </div>
              {preview ? (
                <TicketMarkdown content={description} />
              ) : (
                <Textarea
                  id="task-description"
                  aria-label="Task description"
                  rows={12}
                  maxLength={60000}
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  disabled={busy}
                />
              )}
              <small className="text-muted-foreground">
                Markdown supported
              </small>
              <div className="task-edit-actions">
                <Button type="submit" disabled={busy || !title.trim()}>
                  Save changes
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  disabled={busy}
                  onClick={() => setEditing(false)}
                >
                  Cancel
                </Button>
              </div>
            </form>
          ) : (
            <>
              <h3>Description</h3>
              {instruction.isError && <ErrorNotice error={instruction.error} />}
              <TicketMarkdown
                content={
                  content || "No description yet. Select Edit to add one."
                }
              />
            </>
          )}
        </section>
        <aside className="task-property-panel" aria-label="Task properties">
          <section>
            <div className="task-property-heading">
              <h3>Assignee</h3>
              {me && card.owner_id !== me.user_id && (
                <button
                  className="task-inline-action"
                  disabled={busy}
                  onClick={() => void save({ owner_id: me.user_id })}
                >
                  Assign to me
                </button>
              )}
            </div>
            <SearchPicker
              label="Assignee"
              value={card.owner || "Unassigned"}
              disabled={busy || roster.isPending || roster.isError}
              options={[
                { value: "", label: "Unassigned" },
                ...members.map((m) => ({
                  value: m.user_id,
                  label: m.email,
                  detail:
                    m.role === "admin"
                      ? "Project administrator"
                      : "Project member",
                })),
              ]}
              onSelect={(owner_id) => void save({ owner_id })}
              placeholder="Find a project member…"
            />
            {roster.isError && <ErrorNotice error={roster.error} />}
            <p className="task-field-hint">
              <UserRound size={12} />
              Assignment tracks responsibility.
            </p>
          </section>
          <section>
            <h3>Priority</h3>
            <NativeSelect
              aria-label="Priority"
              value={card.priority || "normal"}
              disabled={busy}
              onChange={(e) => void save({ priority: e.target.value })}
            >
              <option value="high">↑ High priority</option>
              <option value="normal">− Normal priority</option>
              <option value="low">↓ Low priority</option>
            </NativeSelect>
          </section>
          <section>
            <h3>Dependencies</h3>
            <div className="task-dependencies">
              {dependencies.map((dep) => (
                <div className="task-dependency" key={dep.id}>
                  <Link
                    to="/projects/$projectId/$section"
                    params={{ projectId: card.project_id, section: "team" }}
                    search={{ work: dep.id }}
                  >
                    <span>{taskReference(dep)}</span>
                    {dep.title || "Untitled task"}
                  </Link>
                  <button
                    disabled={busy}
                    aria-label={`Remove dependency ${taskReference(dep)}`}
                    onClick={() =>
                      void save({
                        dependencies: (card.dependencies || []).filter(
                          (id) => id !== dep.id,
                        ),
                      })
                    }
                  >
                    <X size={14} />
                  </button>
                </div>
              ))}
              {missing.map((id) => (
                <div className="task-dependency" key={id}>
                  <span>Unavailable dependency</span>
                  <button
                    disabled={busy}
                    aria-label="Remove unavailable dependency"
                    onClick={() =>
                      void save({
                        dependencies: (card.dependencies || []).filter(
                          (v) => v !== id,
                        ),
                      })
                    }
                  >
                    <X size={14} />
                  </button>
                </div>
              ))}
            </div>
            <SearchPicker
              label="Add dependency"
              value="Add dependency"
              disabled={busy}
              options={cards
                .filter(
                  (c) => c.id !== card.id && !card.dependencies?.includes(c.id),
                )
                .map((c) => ({
                  value: c.id,
                  label: `${taskReference(c)} ${c.title || "Untitled task"}`,
                }))}
              onSelect={(id) =>
                void save({ dependencies: [...(card.dependencies || []), id] })
              }
              placeholder="Find a task by title or #…"
            />
          </section>
          <section>
            <h3>Status</h3>
            <NativeSelect
              aria-label="Task status"
              value={card.stage}
              disabled={
                busy ||
                card.stage === "archived" ||
                !!card.work.latest_attempt_id
              }
              onChange={(e) =>
                void save({ stage: e.target.value as BoardStage })
              }
            >
              {columns.map((column) => (
                <option key={column.id} value={column.id}>
                  {column.label}
                </option>
              ))}
              {card.stage === "archived" && (
                <option value="archived">Archived</option>
              )}
            </NativeSelect>
            <p className="task-field-hint">
              {card.work.latest_attempt_id
                ? "Status follows the run and review."
                : "Changing status does not start an agent."}
            </p>
          </section>
          {card.source && (
            <section>
              <h3>Repository issue</h3>
              <a
                className="task-inline-action"
                href={card.source.url}
                target="_blank"
                rel="noreferrer"
              >
                {card.source.repository} #{card.source.number}
                <ExternalLink size={12} />
              </a>
              <p className="task-field-hint">
                {card.dirty
                  ? "Local changes not synced"
                  : "Imported from repository"}
              </p>
              {canSync ? (
                <div className="task-sync-actions">
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={busy}
                    onClick={() => onSync("pull")}
                  >
                    Refresh from repository
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={busy || !card.dirty}
                    onClick={() => onSync("push")}
                  >
                    Push changes
                  </Button>
                  {card.dirty && (
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={busy}
                      onClick={() => onSync("pull", true)}
                    >
                      Use repository version
                    </Button>
                  )}
                </div>
              ) : (
                <p className="task-field-hint">
                  Ask a platform operator to sync repository changes.
                </p>
              )}
            </section>
          )}
        </aside>
      </div>
    </div>
  );
}
