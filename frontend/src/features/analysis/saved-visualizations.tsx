import { useState } from "react";
import { BarChart3, Map, MessageSquare, Plus, Table2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { VisualizationReference, VisualizationRevision } from "./api";
import { DataError } from "./data-state";
import {
  useSavedVisualizations,
  useUpdatePersonalVisualization,
} from "./use-saved-visualizations";
import { Visualization } from "./visualization";

const icons = { chart: BarChart3, geo: Map, table: Table2 };

function visualizationKey({ worker_id, artifact_id }: VisualizationReference) {
  return `${worker_id}/${artifact_id}`;
}

export function SavedVisualizations({
  project,
  onCreate,
  onRevise,
}: {
  project: string;
  onCreate?: () => void;
  onRevise: (item: VisualizationRevision) => boolean;
}) {
  const list = useSavedVisualizations(project);
  const update = useUpdatePersonalVisualization(project);
  const [selection, setSelection] = useState("");
  const [unavailable, setUnavailable] = useState("");
  const items = list.data?.items ?? [];
  const selected =
    items.find((item) => visualizationKey(item) === selection) ?? items[0];
  return (
    <div className="min-w-0 px-4 py-5 sm:px-6">
      <header className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-lg font-semibold">Saved visualizations</h2>
        {onCreate && (
          <Button type="button" variant="outline" size="sm" onClick={onCreate}>
            <Plus />
            New visualization
          </Button>
        )}
      </header>
      {list.isError ? (
        <DataError error={list.error} retry={() => void list.refetch()} />
      ) : list.isPending ? (
        <p role="status" className="text-sm text-muted-foreground">
          Loading saved visualizations…
        </p>
      ) : selected ? (
        <div className="grid items-start gap-4 lg:grid-cols-[minmax(11rem,15rem)_minmax(0,1fr)]">
          <ul
            aria-label="Saved visualization list"
            className="max-h-52 divide-y overflow-auto rounded-md border bg-card lg:max-h-[65vh]"
          >
            {items.map((item) => {
              const key = visualizationKey(item);
              const Icon = icons[item.type];
              return (
                <li key={key}>
                  <button
                    type="button"
                    aria-pressed={key === visualizationKey(selected)}
                    className="flex w-full items-start gap-2 px-3 py-2.5 text-left text-sm hover:bg-muted focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring aria-pressed:bg-muted aria-pressed:font-medium"
                    onClick={() => {
                      setSelection(key);
                      update.reset();
                    }}
                  >
                    <Icon
                      aria-hidden="true"
                      className="mt-0.5 size-4 shrink-0"
                    />
                    <span className="min-w-0 [overflow-wrap:anywhere]">
                      {item.title}
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
          <div className="min-w-0 [&>section]:mt-0">
            <Visualization
              key={visualizationKey(selected)}
              project={project}
              worker={selected.worker_id}
              id={selected.artifact_id}
              savedTitle={selected.title}
            />
            <div className="mt-2 flex flex-wrap items-center justify-end gap-2">
              {unavailable === visualizationKey(selected) && (
                <p role="status" className="text-sm text-muted-foreground">
                  An available data science task on the original worker is
                  needed to request an edit.
                </p>
              )}
              {update.isError && (
                <p role="status" className="text-sm text-destructive">
                  Could not remove this save. Try again.
                </p>
              )}
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => {
                  if (!onRevise(selected))
                    setUnavailable(visualizationKey(selected));
                }}
              >
                <MessageSquare />
                Ask agent to edit
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                disabled={update.isPending}
                onClick={() =>
                  update.mutate({
                    worker_id: selected.worker_id,
                    artifact_id: selected.artifact_id,
                    saved: false,
                  })
                }
              >
                Remove from saved
              </Button>
            </div>
          </div>
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">
          Select Save on a visualization to keep it here.
        </p>
      )}
    </div>
  );
}
