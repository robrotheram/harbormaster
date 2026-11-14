import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Modal } from "@/shared/components";
import { privateContentOptions } from "@/shared/private-content";
import { evidenceURL, type Artifact } from "./api";
import { ChartContent } from "./chart-content";
import { readOutput } from "./read-output";

export function WorkVisuals({
  project,
  artifacts,
  initialEditing = false,
  onDismiss,
  onAsk,
}: {
  project: string;
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
  const [edit, setEdit] = useState(initialEditing);
  const chosen =
    files.find((a) => a.content_url === file) ||
    (!pasted ? files[0] : undefined);
  const url = evidenceURL(chosen?.content_url);
  const query = useQuery({
    ...privateContentOptions,
    queryKey: ["project", project, "visual-data", url],
    queryFn: ({ signal }) => readOutput(url!, signal),
    enabled: !!url?.startsWith("/ui-api/"),
  });
  const source = pasted || query.data || "";
  return (
    <div className="visual-data-pane">
      {!!source && (
        <div className="visual-toolbar">
          <select
            aria-label="Visual data source"
            value={pasted ? "pasted" : chosen?.content_url || ""}
            onChange={(e) => {
              setFile(e.target.value);
              setPasted("");
            }}
          >
            {pasted && <option value="pasted">Pasted data</option>}
            {!files.length && !pasted && <option value="">Data source</option>}
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
          title={chosen?.source_path || "Explore data"}
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
