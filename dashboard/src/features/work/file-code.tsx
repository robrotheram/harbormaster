import { useEffect, useMemo, useRef, useState } from "react";
import {
  canUseWASM,
  parsePatchFiles,
  type SelectedLineRange,
} from "@pierre/diffs";
import { File, FileDiff } from "@pierre/diffs/react";
import { ChevronDown, ChevronUp, MessageSquare, WrapText } from "lucide-react";
import { Button } from "@/components/ui/button";

type Side = "before" | "after";
export type LineReference = { side: Side; start: number; end: number };

const displayLimit = 10000;

export function FileCode({
  content,
  patch,
  path = "file.txt",
  onAsk,
  lineFeedback = true,
  sourceSide = "after",
}: {
  content: string;
  patch: boolean;
  path?: string;
  onAsk?: (reference?: LineReference) => void;
  lineFeedback?: boolean;
  sourceSide?: Side;
}) {
  const lines = useMemo(() => content.split("\n"), [content]);
  const displayed = lines.slice(0, displayLimit).join("\n");
  const file = useMemo(
    () => ({ name: path, contents: displayed }),
    [path, displayed],
  );
  const diff = useMemo(() => {
    if (!patch || lines.length > displayLimit) return;
    try {
      // Recorded diffs can omit the filenames while retaining unified hunks.
      const hunk = content.search(/^@@ /m);
      if (hunk < 0) return;
      const name = path.replace(/[\r\n\t]/g, "_");
      const unified = /^--- /m.test(content.slice(0, hunk))
        ? content
        : `--- a/${name}\n+++ b/${name}\n${content.slice(hunk)}`;
      const files = parsePatchFiles(unified, { throwOnError: true }).flatMap(
        (entry) => entry.files,
      );
      return files.length === 1 && files[0].hunks.length ? files[0] : undefined;
    } catch {
      // A truncated capture remains readable as raw patch text.
      return undefined;
    }
  }, [content, lines.length, patch, path]);
  const code = useRef<HTMLDivElement>(null);
  const [hunk, setHunk] = useState(-1);
  const [wrap, setWrap] = useState(false);
  const [selected, setSelected] = useState<SelectedLineRange | null>(null);
  const canSelect = !!onAsk && lineFeedback && (!patch || !!diff);
  const [dark, setDark] = useState(() =>
    document.documentElement.classList.contains("dark"),
  );
  useEffect(() => {
    const root = document.documentElement;
    const observer = new MutationObserver(() =>
      setDark(root.classList.contains("dark")),
    );
    observer.observe(root, { attributeFilter: ["class"] });
    setDark(root.classList.contains("dark"));
    return () => observer.disconnect();
  }, []);
  const options = useMemo(
    () => ({
      disableFileHeader: true,
      themeType: dark ? ("dark" as const) : ("light" as const),
      overflow: wrap ? ("wrap" as const) : ("scroll" as const),
      preferredHighlighter: canUseWASM()
        ? ("highlights" as const)
        : ("shiki-js" as const),
      enableLineSelection: canSelect,
      onLineSelected: setSelected,
    }),
    [wrap, canSelect, dark],
  );
  const diffOptions = useMemo(
    () => ({
      ...options,
      diffStyle: "unified" as const,
      hunkSeparators: "metadata" as const,
    }),
    [options],
  );
  const hunks = diff?.hunks ?? [];
  const side =
    selected?.side === "deletions" ? "before" : diff ? "after" : sourceSide;
  const range =
    selected && (!selected.endSide || selected.endSide === selected.side)
      ? { start: selected.start, end: selected.end, side }
      : null;
  const moveHunk = (index: number) => {
    const target = hunks[index];
    const root = code.current?.querySelector("diffs-container")?.shadowRoot;
    if (!target) return;
    const line = target.additionCount
      ? target.additionStart
      : target.deletionStart;
    const row = root?.querySelector(`[data-line="${line}"]`);
    if (!row) return;
    row.scrollIntoView({ block: "nearest" });
    setHunk(index);
  };
  const setRange = (bound: "start" | "end", value: number) => {
    if (!Number.isSafeInteger(value) || value < 1) return;
    setSelected((previous) => ({
      start: value,
      end: value,
      ...(diff ? { side: "additions" as const } : {}),
      ...previous,
      [bound]: value,
    }));
  };

  return (
    <>
      <div className="file-code-tools">
        {diff && (
          <span className="file-diff-counts">
            <span className="file-additions">
              +{hunks.reduce((total, item) => total + item.additionLines, 0)}
            </span>
            <span className="file-removals">
              −{hunks.reduce((total, item) => total + item.deletionLines, 0)}
            </span>
          </span>
        )}
        {hunks.length > 1 && (
          <div
            className="file-hunk-nav"
            role="group"
            aria-label="Diff hunk navigation"
          >
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label="Previous hunk"
              disabled={hunk <= 0}
              onClick={() => moveHunk(hunk - 1)}
            >
              <ChevronUp />
            </Button>
            <span aria-live="polite">
              {hunk < 0
                ? `${hunks.length} hunks`
                : `${hunk + 1} / ${hunks.length}`}
            </span>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label="Next hunk"
              disabled={hunk >= hunks.length - 1}
              onClick={() => moveHunk(hunk + 1)}
            >
              <ChevronDown />
            </Button>
          </div>
        )}
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="Wrap lines"
          aria-pressed={wrap}
          onClick={() => setWrap(!wrap)}
        >
          <WrapText />
        </Button>
        {canSelect && (
          <details className="file-line-range">
            <summary>Select lines</summary>
            {diff && (
              <select
                aria-label="Diff side"
                value={selected?.side ?? "additions"}
                onChange={(event) => {
                  const side =
                    event.target.value === "deletions"
                      ? "deletions"
                      : "additions";
                  setSelected((previous) => ({
                    start: 1,
                    end: 1,
                    ...previous,
                    side,
                    endSide: side,
                  }));
                }}
              >
                <option value="additions">After</option>
                <option value="deletions">Before</option>
              </select>
            )}
            <input
              type="number"
              min={1}
              aria-label="First line"
              value={selected?.start ?? ""}
              onChange={(event) =>
                event.target.value
                  ? setRange("start", event.target.valueAsNumber)
                  : setSelected(null)
              }
            />
            <input
              type="number"
              min={1}
              aria-label="Last line"
              value={selected?.end ?? ""}
              onChange={(event) =>
                event.target.value
                  ? setRange("end", event.target.valueAsNumber)
                  : setSelected(null)
              }
            />
          </details>
        )}
        {onAsk && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="file-ask"
            disabled={!!selected && !range}
            onClick={() => onAsk(range ?? undefined)}
          >
            <MessageSquare />
            {!range
              ? "Explain this file"
              : range.start === range.end
                ? "Explain this line"
                : "Explain selected lines"}
          </Button>
        )}
        {selected && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => setSelected(null)}
          >
            Clear selection
          </Button>
        )}
      </div>
      {lines.length > displayLimit && (
        <p className="file-viewer-notice">
          Showing the first {displayLimit.toLocaleString()} lines.
        </p>
      )}
      <div
        ref={code}
        className="file-code"
        role="region"
        aria-label={patch ? "Git diff" : "File contents"}
        tabIndex={0}
      >
        {diff ? (
          <FileDiff
            fileDiff={diff}
            options={diffOptions}
            selectedLines={selected}
          />
        ) : (
          <File file={file} options={options} selectedLines={selected} />
        )}
      </div>
    </>
  );
}
