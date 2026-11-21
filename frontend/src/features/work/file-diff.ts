type DiffLine = {
  kind: "added" | "removed" | "context" | "hunk" | "meta";
  text: string;
  before?: number;
  after?: number;
};

function gitPath(path: string) {
  let quoted = "";
  for (const byte of new TextEncoder().encode(path)) {
    if (byte === 34) quoted += '\\"';
    else if (byte === 92) quoted += "\\\\";
    else if (byte === 9) quoted += "\\t";
    else if (byte === 7) quoted += "\\a";
    else if (byte === 8) quoted += "\\b";
    else if (byte === 11) quoted += "\\v";
    else if (byte === 12) quoted += "\\f";
    else if (byte < 32 || byte >= 127)
      quoted += "\\" + byte.toString(8).padStart(3, "0");
    else quoted += String.fromCharCode(byte);
  }
  return quoted === path ? path : `"${quoted}"`;
}

export function filePatches(diff: string, paths: string[]) {
  const headers = new Map(
    paths.map((path) => [
      `diff --git ${gitPath("a/" + path)} ${gitPath("b/" + path)}`,
      path,
    ]),
  );
  const patches = new Map<string, string>();
  // Older single-file captures may only contain a hunk.
  if (paths.length === 1 && diff && !diff.startsWith("diff --git "))
    patches.set(paths[0], diff);
  else {
    for (const block of diff.split(/(?=^diff --git )/m)) {
      const newline = block.indexOf("\n");
      if (newline === -1) continue;
      const path = headers.get(block.slice(0, newline));
      if (!path) continue;
      // Git can emit two blocks for one path when its type changed.
      const previous = patches.get(path);
      const body = block.slice(newline + 1);
      patches.set(path, previous ? previous + "\n" + body : body);
    }
  }
  return patches;
}

export function diffLines(patch: string): DiffLine[] {
  let before = 0;
  let after = 0;
  let oldRemaining = 0;
  let newRemaining = 0;
  return patch
    .replace(/\n$/, "")
    .split("\n")
    .map((text) => {
      const hunk = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/.exec(text);
      if (hunk) {
        before = Number(hunk[1]);
        oldRemaining = Number(hunk[2] ?? 1);
        after = Number(hunk[3]);
        newRemaining = Number(hunk[4] ?? 1);
        return { kind: "hunk", text };
      }
      if (text.startsWith("-") && oldRemaining > 0) {
        oldRemaining--;
        return { kind: "removed", text, before: before++ };
      }
      if (text.startsWith("+") && newRemaining > 0) {
        newRemaining--;
        return { kind: "added", text, after: after++ };
      }
      if (text.startsWith(" ") && oldRemaining > 0 && newRemaining > 0) {
        oldRemaining--;
        newRemaining--;
        return { kind: "context", text, before: before++, after: after++ };
      }
      return { kind: "meta", text };
    });
}

// Older revisions retained only patches. Git includes the whole text in a
// regular file addition or deletion, so it can be recovered from one full hunk.
export function wholeFileFromPatch(patch: string, status: string) {
  if (status !== "A" && status !== "D") return undefined;
  const added = status === "A";
  const mode = added ? /^new file mode (\d+)$/m : /^deleted file mode (\d+)$/m;
  if (!mode.exec(patch)?.[1].startsWith("100")) return undefined;
  const lines = diffLines(patch);
  const hunks = lines.filter((line) => line.kind === "hunk");
  if (!hunks.length)
    return /^index (?:0+\.\.e69de29|e69de29\.\.0+)$/m.test(patch)
      ? ""
      : undefined;
  if (hunks.length !== 1) return undefined;
  const header = (
    added ? /^@@ -0,0 \+1(?:,(\d+))? @@/ : /^@@ -1(?:,(\d+))? \+0,0 @@/
  ).exec(hunks[0].text);
  if (!header || !patch.endsWith("\n")) return undefined;
  const changes = lines.filter(
    (line) => line.kind === (added ? "added" : "removed"),
  );
  if (changes.length !== Number(header[1] ?? 1)) return undefined;
  return (
    changes.map(({ text }) => text.slice(1)).join("\n") +
    (lines.at(-1)?.text === "\\ No newline at end of file" ? "" : "\n")
  );
}
