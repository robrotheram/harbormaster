import type { Artifact } from "@/features/work/api";
import type { HistoryRecord } from "@/features/history/history-api";

export type ReviewMode = "preview" | "code" | "visuals" | "reports";
export function artifactMode(
  artifact: Artifact,
): Exclude<ReviewMode, "preview"> {
  const path = (artifact.source_path || artifact.name || "").toLowerCase();
  if (/\.(csv|json|png|jpe?g|gif|webp|svg)$/.test(path)) return "visuals";
  if (/\.(md|markdown|txt|pdf|html?)$/.test(path)) return "reports";
  return "code";
}
export function recordedEdits(records: HistoryRecord[]) {
  return records.flatMap((r) => {
    if (r.redacted || !r.content) return [];
    if (r.tool === "file_change" && r.kind === "tool.result")
      return [
        {
          id: r.id,
          title: "Recorded file change",
          content: readableChange(r.content),
          truncated: r.truncated,
        },
      ];
    if (
      r.kind !== "tool.call" ||
      !/apply_patch|edit_file|replace_in_file|str_replace|write_file/i.test(
        r.tool || "",
      )
    )
      return [];
    let content = r.content;
    try {
      const value = JSON.parse(content);
      if (typeof value === "string") content = value;
      else if (typeof value.patch === "string") content = value.patch;
      else if (typeof value.input === "string") content = value.input;
      else content = JSON.stringify(value, null, 2);
    } catch {
      /* Plain patches are already readable. */
    }
    return [
      {
        id: r.id,
        title: `Recorded edit request · ${r.tool}`,
        content,
        truncated: r.truncated,
      },
    ];
  });
}
export function visualSections(content: string) {
  // Include only delivered chart blocks and image/data references, not prose.
  return [
    ...content.matchAll(
      /```workbench-chart\s*\n[\s\S]*?```|!?\[[^\]\n]*\]\([^\)\n]+\.(?:json|csv|png|jpe?g|gif|webp|svg)(?:\s+"[^"]*")?\)/gi,
    ),
  ]
    .map((match) => match[0])
    .join("\n\n");
}

function readableChange(content: string) {
  try {
    const value = JSON.parse(content);
    if (Array.isArray(value))
      return value
        .map((change) => {
          if (!change || typeof change !== "object")
            return JSON.stringify(change);
          return [
            change.path || change.file || "File",
            change.kind || change.type,
            change.diff || change.patch,
          ]
            .filter(Boolean)
            .join("\n");
        })
        .join("\n\n");
    return JSON.stringify(value, null, 2);
  } catch {
    return content;
  }
}
