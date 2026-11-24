import { artifactReference } from "@/features/analysis/api";
import { dataPath, imagePath } from "./workspace-output";
export type ReplyOutput = {
  name: string;
  path?: string;
  source?: string;
  label?: string;
  artifact?: string;
};

// Only explicit links and chart blocks in agent replies become output sources.
// The same path rules as inline output rendering apply; prose is never fetched.
export function replyOutputs(content: string): ReplyOutput[] {
  const outputs: ReplyOutput[] = [];
  const seen = new Set<string>();
  const prose = content
    .replace(/```workbench-chart\s*\n([\s\S]*?)```/g, (_, source: string) => {
      outputs.push({
        name: `Chart ${outputs.length + 1}`,
        source: source.trim(),
      });
      return "";
    })
    .replace(
      /```harbormaster-visualization\s*\n([\s\S]*?)```/g,
      (_, source: string) => {
        const id = artifactReference(source);
        if (id && !seen.has(id)) {
          seen.add(id);
          outputs.push({ name: id, artifact: id });
        }
        return "";
      },
    )
    .replace(/```[\s\S]*?```/g, "");
  for (const match of prose.matchAll(
    /!?\[([^\]]*)\]\(<?([^\s)>]+)>?(?:\s+"[^"]*")?\)/g,
  )) {
    const path = dataPath(match[2]) || imagePath(match[2]);
    if (path && !seen.has(path)) {
      seen.add(path);
      outputs.push({ name: path, path, label: match[1] || path });
    }
  }
  return outputs;
}
