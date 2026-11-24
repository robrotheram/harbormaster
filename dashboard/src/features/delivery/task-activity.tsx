import { AlertTriangle, ArrowRight } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { ErrorNotice } from "@/shared/components";
import { APIError } from "@/shared/http";
import { workApi, workKeys } from "@/features/work/api";
import { workFinished } from "@/features/work/stages";
import type { BoardCard } from "./board-api";
import { taskReference } from "./task-controls";
import { activityTone, pendingDependencies, teamActivity } from "./team-state";

const laneWidth = 258;
const nodeWidth = 214;
const nodeHeight = 104;
const rowHeight = 126;
const margin = 20;

function layout(cards: ReadonlyMap<string, BoardCard>, focus?: string) {
  const included = new Set<string>();
  const pending = focus
    ? [
        focus,
        ...[...cards.values()]
          .filter((card) => card.dependencies?.includes(focus))
          .map((card) => card.id),
      ]
    : [...cards.values()]
        .filter((card) => card.stage !== "archived")
        .map((card) => card.id);
  while (pending.length) {
    const id = pending.pop()!;
    if (included.has(id)) continue;
    included.add(id);
    pending.push(...(cards.get(id)?.dependencies || []));
  }
  const edges = [...included].flatMap((to) =>
    (cards.get(to)?.dependencies || []).map((from) => ({ from, to })),
  );
  const children = new Map<string, string[]>();
  const incoming = new Map([...included].map((id) => [id, 0]));
  for (const { from, to } of edges) {
    const list = children.get(from) || [];
    list.push(to);
    children.set(from, list);
    incoming.set(to, incoming.get(to)! + 1);
  }
  const lanes = new Map<string, number>();
  const queue = [...included].filter((id) => !incoming.get(id));
  for (let index = 0; index < queue.length; index++) {
    const id = queue[index];
    const lane = lanes.get(id) || 0;
    lanes.set(id, lane);
    for (const child of children.get(id) || []) {
      lanes.set(child, Math.max(lanes.get(child) || 0, lane + 1));
      const remaining = incoming.get(child)! - 1;
      incoming.set(child, remaining);
      if (remaining === 0) queue.push(child);
    }
  }
  const unresolved = [...included].filter((id) => incoming.get(id));
  let lastLane = 0;
  for (const id of queue) lastLane = Math.max(lastLane, lanes.get(id)!);
  for (const id of unresolved) lanes.set(id, queue.length ? lastLane + 1 : 0);
  const rows = new Map<number, number>();
  const nodes = [...included]
    .sort((a, b) => (cards.get(a)?.number || 0) - (cards.get(b)?.number || 0))
    .map((id) => {
      const lane = lanes.get(id) || 0;
      const row = rows.get(lane) || 0;
      rows.set(lane, row + 1);
      return {
        id,
        lane,
        x: margin + lane * laneWidth,
        y: margin + row * rowHeight,
      };
    });
  const rowCount = Math.max(1, ...rows.values());
  return {
    nodes: nodes.map(({ lane, ...node }) => ({
      ...node,
      y: node.y + ((rowCount - rows.get(lane)!) * rowHeight) / 2,
    })),
    edges,
    unresolved,
    width: Math.max(1, rows.size) * laneWidth + margin,
    height: rowCount * rowHeight + margin,
  };
}

