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
export function teamAttention(card: BoardCard) {
  if (card.source?.error) return "Resolve import sync failure";
  if (card.work.state === "awaiting_review")
    return "Reviewer needed · Review latest output";
  if (
    ["changes_requested", "failed", "blocked", "cancelled", "unknown"].includes(
      card.work.state,
    )
  )
    return `${card.owner || "Owner needed"} · Inspect run`;
  if (card.dependencies?.length)
    return `Dependencies: ${card.dependencies.join(", ")}`;
  if (!card.owner && teamScope(card) !== "completed") return "Assign an owner";
  return "—";
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
