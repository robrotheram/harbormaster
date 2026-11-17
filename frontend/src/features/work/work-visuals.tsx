import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Modal } from "@/shared/components";
import { privateContentOptions } from "@/shared/private-content";
import { evidenceURL, type Artifact } from "./api";
import { ChartContent } from "./chart-content";
import { readOutput, readWorkspaceData } from "./read-output";
import type { ReplyOutput } from "./reply-outputs";
import type { Attempt } from "./api";
import { dataPath } from "./workspace-output";

export function WorkVisuals({
  project,
  artifacts,
  initialEditing = false,
  onDismiss,
  onAsk,
  sources = [],
  attempt,
  openRequest = 0,
}: {
  project: string;
  sources?: ReplyOutput[];
  attempt?: Attempt;
  openRequest?: number;
  artifacts: Artifact[];
  initialEditing?: boolean;
  onDismiss?: () => void;
  onAsk?: (message: string) => void;
}) {
  const files = artifacts.filter((a) =>
    /\.(csv|json)$/i.test(a.source_path || a.name || ""),
  );
  const [file, setFile] = useState("");
  const [pasted, setPasted] = useState("");
  const [draft, setDraft] = useState("");
  const linked = sources.filter(
    (s) => s.source || (s.path && dataPath(s.path)),
  );
  const [edit, setEdit] = useState(
    initialEditing && !files.length && !linked.length,
  );
  const root = useRef<HTMLDivElement>(null);
  const selectedReply =
    linked.find((s) => s.name === file) ||
    (!file && !files.length && !pasted ? linked[0] : undefined);
  const chosen =
    files.find((a) => a.content_url === file) ||
    (!pasted && !selectedReply ? files[0] : undefined);
  const url = evidenceURL(chosen?.content_url);
  const query = useQuery({
    ...privateContentOptions,
    queryKey: [
      "project",
      project,
      "visual-data",
      url,
      attempt?.id,
      selectedReply?.path,
    ],
    queryFn: ({ signal }) =>
      selectedReply?.path && attempt
        ? readWorkspaceData(project, attempt, selectedReply.path, signal)
        : readOutput(url!, signal),
    enabled:
      !!url?.startsWith("/ui-api/") || (!!selectedReply?.path && !!attempt),
  });
  const source = pasted || selectedReply?.source || query.data || "";
  useEffect(() => {
    if (!openRequest) return;
    root.current?.scrollIntoView({ block: "nearest" });
    if (!source && !chosen && !selectedReply) setEdit(true);
  }, [openRequest, source, !!chosen, !!selectedReply]);
  return (
    <div className="visual-data-pane" ref={root}>
      {!!source && (
        <div className="visual-toolbar">
          <select
            aria-label="Visual data source"
            value={
              pasted
                ? "pasted"
                : selectedReply?.name || chosen?.content_url || ""
            }
            onChange={(e) => {
              setFile(e.target.value);
              setPasted("");
            }}
          >
            {pasted && <option value="pasted">Pasted data</option>}
            {!files.length && !pasted && <option value="">Data source</option>}
            {linked.map((s) => (
              <option key={s.name} value={s.name}>
                {s.name}
              </option>
            ))}
            {files.map((a) => (
              <option key={a.content_url} value={a.content_url}>
                {a.source_path || a.name}
              </option>
            ))}
          </select>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              setDraft(pasted);
              setEdit(true);
            }}
          >
            Paste data
          </Button>
        </div>
      )}
      {query.isError && <p role="alert">Could not read this data file.</p>}
      {query.isFetching && !source && <p role="status">Loading chart…</p>}
      {!!source && (
        <ChartContent
          source={source}
          title={selectedReply?.name || chosen?.source_path || "Explore data"}
          onAsk={onAsk}
        />
      )}
      {edit && (
        <Modal
          title="Visualize data"
          onClose={() => {
            setEdit(false);
            if (!source) onDismiss?.();
          }}
        >
          <p>
            Paste CSV with column headers, or a JSON array of objects. Up to
            2,000 rows.
          </p>
          <Textarea
            aria-label="Chart data"
            rows={10}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            placeholder={"experiment,score\nbaseline,72\nrevision,86"}
          />
          <Button
            disabled={!draft.trim()}
            onClick={() => {
              setPasted(draft);
              setEdit(false);
            }}
          >
            Draw chart
          </Button>
        </Modal>
      )}
    </div>
  );
}
