import type { BoardCard } from "./board-api";
import { workStatus, workFinished } from "@/features/work/stages";
import { label } from "@/shared/format";
export function teamScope(card: BoardCard) {
  if (workFinished(card.work.state)) return "completed";
  if (["draft", "planned"].includes(card.work.state)) return "planned";
  return "active";
}
export function teamActivity(card: BoardCard) {
  if (["draft", "planned"].includes(card.work.state)) return "Not started";
  if (card.work.state === "in_progress") return "Ready to start";
  return label(workStatus(card.work.state));
}
export function pendingDependencies(
  card: BoardCard,
  cards: ReadonlyMap<string, BoardCard>,
) {
  return (card.dependencies || []).filter((id) => {
    const dependency = cards.get(id);
    return (
      !dependency ||
      !(
        dependency.stage === "done" ||
        dependency.archived_from === "done" ||
        workFinished(dependency.work.state)
      )
    );
  });
}

export function needsAttention(card: BoardCard, pending: number) {
  return (
    !!card.source?.error ||
    (teamScope(card) !== "completed" &&
      (pending > 0 ||
        !card.owner ||
        [
          "awaiting_review",
          "changes_requested",
          "failed",
          "blocked",
          "cancelled",
          "unknown",
        ].includes(card.work.state)))
  );
}

export function taskIsForMember(
  card: BoardCard,
  userId: string,
  email: string,
) {
  return (
    !!userId &&
    teamScope(card) !== "completed" &&
    (card.owner_id === userId ||
      (!card.owner_id && assigneeKey(card) === email.toLowerCase()) ||
      card.work.controller_id === userId ||
      (card.work.state === "awaiting_review" &&
        card.work.reviewer_id === userId))
  );
}

export function inTaskScope(card: BoardCard, scope: string) {
  return scope === "open"
    ? teamScope(card) !== "completed"
    : teamScope(card) === scope;
}
export function assigneeKey(card: BoardCard) {
  return card.owner?.trim().toLowerCase() || "unassigned";
}

export function activityTone(card: BoardCard) {
  const state = card.work.state;
  if (["failed", "unknown"].includes(state)) return "failed";
  if (
    [
      "awaiting_review",
      "changes_requested",
      "blocked",
      "cancel_requested",
    ].includes(state)
  )
    return "attention";
  if (workFinished(state)) return "completed";
  if (["running", "submitting", "dispatching", "queued"].includes(state))
    return "active";
  if (state === "in_progress") return "ready";
  return "planned";
}
