import { useEffect, useMemo, useState } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { Download } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ErrorNotice } from "@/shared/components";
import { privateContentOptions } from "@/shared/private-content";
import { workApi } from "./api";
import { FileCode } from "./file-code";

const imageTypes = new Map([
  ["png", "image/png"],
  ["jpg", "image/jpeg"],
  ["jpeg", "image/jpeg"],
  ["gif", "image/gif"],
  ["webp", "image/webp"],
  ["avif", "image/avif"],
  ["svg", "image/svg+xml"],
]);

export function WorkspaceFile({
  project,
  work,
  workspace,
  path,
  revision,
  changed,
  patch,
  truncated,
}: {
  project: string;
  work: string;
  workspace: string;
  path: string;
  revision?: string;
  changed: boolean;
  patch?: string;
  truncated: boolean;
}) {
  const file = useQuery({
    ...privateContentOptions,
    queryKey: [
      "project",
      project,
      "workspace-file",
      work,
      workspace,
      path,
      revision,
    ],
    queryFn: ({ signal }) => workApi.file(project, work, path, signal),
    placeholderData: keepPreviousData,
  });
  const [mode, setMode] = useState<"file" | "diff">("file");
  const showDiff = changed && mode === "diff";
  const [url, setURL] = useState("");
  const type = imageTypes.get(path.split(".").at(-1)?.toLowerCase() ?? "");
  const bytes = useMemo(
    () =>
      file.data &&
      Uint8Array.from(atob(file.data.content ?? ""), (value) =>
        value.charCodeAt(0),
      ),
    [file.data],
  );
  const content = useMemo(() => {
    if (!bytes || bytes.includes(0)) return undefined;
    try {
      return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
    } catch {
      return undefined;
    }
  }, [bytes]);
  useEffect(() => {
    if (!bytes) {
      setURL("");
      return;
    }
    const next = URL.createObjectURL(
      new Blob([bytes], { type: type || "application/octet-stream" }),
    );
    setURL(next);
    return () => URL.revokeObjectURL(next);
  }, [bytes, type]);
  return (
    <>
      <header className="file-inspector-heading">
        <h3 title={path}>{path}</h3>
        {url && (
          <Button asChild variant="ghost" size="sm">
            <a href={url} download={path.split("/").at(-1)}>
              <Download /> Download
            </a>
          </Button>
        )}
        {changed && (
          <div className="file-mode" role="group" aria-label="File view">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              aria-pressed={!showDiff}
              onClick={() => setMode("file")}
            >
              File
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              aria-pressed={showDiff}
              onClick={() => setMode("diff")}
            >
              Diff
            </Button>
          </div>
        )}
      </header>
      {showDiff && truncated && (
        <p className="file-viewer-notice" role="status">
          The captured diff is incomplete.
        </p>
      )}
      {showDiff ? (
        patch ? (
          <FileCode key="diff" path={path} content={patch} patch />
        ) : (
          <p className="file-viewer-notice">
            No text diff was captured for this file.
          </p>
        )
      ) : file.isError ? (
        <div className="file-inspector-body">
          <ErrorNotice
            error={file.error}
            onRetry={() => void file.refetch()}
            retrying={file.isFetching}
          />
        </div>
      ) : file.isPending ? (
        <p className="file-viewer-notice" role="status">
          Loading file…
        </p>
      ) : type && url ? (
        <div className="file-inspector-body">
          <span className="message-image">
            <img src={url} alt={path} />
          </span>
        </div>
      ) : content !== undefined ? (
        <FileCode key="file" path={path} content={content} patch={false} />
      ) : (
        <p className="file-viewer-notice">Download this file to inspect it.</p>
      )}
    </>
  );
}
