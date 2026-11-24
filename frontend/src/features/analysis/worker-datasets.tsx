import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { bytes } from "@/shared/format";
import { privateContentOptions } from "@/shared/private-content";
import type { Workspace } from "@/shared/types";
import { analysisAPI } from "./api";
import { DataError } from "./data-state";
import { DataTable } from "./data-table";

export function WorkerDatasets({
  worker,
  project,
}: {
  worker: Workspace;
  project: string;
}) {
  const api = analysisAPI(project, worker.id);
  const client = useQueryClient();
  const key = ["project", project, "analysis-datasets", worker.id];
  const data = useQuery({
    ...privateContentOptions,
    queryKey: key,
    queryFn: ({ signal }) => api.list(signal),
    enabled: worker.ready,
    retry: false,
  });
  const [selected, setSelected] = useState("");
  const schema = useQuery({
    ...privateContentOptions,
    queryKey: [...key, "schema", selected],
    queryFn: ({ signal }) => api.schema(selected, signal),
    enabled: worker.ready && !!selected,
    retry: false,
  });
  const upload = useRef<AbortController>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const [status, setStatus] = useState("");
  useEffect(() => () => upload.current?.abort(), []);
  return (
    <section
      aria-label="Worker datasets"
      className="min-w-0 space-y-4 rounded-md border bg-card p-4 sm:p-5"
    >
      {worker.ready ? (
        <>
          <form
            className="flex flex-wrap items-end gap-2"
            onSubmit={async (event) => {
              event.preventDefault();
              const file = fileInput.current?.files?.[0];
              if (!file || upload.current) return;
              const abort = new AbortController();
              upload.current = abort;
              setBusy(true);
              setStatus("Uploading and inspecting dataset…");
              setError(undefined);
              try {
                const saved = await api.upload(file, abort.signal);
                setSelected(saved.dataset_id);
                if (fileInput.current) fileInput.current.value = "";
                setStatus(`Uploaded ${saved.name}.`);
              } catch (err) {
                if (!abort.signal.aborted) setError(err);
                setStatus(abort.signal.aborted ? "Upload canceled." : "");
              } finally {
                upload.current = null;
                setBusy(false);
                void client.invalidateQueries({ queryKey: key });
              }
            }}
          >
            <label className="grid gap-1 text-sm">
              CSV or Parquet file
              <Input
                ref={fileInput}
                required
                type="file"
                accept=".csv,.parquet"
                disabled={busy}
              />
            </label>
            <Button disabled={busy}>Upload dataset</Button>
            {busy && (
              <Button
                type="button"
                variant="outline"
                onClick={() => upload.current?.abort()}
              >
                Cancel upload
              </Button>
            )}
          </form>
          <p className="text-sm text-muted-foreground">
            Files up to 1 GiB stay on this worker.
          </p>
          {status && (
            <p role="status" className="text-sm">
              {status}
            </p>
          )}
          {!!error && <DataError error={error} />}
          {data.isError ? (
            <DataError error={data.error} retry={() => void data.refetch()} />
          ) : !data.data ? (
            <p role="status">Loading datasets…</p>
          ) : data.data.items.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              Upload a file to explore its schema and rows.
            </p>
          ) : (
            <ul className="divide-y rounded-md border">
              {data.data.items.map((file) => (
                <li key={file.dataset_id}>
                  <button
                    type="button"
                    aria-pressed={selected === file.dataset_id}
                    onClick={() => setSelected(file.dataset_id)}
                    className="grid w-full gap-1 p-3 text-left text-sm hover:bg-muted aria-pressed:bg-muted"
                  >
                    <span className="font-medium [overflow-wrap:anywhere]">
                      {file.name}
                    </span>
                    <span className="text-xs text-muted-foreground">
                      {file.format} · {bytes(file.size_bytes)} ·{" "}
                      {file.dataset_id}
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
          {selected && (
            <section
              aria-label="Dataset schema and preview"
              className="space-y-3"
            >
              {schema.isError ? (
                <DataError
                  error={schema.error}
                  retry={() => void schema.refetch()}
                />
              ) : !schema.data ? (
                <p role="status">Loading schema…</p>
              ) : (
                <>
                  <h2 className="font-medium">{schema.data.name}</h2>
                  <p className="text-xs text-muted-foreground [overflow-wrap:anywhere]">
                    SHA-256 {schema.data.sha256}
                  </p>
                  <DataTable
                    key={selected}
                    project={project}
                    worker={worker.id}
                    id={selected}
                    spec={{ sortable: true, filterable: true }}
                    load={(params, signal) =>
                      api.preview(selected, params, signal)
                    }
                  />
                </>
              )}
            </section>
          )}
        </>
      ) : (
        <p className="text-sm text-muted-foreground">
          Start the worker to open its datasets.
        </p>
      )}
    </section>
  );
}
