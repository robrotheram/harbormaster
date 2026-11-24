import type { ReactNode } from "react";
import { analysisAPI } from "./api";
import { DataTable } from "./data-table";

export function DatasetPreview({
  project,
  worker,
  dataset,
  id,
  actions,
}: {
  project: string;
  worker: string;
  dataset: string;
  id?: string;
  actions?: ReactNode;
}) {
  const api = analysisAPI(project, worker);
  return (
    <section
      id={id}
      aria-label="Dataset schema and preview"
      className="min-w-0 space-y-3"
    >
      {actions}
      <DataTable
        key={dataset}
        project={project}
        worker={worker}
        id={dataset}
        compact
        spec={{ sortable: true, filterable: true }}
        load={(params, signal) => api.preview(dataset, params, signal)}
      />
    </section>
  );
}