function Connections({
  graph,
  selected,
}: {
  graph: ReturnType<typeof layout>;
  selected?: string;
}) {
  const canvas = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    const element = canvas.current;
    const ctx = element?.getContext("2d");
    if (!element || !ctx) return;
    const ratio = Math.min(
      window.devicePixelRatio || 1,
      2,
      8192 / Math.max(graph.width, graph.height),
    );
    element.width = Math.ceil(graph.width * ratio);
    element.height = Math.ceil(graph.height * ratio);
    ctx.scale(ratio, ratio);
    const palette = getComputedStyle(element);
    const positions = new Map(graph.nodes.map((node) => [node.id, node]));
    for (const edge of graph.edges) {
      const from = positions.get(edge.from);
      const to = positions.get(edge.to);
      if (!from || !to) continue;
      const highlighted = selected === edge.from || selected === edge.to;
      ctx.strokeStyle = palette
        .getPropertyValue(
          highlighted ? "--task-graph-accent" : "--muted-foreground",
        )
        .trim();
      ctx.fillStyle = ctx.strokeStyle;
      ctx.globalAlpha = highlighted ? 0.9 : 0.48;
      ctx.lineWidth = highlighted ? 2 : 1.4;
      const sameLane = from.x === to.x;
      const startX = from.x + nodeWidth;
      const endX = sameLane ? to.x + nodeWidth : to.x;
      const startY = from.y + nodeHeight / 2;
      const endY = to.y + nodeHeight / 2;
      const bend = sameLane ? startX + 28 : startX + (endX - startX) / 2;
      ctx.beginPath();
      ctx.moveTo(startX, startY);
      ctx.bezierCurveTo(bend, startY, bend, endY, endX, endY);
      ctx.stroke();
      ctx.beginPath();
      ctx.moveTo(endX, endY);
      ctx.lineTo(endX + (sameLane ? 7 : -7), endY - 4);
      ctx.lineTo(endX + (sameLane ? 7 : -7), endY + 4);
      ctx.closePath();
      ctx.fill();
    }
  }, [graph, selected]);
  return (
    <canvas
      aria-hidden="true"
      ref={canvas}
      style={{ width: graph.width, height: graph.height }}
    />
  );
}

function status(card: BoardCard) {
  if (card.stage === "archived") return "Archived";
  if (card.stage === "done" && !workFinished(card.work.state)) return "Done";
  return teamActivity(card);
}

export function TaskRunDetails({
  card,
  memberLabel,
}: {
  card: BoardCard;
  memberLabel: (id: string) => string | undefined;
}) {
  const { controller_id, reviewer_id, latest_attempt_id, state } = card.work;
  const attempts = useQuery({
    queryKey: workKeys.attempts(card.project_id, card.id),
    queryFn: ({ signal }) => workApi.attempts(card.project_id, card.id, signal),
    enabled: !!latest_attempt_id,
    refetchInterval: 5000,
  });
  const attempt = attempts.data?.find((item) => item.id === latest_attempt_id);
  const error = workFinished(state) ? undefined : attempt?.error_code;
  const review = reviewer_id || state === "awaiting_review";
  if (
    !controller_id &&
    !review &&
    !error &&
    !attempt?.review_feedback &&
    !card.source?.error
  )
    return null;

  return (
    <section className="task-run-details" aria-label="Execution">
      <h3>Execution</h3>
      {(controller_id || review) && (
        <dl>
          {controller_id && (
            <div>
              <dt>Agent controller</dt>
              <dd>{memberLabel(controller_id) || "Assigned member"}</dd>
            </div>
          )}
          {review && (
            <div>
              <dt>Reviewer</dt>
              <dd>
                {reviewer_id
                  ? memberLabel(reviewer_id) || "Assigned member"
                  : "Unassigned"}
              </dd>
            </div>
          )}
        </dl>
      )}
      {error && <ErrorNotice error={new APIError(error, 409)} />}
      {attempt?.review_feedback && (
        <p className="activity-review-feedback">{attempt.review_feedback}</p>
      )}
      {card.source?.error && (
        <p className="activity-map-warning">
          <AlertTriangle size={15} /> Repository sync needs attention.
        </p>
      )}
    </section>
  );
}

