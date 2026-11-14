export const stages = [
  { id: "all", label: "All work" },
  { id: "queued", label: "Planned" },
  { id: "running", label: "In progress" },
  { id: "review", label: "Needs review" },
  { id: "done", label: "Done" },
  { id: "blocked", label: "Needs attention" },
] as const;
export type WorkStage = (typeof stages)[number]["id"];
export function workStage(state: string): WorkStage {
  const s = state.toLowerCase();
  if (["draft", "planned", "queued", "pending"].includes(s)) return "queued";
  if (["in_progress", "running", "submitting"].includes(s)) return "running";
  if (
    ["awaiting_review", "review", "review_required", "pending_review"].includes(
      s,
    )
  )
    return "review";
  if (s === "completed" || s === "accepted" || s === "closed") return "done";
  return "blocked";
}

// Stage groups organise work; they must not replace its actual review outcome.
export function workStatus(state: string): string {
  return state === "completed" ? "accepted" : state;
}
export function workFinished(state: string): boolean {
  return workStage(state) === "done";
}
