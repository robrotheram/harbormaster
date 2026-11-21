import type { Project } from "@/shared/types";
import { IssueBoard } from "./issue-board";

export function Delivery({ project }: { project: Project }) {
  return <IssueBoard project={project} />;
}
