import { useEffect, useRef, useState } from "react";
import { Link } from "@tanstack/react-router";
import { GitBranch, Plus, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { BoardCard } from "./board-api";
import { SearchPicker, taskReference } from "./task-controls";
import type { TaskChange, TaskEntry } from "./task-details";
import { pendingDependencies, teamActivity } from "./team-state";
import { DependencyGraph } from "./task-activity";

export function TaskDependencies({
  card,
  cards,
  entry,
  busy,
  memberLabel,
  onOpenTask,
  onSave,
}: {
  card: BoardCard;
  cards: BoardCard[];
  entry: TaskEntry;
  busy: boolean;
  memberLabel: (id: string) => string | undefined;
  onOpenTask: (id: string, projectScope: boolean) => void;
  onSave: (change: TaskChange) => Promise<boolean>;
}) {
  const section = useRef<HTMLElement>(null);
  const [showOptionalGraph, setShowOptionalGraph] = useState(
    entry === "graph" || entry === "project",
  );
  useEffect(() => {
    if (entry === "details") return;
    const frame = requestAnimationFrame(() => {
      section.current?.focus({ preventScroll: true });
      section.current?.scrollIntoView({ block: "start" });
    });
    return () => cancelAnimationFrame(frame);
  }, [entry]);

  const cardsById = new Map(cards.map((item) => [item.id, item]));
  const dependencies = card.dependencies || [];
  const dependents = cards.filter((item) =>
    item.dependencies?.includes(card.id),
  );
  const hasDependencies = dependencies.length > 0 || dependents.length > 0;
  const showGraph = hasDependencies || showOptionalGraph;
  const pending = new Set(pendingDependencies(card, cardsById));
  return (
    <section
      className="task-dependency-section"
      ref={section}
      tabIndex={-1}
      aria-labelledby="task-dependencies-heading"
    >
      <header className="task-section-heading">
        <h3 id="task-dependencies-heading">Dependencies</h3>
        {!hasDependencies && (
          <Button
            variant={showGraph ? "secondary" : "outline"}
            size="sm"
            aria-expanded={showGraph}
            aria-controls="task-dependency-graph"
            onClick={() => setShowOptionalGraph(!showOptionalGraph)}
          >
            <GitBranch size={15} />
            {showGraph ? "Hide graph" : "View graph"}
          </Button>
        )}
      </header>
      <div id="task-dependency-graph" hidden={!showGraph}>
        {showGraph && (
          <DependencyGraph
            cards={cards}
            initialTask={card.id}
            showProjectInitially={entry === "project"}
            memberLabel={memberLabel}
            onOpenTask={onOpenTask}
          />
        )}
      </div>
      <div className="task-relationships">
        <div className="task-relationship-heading">
          <h4>Depends on</h4>
          <SearchPicker
            label="Add dependency"
            value="Add dependency"
            icon={<Plus size={14} />}
            disabled={busy}
            options={cards
              .filter(
                (item) =>
                  item.id !== card.id && !dependencies.includes(item.id),
              )
              .map((item) => ({
                value: item.id,
                label: `${taskReference(item)} ${item.title || "Untitled task"}`,
              }))}
            onSelect={(id) =>
              void onSave({ dependencies: [...dependencies, id] })
            }
            placeholder="Find a task by title or #…"
          />
        </div>
        {dependencies.length ? (
          <ul>
            {dependencies.map((id) => {
              const dependency = cardsById.get(id);
              return (
                <TaskRelationship
                  key={id}
                  card={dependency}
                  status={
                    pending.has(id) && dependency
                      ? teamActivity(dependency)
                      : "Done"
                  }
                  onRemove={() =>
                    void onSave({
                      dependencies: dependencies.filter(
                        (value) => value !== id,
                      ),
                    })
                  }
                  busy={busy}
                />
              );
            })}
          </ul>
        ) : (
          <p className="task-relationship-empty">No prerequisites.</p>
        )}
      </div>
      {dependents.length > 0 && (
        <div className="task-relationships">
          <div className="task-relationship-heading">
            <h4>Needed by</h4>
          </div>
          <ul>
            {dependents.map((dependent) => (
              <TaskRelationship
                key={dependent.id}
                card={dependent}
                status={
                  dependent.stage === "archived"
                    ? "Archived"
                    : teamActivity(dependent)
                }
              />
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}

function TaskRelationship({
  card,
  status,
  onRemove,
  busy,
}: {
  card?: BoardCard;
  status: string;
  onRemove?: () => void;
  busy?: boolean;
}) {
  return (
    <li className="task-dependency">
      <div className="task-dependency-task">
        {card ? (
          <>
            <Link
              to="/projects/$projectId/$section"
              params={{ projectId: card.project_id, section: "team" }}
              search={{ work: card.id }}
            >
              <span className="task-reference">{taskReference(card)}</span>{" "}
              <span>{card.title || "Untitled task"}</span>
            </Link>
            {card.owner && (
              <span className="task-dependency-owner">{card.owner}</span>
            )}
          </>
        ) : (
          <span>Unavailable prerequisite</span>
        )}
      </div>
      {card && <span className="task-dependency-status">{status}</span>}
      {onRemove && (
        <Button
          variant="ghost"
          size="icon-sm"
          disabled={busy}
          aria-label={
            card
              ? `Remove dependency ${taskReference(card)}`
              : "Remove unavailable dependency"
          }
          onClick={onRemove}
        >
          <X size={14} />
        </Button>
      )}
    </li>
  );
}
