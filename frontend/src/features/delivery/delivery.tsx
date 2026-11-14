import type { Project } from "@/shared/types";
import { IssueBoard } from "./issue-board";
import { Reviews } from "./reviews";

export function Delivery({
  project,
  review = false,
}: {
  project: Project;
  review?: boolean;
}) {
  return review ? (
    <Reviews key={project.id} project={project} />
  ) : (
    <IssueBoard project={project} />
  );
}
