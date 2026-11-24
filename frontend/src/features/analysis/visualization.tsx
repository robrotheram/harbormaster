import { cn } from "@/lib/utils";
import { lazy, Suspense, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { privateContentOptions } from "@/shared/private-content";
import { analysisAPI, type ArtifactSpec } from "./api";
import { DataTable } from "./data-table";
import { DataError, DataNotice } from "./data-state";
import { EditVisualizationDetails } from "./edit-visualization-details";
import { useVisualizationDetails } from "./use-visualization-details";
import { useVisualizationSpec } from "./use-visualization-spec";

const GeoPoints = lazy(() => import("./geo-points"));
const AnalysisChart = lazy(() =>
  import("./analysis-chart").then(({ AnalysisChart }) => ({
    default: AnalysisChart,
  })),
);

export function Visualization({
  project,
  worker,
  id,
  canEditDetails = false,
  fallbackTitle,
  actions,
  className,
}: {
  project: string;
  worker: string;
  id: string;
  canEditDetails?: boolean;
  fallbackTitle?: string;
  actions?: ReactNode;
  className?: string;
}) {
  const api = analysisAPI(project, worker);
  const query = useVisualizationSpec(project, worker, id);
  const personal = useVisualizationDetails(project);
  const custom = personal.data?.titles?.find(
    (item) => item.worker_id === worker && item.artifact_id === id,
  );
  const spec = query.data;
  const title = custom?.title ?? fallbackTitle ?? spec?.title;
  const description = custom?.description ?? spec?.description ?? "";
  let content: ReactNode;
  if (!worker) {
    content = <p role="status">The originating worker is unavailable.</p>;
  } else if (query.isError) {
    content = (
      <DataError error={query.error} retry={() => void query.refetch()} />
    );
  } else if (query.isPending) {
    content = (
      <p role="status" className="text-sm">
        Loading visualization…
      </p>
    );
  } else if (!spec) {
    content = (
      <p role="status">
        The worker returned an invalid visualization specification.
      </p>
    );
  } else if (spec.type === "table") {
    content = (
      <DataTable
        key={`${worker}/${id}`}
        project={project}
        worker={worker}
        id={id}
        spec={spec.visualization}
        load={(params, signal) => api.data(id, params, signal)}
      />
    );
  } else {
    content = (
      <VisualData
        spec={spec}
        title={title ?? spec.title}
        worker={worker}
        project={project}
      />
    );
  }
  if (!title) return content;
  return (
    <section
      aria-label={title}
      className={cn(
        "my-3 min-w-0 space-y-3 rounded-md border bg-card p-3 sm:p-4",
        className,
      )}
    >
      <header className="flex items-start justify-between gap-3">
        <div className="min-w-0 [overflow-wrap:anywhere]">
          <div className="flex items-start gap-1">
            <h3 className="min-w-0 self-center font-semibold">{title}</h3>
            {canEditDetails && spec && (
              <EditVisualizationDetails
                project={project}
                worker={worker}
                id={id}
                title={title}
                description={description}
                disabled={personal.isPending}
              />
            )}
          </div>
          {description && (
            <p className="whitespace-pre-wrap text-sm text-muted-foreground">
              {description}
            </p>
          )}
        </div>
        {actions}
      </header>
      {content}
    </section>
  );
}

function VisualData({
  spec,
  title,
  worker,
  project,
}: {
  spec: Exclude<ArtifactSpec, { type: "table" }>;
  title: string;
  worker: string;
  project: string;
}) {
  const query = useQuery({
    ...privateContentOptions,
    queryKey: [
      "project",
      project,
      "analysis-visual-data",
      worker,
      spec.artifact_id,
    ],
    queryFn: ({ signal }) =>
      analysisAPI(project, worker).data(spec.artifact_id, {}, signal),
    retry: false,
  });
  if (query.isError)
    return <DataError error={query.error} retry={() => void query.refetch()} />;
  if (!query.data)
    return (
      <p role="status" className="text-sm">
        Loading visualization data…
      </p>
    );
  return (
    <>
      <DataNotice data={query.data} />
      <Suspense
        fallback={
          <p role="status" className="text-sm">
            Loading renderer…
          </p>
        }
      >
        {spec.type === "chart" ? (
          <AnalysisChart
            spec={spec.visualization}
            page={query.data}
            title={title}
          />
        ) : (
          <GeoPoints spec={spec.visualization} page={query.data} />
        )}
      </Suspense>
    </>
  );
}
