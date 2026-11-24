import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Download, MessageSquare } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ErrorNotice } from "@/shared/components";
import { privateContentOptions } from "@/shared/private-content";
import { evidenceURL, type Artifact, type Attempt } from "./api";
import { FileCode, type LineReference } from "./file-code";
import { wholeFileFromPatch } from "./file-diff";
import { MessageContent, WorkspaceImage } from "./message-content";
import { readOutput, readWorkspaceData } from "./read-output";

export type ReviewFile = {
  path: string;
  status?: string;
  source?: string;
  artifact?: Artifact;
  linked?: boolean;
};

export function FileInspector({
  file,
  patch,
  truncated,
  project,
  attempt,
  onAsk,
  onAskAboutContent,
}: {
  file: ReviewFile;
  patch?: string;
  truncated: boolean;
  project: string;
  attempt: Attempt;
  onAsk?: (reference?: LineReference) => void;
  onAskAboutContent?: (message: string) => void;
}) {
  const image = /\.(?:png|jpe?g|gif|webp|avif|svg)$/i.test(file.path);
  const markdown = /\.(?:md|markdown)$/i.test(file.path);
  const binary = /\.(?:pdf|zip|gz|exe|wasm|bin)$/i.test(file.path);
  const patchFile = /\.(?:diff|patch)$/i.test(file.path);
  const retainedURL = evidenceURL(file.artifact?.content_url);
  const [mode, setMode] = useState<"diff" | "file" | "rendered">("file");
  const showRecordedDiff = mode === "diff" && !!file.status;
  const snapshot = useMemo(() => {
    if (mode === "diff" || !file.status) return undefined;
    if (file.source !== undefined) {
      try {
        return new TextDecoder("utf-8", {
          fatal: true,
          ignoreBOM: true,
        }).decode(Uint8Array.from(atob(file.source), (c) => c.charCodeAt(0)));
      } catch {
        return undefined;
      }
    }
    // Legacy captures contain the full text for ordinary additions and deletions.
    return !truncated && !file.artifact && !file.linked && patch
      ? wholeFileFromPatch(patch, file.status)
      : undefined;
  }, [
    mode,
    file.status,
    file.source,
    file.artifact,
    file.linked,
    patch,
    truncated,
  ]);
  const previewImage =
    image &&
    snapshot === undefined &&
    !showRecordedDiff &&
    (!!file.artifact || !!file.linked);
  const linked = !file.artifact && !!file.linked;
  const textEnabled =
    !showRecordedDiff &&
    snapshot === undefined &&
    !previewImage &&
    !binary &&
    (linked ||
      (!!retainedURL?.startsWith("/ui-api/") &&
        (file.artifact?.size_bytes ?? 0) <= 262144));
  const text = useQuery({
    ...privateContentOptions,
    queryKey: [
      "project",
      project,
      "file-content",
      attempt.id,
      file.path,
      retainedURL,
    ],
    queryFn: ({ signal }) =>
      retainedURL
        ? readOutput(retainedURL, signal)
        : readWorkspaceData(project, attempt, file.path, signal),
    enabled: textEnabled,
    retry: false,
  });
  const content =
    snapshot ?? (textEnabled && !text.isError ? text.data : undefined);
  const [download, setDownload] = useState("");
  const downloadable = snapshot ?? (linked ? content : undefined);
  useEffect(() => {
    if (downloadable === undefined) {
      setDownload("");
      return;
    }
    const url = URL.createObjectURL(
      new Blob([downloadable], { type: "text/plain;charset=utf-8" }),
    );
    setDownload(url);
    return () => URL.revokeObjectURL(url);
  }, [downloadable]);
  const downloadURL =
    (downloadable !== undefined ? download : "") || retainedURL;
  const codeAsk = onAsk && (
    <Button type="button" size="sm" variant="outline" onClick={() => onAsk()}>
      <MessageSquare /> Explain this file
    </Button>
  );
  return (
    <>
      <header className="file-inspector-heading">
        <h3 title={file.path}>{file.path}</h3>
        {downloadURL && (
          <Button asChild variant="ghost" size="sm">
            <a href={downloadURL} download={file.path.split("/").at(-1)}>
              <Download /> Download
            </a>
          </Button>
        )}
        <div className="file-mode" role="group" aria-label="File view">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            aria-pressed={mode === "file"}
            onClick={() => setMode("file")}
          >
            File
          </Button>
          {(file.status || patchFile) && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              aria-pressed={mode === "diff"}
              onClick={() => setMode("diff")}
            >
              Diff
            </Button>
          )}
          {markdown && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              aria-pressed={mode === "rendered"}
              onClick={() => setMode("rendered")}
            >
              Rendered
            </Button>
          )}
        </div>
      </header>
      {file.status === "D" && snapshot !== undefined && (
        <p className="file-viewer-notice">File contents before deletion</p>
      )}
      {showRecordedDiff && truncated && (
        <p className="file-viewer-notice" role="status">
          The captured diff is incomplete.
        </p>
      )}
      {showRecordedDiff && patch ? (
        <FileCode
          key="diff"
          path={file.path}
          content={patch}
          patch
          onAsk={onAsk}
        />
      ) : showRecordedDiff ? (
        <div className="file-inspector-body">
          <p>No text diff was captured for this file.</p>
          {codeAsk}
        </div>
      ) : previewImage ? (
        <div className="file-inspector-body">
          {codeAsk}
          <WorkspaceImage
            src={file.path}
            alt={file.path}
            project={project}
            attempt={attempt}
          />
        </div>
      ) : textEnabled && text.isError ? (
        <div className="file-inspector-body">
          <ErrorNotice
            error={text.error}
            onRetry={() => void text.refetch()}
            retrying={text.isFetching}
          />
          {codeAsk}
        </div>
      ) : textEnabled && text.isPending ? (
        <p className="file-viewer-notice" role="status">
          Loading file…
        </p>
      ) : content === undefined ? (
        <div className="file-inspector-body">
          <p>
            {downloadURL
              ? "Download this file to inspect it."
              : file.status
                ? "Full contents are unavailable for this revision. Open Diff to review the captured changes."
                : "File contents are unavailable."}
          </p>
          {codeAsk}
        </div>
      ) : content.includes("\x00") ? (
        <div className="file-inspector-body">
          <p>This is a binary file. Download it to inspect it.</p>
          {codeAsk}
        </div>
      ) : mode === "rendered" ? (
        <div className="file-inspector-body">
          {codeAsk}
          <MessageContent
            content={content}
            onAsk={onAskAboutContent}
            embedData
            project={project}
            attempt={attempt}
          />
        </div>
      ) : (
        <FileCode
          key={mode}
          content={content}
          path={file.path}
          patch={mode === "diff"}
          lineFeedback={mode !== "diff"}
          sourceSide={
            file.status === "D" && snapshot !== undefined ? "before" : "after"
          }
          onAsk={onAsk}
        />
      )}
    </>
  );
}
