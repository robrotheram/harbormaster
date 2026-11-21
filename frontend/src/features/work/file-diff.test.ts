import { describe, expect, it } from "vitest";
import { diffLines, filePatches, wholeFileFromPatch } from "./file-diff";

describe("recorded Git diffs", () => {
  it("indexes spaces, quoted UTF-8 paths and two blocks for one type change", () => {
    const quoted = String.raw`diff --git "a/src/\303\251\".ts" "b/src/\303\251\".ts"`;
    const patch = [
      "diff --git a/src/score model.ts b/src/score model.ts",
      "@@ -1 +1 @@",
      "-old",
      "+new",
      quoted,
      "deleted file mode 120000",
      "@@ -1 +0,0 @@",
      "-target",
      quoted,
      "new file mode 100644",
      "@@ -0,0 +1 @@",
      "+text",
    ].join("\n");
    const files = filePatches(patch, [
      "src/score model.ts",
      'src/é".ts',
      "unavailable",
    ]);
    expect(files.get("src/score model.ts")).toBe("@@ -1 +1 @@\n-old\n+new\n");
    expect(files.get('src/é".ts')).toContain("-target");
    expect(files.get('src/é".ts')).toContain("+text");
    expect(files.has("unavailable")).toBe(false);
  });

  it("distinguishes patch metadata from real changes and tracks both sides", () => {
    expect(
      diffLines(
        [
          "--- a/source",
          "+++ b/source",
          "@@ -5,3 +7,3 @@",
          " stay",
          "-old",
          "+new",
          " same",
          "@@ -20 +21,0 @@",
          "---this is a removed line",
          "\\ No newline at end of file",
        ].join("\n"),
      ),
    ).toEqual([
      { kind: "meta", text: "--- a/source" },
      { kind: "meta", text: "+++ b/source" },
      { kind: "hunk", text: "@@ -5,3 +7,3 @@" },
      { kind: "context", text: " stay", before: 5, after: 7 },
      { kind: "removed", text: "-old", before: 6 },
      { kind: "added", text: "+new", after: 8 },
      { kind: "context", text: " same", before: 7, after: 9 },
      { kind: "hunk", text: "@@ -20 +21,0 @@" },
      { kind: "removed", text: "---this is a removed line", before: 20 },
      { kind: "meta", text: "\\ No newline at end of file" },
    ]);
  });
});

describe("file contents from legacy patches", () => {
  it("recovers a complete text addition and preserves a missing final newline", () => {
    const patch = [
      "new file mode 100644",
      "index 0000000..a18bb89",
      "--- /dev/null",
      "+++ b/script.ts",
      "@@ -0,0 +1,2 @@",
      "+line 1",
      "+line 2",
      "\\ No newline at end of file",
      "",
    ].join("\n");
    expect(wholeFileFromPatch(patch, "A")).toBe("line 1\nline 2");
    expect(wholeFileFromPatch(patch, "M")).toBeUndefined();
    expect(wholeFileFromPatch(patch.split("+line 2")[0], "A")).toBeUndefined();
  });

  it("recovers deleted text and skips binary changes", () => {
    expect(
      wholeFileFromPatch(
        "deleted file mode 100644\n@@ -1 +0,0 @@\n-line\n",
        "D",
      ),
    ).toBe("line\n");
    expect(
      wholeFileFromPatch("new file mode 100644\nBinary files differ\n", "A"),
    ).toBeUndefined();
  });
});
