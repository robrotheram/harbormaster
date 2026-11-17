import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { ErrorNotice } from "@/shared/components";
import { privateContentOptions } from "@/shared/private-content";
import { FileText, Download } from "lucide-react";
import { WorkspaceImage } from "./message-content";
import { imagePath } from "./workspace-output";
import { readWorkspaceData } from "./read-output";
import type { Attempt } from "./api";
import type { ReplyOutput } from "./reply-outputs";

export function LinkedOutputs({
  project,
  attempt,
  sources,
}: {
  project: string;
  attempt: Attempt;
  sources: ReplyOutput[];
}) {
  const paths = sources.filter(
    (s) =>
      s.path &&
      !attempt.artifacts?.some((a) => (a.source_path || a.name) === s.path),
  );
  const [selected, setSelected] = useState("");
  const [download, setDownload] = useState("");
  const query = useQuery({
    ...privateContentOptions,
    queryKey: ["project", project, "linked-output", attempt.id, selected],
    queryFn: ({ signal }) =>
      readWorkspaceData(project, attempt, selected, signal),
    enabled: !!selected && !imagePath(selected),
  });
  useEffect(() => {
    if (query.data === undefined) {
      setDownload("");
      return;
    }
    const url = URL.createObjectURL(
      new Blob([query.data], { type: "text/plain;charset=utf-8" }),
    );
    setDownload(url);
    return () => URL.revokeObjectURL(url);
  }, [query.data]);
  if (!paths.length) return null;
  return (
    <section aria-label="Files linked in reply" className="space-y-3">
      <h3>From this reply</h3>
      <ul className="conversation-files">
        {paths.map(({ path }) => (
          <li key={path}>
            <FileText size={16} />
            <Button variant="link" onClick={() => setSelected(path!)}>
              {path}
            </Button>
          </li>
        ))}
      </ul>
      {selected && (
        <section
          className="conversation-output-preview"
          aria-label="Linked file preview"
        >
          <div>
            <strong>{selected}</strong>
            <Button variant="ghost" size="sm" onClick={() => setSelected("")}>
              Close preview
            </Button>
          </div>
          {imagePath(selected) ? (
            <WorkspaceImage
              src={selected}
              alt={selected}
              project={project}
              attempt={attempt}
            />
          ) : (
            <>
              {query.isPending && <p role="status">Loading file…</p>}
              {query.isError && <ErrorNotice error={query.error} />}
              {query.data !== undefined && <pre>{query.data}</pre>}
              {download && query.data !== undefined && !query.isError && (
                <a href={download} download={selected.split("/").at(-1)}>
                  <Download size={14} /> Download
                </a>
              )}
            </>
          )}
        </section>
      )}
    </section>
  );
}
