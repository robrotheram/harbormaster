import { useId, useState } from "react";
import { ChartNoAxesCombined, Database, ScanSearch } from "lucide-react";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/native-select";
import { DatasetPreview } from "./dataset-preview";
import type { Dataset } from "./api";
import type { useWorkerDatasets } from "./use-worker-datasets";

export function AnalysisExplorer({
  project,
  worker,
  datasets,
  dataset,
  onSelect,
  onBrowse,
  onDraft,
}: {
  project: string;
  worker: string;
  datasets: ReturnType<typeof useWorkerDatasets>;
  dataset?: Dataset;
  onSelect: (id: string) => void;
  onBrowse: () => void;
  onDraft?: (prompt?: string) => void;
}) {
  const previewId = useId();
  const [expanded, setExpanded] = useState(false);
  const items = datasets.data?.items ?? [];

  return (
    <section
      aria-labelledby="analysis-explore-heading"
      className="mx-4 mt-5 grid min-w-0 gap-3 rounded-md border bg-card px-4 py-3 sm:mx-6"
    >
      <h2 id="analysis-explore-heading" className="font-semibold">
        Explore data
      </h2>
      {datasets.isSuccess && !items.length && (
        <p className="text-sm text-muted-foreground">
          Upload a CSV or Parquet file to start exploring.
        </p>
      )}
      <div className="flex flex-wrap items-center gap-2">
        {items.length > 0 && (
          <NativeSelect
            aria-label="Dataset to explore"
            wrapperClassName="max-w-full flex-1 basis-52 sm:max-w-sm"
            value={dataset?.dataset_id ?? ""}
            onChange={(event) => onSelect(event.target.value)}
          >
            <option value="">Any dataset</option>
            {items.map((item) => (
              <option key={item.dataset_id} value={item.dataset_id}>
                {item.name}
              </option>
            ))}
          </NativeSelect>
        )}
        <Button
          type="button"
          size="lg"
          variant="outline"
          aria-expanded={dataset ? expanded : undefined}
          aria-controls={dataset && expanded ? previewId : undefined}
          onClick={() => (dataset ? setExpanded(!expanded) : onBrowse())}
        >
          <Database />{" "}
          {dataset
            ? expanded
              ? "Hide rows"
              : "Preview rows"
            : "Browse datasets"}
        </Button>
        {!items.length && datasets.isFetching && (
          <span role="status" className="text-sm text-muted-foreground">
            Loading datasets…
          </span>
        )}
        {datasets.isError && (
          <Button
            type="button"
            size="lg"
            variant="ghost"
            onClick={() => void datasets.refetch()}
          >
            Retry dataset list
          </Button>
        )}
        {onDraft && (
          <>
            <Button
              type="button"
              size="lg"
              variant="outline"
              disabled={!dataset}
              onClick={() => {
                if (dataset)
                  onDraft(
                    `Profile dataset ${dataset.dataset_id}. Identify the unit of observation, summarize the main distributions and missingness, and flag duplicates, outliers, or coverage limits that could change the analysis.`,
                  );
              }}
            >
              <ScanSearch /> Profile dataset
            </Button>
            <Button
              type="button"
              size="lg"
              variant="outline"
              onClick={() =>
                onDraft(
                  dataset &&
                    `Create a visualization from dataset ${dataset.dataset_id} that shows a pattern worth investigating. Explain the measure, grouping, and how you handled missing values.`,
                )
              }
            >
              <ChartNoAxesCombined /> New visualization
            </Button>
          </>
        )}
      </div>
      {dataset && expanded && (
        <div className="min-w-0 border-t pt-3">
          <DatasetPreview
            id={previewId}
            project={project}
            worker={worker}
            dataset={dataset.dataset_id}
          />
        </div>
      )}
    </section>
  );
}
