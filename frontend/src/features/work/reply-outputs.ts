import { dataPath, imagePath } from "./workspace-output";
export type ReplyOutput = { name: string; path?: string; source?: string };

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
    .replace(/```[\s\S]*?```/g, "");
  for (const match of prose.matchAll(
    /!?\[[^\]]*\]\(<?([^\s)>]+)>?(?:\s+"[^"]*")?\)/g,
  )) {
    const path = dataPath(match[1]) || imagePath(match[1]);
    if (path && !seen.has(path)) {
      seen.add(path);
      outputs.push({ name: path, path });
    }
  }
  return outputs;
}
