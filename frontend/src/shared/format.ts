export function date(value?: string | null) {
  if (!value) return "Not yet";
  const parsed = new Date(value);
  return Number.isNaN(parsed.valueOf())
    ? "Unknown"
    : new Intl.DateTimeFormat(undefined, {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(parsed);
}
export function bytes(value?: number | null) {
  if (value == null) return "Unknown";
  if (value < 1024) return value + " B";
  return value < 1024 * 1024
    ? (value / 1024).toFixed(0) + " KiB"
    : (value / (1024 * 1024)).toFixed(0) + " MiB";
}
export function label(value: string) {
  return value
    .replaceAll("_", " ")
    .toLowerCase()
    .replace(/^./, (c) => c.toUpperCase());
}
export function stateTone(value: string) {
  const state = value.toLowerCase();
  if (
    ["ready", "succeeded", "completed", "accepted", "healthy", "ok"].includes(
      state,
    )
  )
    return "good";
  if (
    [
      "error",
      "unknown",
      "failed",
      "blocked",
      "rejected",
      "cancel_requested",
      "unavailable",
      "degraded",
      "awaiting_review",
      "review",
      "review_required",
      "pending_review",
    ].includes(state)
  )
    return "warn";
  if (["busy", "starting", "running", "queued", "submitting"].includes(state))
    return "active";
  return "quiet";
}