export function DependencyGraph({
  cards,
  initialTask,
  memberLabel,
  showProjectInitially = false,
  onOpenTask,
}: {
  cards: BoardCard[];
  initialTask: string;
  memberLabel: (id: string) => string | undefined;
  showProjectInitially?: boolean;
  onOpenTask: (id: string, projectScope: boolean) => void;
}) {
  const [selected, setSelected] = useState(initialTask);
  const [showProject, setShowProject] = useState(showProjectInitially);
  const index = useMemo(
    () => new Map(cards.map((card) => [card.id, card])),
    [cards],
  );
  const graph = useMemo(
    () => layout(index, showProject ? undefined : initialTask),
    [index, initialTask, showProject],
  );
  const activeId = selected || graph.nodes[0]?.id;
  const active = activeId ? index.get(activeId) : undefined;
  const pending = new Set(active ? pendingDependencies(active, index) : []);
  const dependents = cards.filter(
    (card) => active && card.dependencies?.includes(active.id),
  );
  const selectRelated = (id: string) => {
    if (!graph.nodes.some((node) => node.id === id)) setShowProject(true);
    setSelected(id);
  };
  const related = (ids: string[]) => (
    <ul className="activity-related">
      {ids.map((id) => {
        const card = index.get(id);
        return (
          <li key={id}>
            <button type="button" onClick={() => selectRelated(id)}>
              {card?.title || "Unavailable task"}
            </button>
            <small>
              {card
                ? `${status(card)} · ${card.owner || "Unassigned"}`
                : "Unavailable"}
            </small>
            {pending.has(id) && (
              <AlertTriangle size={14} aria-label="Unfinished" />
            )}
          </li>
        );
      })}
    </ul>
  );
  return (
    <div className="task-activity">
      <section className="activity-map" aria-label="Dependency graph">
        <header>
          <div
            className="activity-map-switch"
            role="group"
            aria-label="Dependency graph scope"
          >
            <Button
              size="sm"
              variant={showProject ? "ghost" : "secondary"}
              aria-pressed={!showProject}
              onClick={() => {
                setShowProject(false);
                setSelected(initialTask);
              }}
            >
              Task dependencies
            </Button>
            <Button
              size="sm"
              variant={showProject ? "secondary" : "ghost"}
              aria-pressed={showProject}
              onClick={() => setShowProject(true)}
            >
              Entire project
            </Button>
          </div>
          <small>
            Prerequisite <ArrowRight size={12} /> dependent task
          </small>
        </header>
        {graph.nodes.length ? (
          <div
            className="activity-map-scroll"
            tabIndex={0}
            aria-label="Scrollable dependency graph"
          >
            <div
              className="activity-map-surface"
              style={{ width: graph.width, height: graph.height }}
            >
              <Connections graph={graph} selected={activeId} />
              {graph.nodes.map(({ id, x, y }) => {
                const card = index.get(id);
                const blocked =
                  card && pendingDependencies(card, index).length > 0;
                return (
                  <button
                    className="activity-map-node"
                    key={id}
                    type="button"
                    aria-pressed={activeId === id}
                    onClick={() => setSelected(id)}
                    style={{
                      left: x,
                      top: y,
                      width: nodeWidth,
                      height: nodeHeight,
                    }}
                  >
                    <span className="activity-map-node-top">
                      <span>
                        {card ? taskReference(card) : "Missing prerequisite"}
                      </span>
                      {card && (
                        <span
                          className={`task-pill activity-${activityTone(card)}`}
                        >
                          <i />
                          {status(card)}
                        </span>
                      )}
                    </span>
                    <strong title={card?.title}>
                      {card?.title || "Unavailable task"}
                    </strong>
                    <small>
                      <span>{card?.owner || "Unassigned"}</span>
                      {blocked && (
                        <AlertTriangle
                          size={13}
                          aria-label="Dependency warning"
                        />
                      )}
                    </small>
                  </button>
                );
              })}
            </div>
          </div>
        ) : (
          <p className="activity-map-empty">No tasks to show.</p>
        )}
        {graph.unresolved.length > 0 && (
          <p className="activity-map-warning">
            <AlertTriangle size={15} /> A dependency loop prevents some tasks
            from being ordered.
          </p>
        )}
      </section>
      {active && active.id !== initialTask ? (
        <section
          className="activity-inspector"
          aria-label="Selected task activity"
        >
          <header>
            <div>
              <span className="task-reference">{taskReference(active)}</span>
              <h4>{active.title || "Untitled task"}</h4>
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={() => onOpenTask(active.id, showProject)}
            >
              Open task <ArrowRight size={14} />
            </Button>
          </header>
          <div className="activity-inspector-meta">
            <span className={`task-pill activity-${activityTone(active)}`}>
              <i />
              {status(active)}
            </span>
            <span>Assignee: {active.owner || "Unassigned"}</span>
          </div>
          <TaskRunDetails card={active} memberLabel={memberLabel} />
          {pending.size > 0 && (
            <p className="activity-map-warning">
              <AlertTriangle size={15} />
              {active.require_dependencies
                ? "Required dependencies must finish before work can start."
                : "Some dependencies are unfinished or unavailable."}
            </p>
          )}
          {!!active.dependencies?.length && (
            <>
              <h5>Depends on</h5>
              {related(active.dependencies)}
            </>
          )}
          {dependents.length > 0 && (
            <>
              <h5>Needed by</h5>
              {related(dependents.map((card) => card.id))}
            </>
          )}
        </section>
      ) : (
        activeId &&
        !active && (
          <p className="activity-map-empty">
            This prerequisite is unavailable.
          </p>
        )
      )}
    </div>
  );
}
