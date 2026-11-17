import { queryOptions } from "@tanstack/react-query";
import { privateContentOptions } from "@/shared/private-content";
import { workApi, workKeys, type WorkItem } from "./api";
import { userMessage } from "./conversation-api";

export function instructionOptions(project: string, item: WorkItem) {
  return queryOptions({
    ...privateContentOptions,
    queryKey: [...workKeys.detail(project, item.id), "instruction"],
    queryFn: ({ signal }) => workApi.instruction(project, item.id, signal),
    staleTime: Infinity,
    retry: false,
  });
}

export function conversationTitle(
  item: Pick<WorkItem, "id" | "specification_path">,
  instruction?: string,
) {
  const firstLine =
    instruction &&
    userMessage(instruction, {
      parent_attempt_id: instruction.startsWith(
        "Continue the same work in this workspace.",
      )
        ? "followup"
        : undefined,
    })
      .split(/\r?\n/)
      .map((line) => line.trim())
      .find(Boolean);
  const text = firstLine
    ?.replace(/^#{1,6}\s+/, "")
    .replace(/\s+/g, " ")
    .trim();
  if (text) {
    const chars = Array.from(text);
    return chars.length > 120 ? chars.slice(0, 117).join("") + "…" : text;
  }
  if (!item.specification_path.startsWith("work-requests/")) {
    return item.specification_path.split("/").at(-1) || "Untitled conversation";
  }
  return "Untitled conversation";
}
