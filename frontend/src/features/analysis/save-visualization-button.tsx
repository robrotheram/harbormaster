import { Bookmark } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  useSavedVisualizations,
  useUpdatePersonalVisualization,
} from "./use-saved-visualizations";

export function SaveVisualizationButton({
  project,
  worker,
  id,
}: {
  project: string;
  worker: string;
  id: string;
}) {
  const list = useSavedVisualizations(project);
  const update = useUpdatePersonalVisualization(project);
  const saved =
    list.data?.items.some(
      (item) => item.worker_id === worker && item.artifact_id === id,
    ) ?? false;
  return (
    <div className="shrink-0 space-y-1 text-right">
      <Button
        type="button"
        variant={saved ? "ghost" : "outline"}
        size="sm"
        title={saved ? "Remove from saved visualizations" : undefined}
        aria-label="Save visualization"
        aria-pressed={saved}
        disabled={list.isPending || update.isPending}
        onClick={() => {
          if (list.isError) void list.refetch();
          else
            update.mutate({
              worker_id: worker,
              artifact_id: id,
              saved: !saved,
            });
        }}
      >
        <Bookmark className={saved ? "fill-current" : undefined} />
        {saved ? "Saved" : "Save"}
      </Button>
      {(update.isError || list.isError) && (
        <p role="status" className="max-w-40 text-xs text-destructive">
          {list.isError
            ? "Could not load saves. Try again."
            : saved
              ? "Could not remove this save."
              : "Could not save. Try again."}
        </p>
      )}
    </div>
  );
}
