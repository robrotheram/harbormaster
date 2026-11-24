import type { WorkItem } from "./api";

export const workProfiles = [
  { kind: "software", label: "Web Development" },
  { kind: "desktop", label: "Desktop App Development" },
  { kind: "data-science", label: "Research / Data Science" },
] as const satisfies readonly { kind: WorkItem["kind"]; label: string }[];

export type WorkProfile = (typeof workProfiles)[number]["kind"];

export function startsInWorkspace(kind: WorkItem["kind"]) {
  return kind === "desktop" || kind === "data-science";
}
