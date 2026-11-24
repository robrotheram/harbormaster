import { APIError } from "@/shared/http";
import { Button } from "@/components/ui/button";
import type { DataPage } from "./api";

export function DataError({
  error,
  retry,
}: {
  error: unknown;
  retry?: () => void;
}) {
  const missing =
    error instanceof APIError &&
    [404, 503].includes(error.status) &&
    [
      "NOT_FOUND_OR_FORBIDDEN",
      "ANALYSIS_DATA_UNAVAILABLE",
      "ANALYSIS_ARTIFACT_UNAVAILABLE",
      "RUNTIME_UNAVAILABLE",
      "NOT_READY",
      "WORKSPACE_UNAVAILABLE",
    ].includes(error.code);
  return (
    <div
      role="status"
      className="space-y-2 rounded-md border bg-muted/30 p-3 text-sm"
    >
      <p>
        {missing
          ? "Worker data is unavailable. Start the originating worker if it still exists."
          : error instanceof Error
            ? error.message
            : "Data could not load."}
      </p>
      {retry && (
        <Button size="sm" variant="outline" onClick={retry}>
          Retry
        </Button>
      )}
    </div>
  );
}

export function DataNotice({ data }: { data: DataPage }) {
  return (
    <>
      {data.sampled && (
        <p className="text-sm text-muted-foreground">
          Showing a sample of {data.rows.length.toLocaleString()}{" "}
          {data.rows.length === 1 ? "row" : "rows"}.
        </p>
      )}
      {data.truncated && (
        <p className="text-sm text-muted-foreground">
          Some rows or values were clipped to stay within the response limit.
        </p>
      )}
    </>
  );
}
