import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { request } from "@/shared/http";
import { privateContentOptions } from "@/shared/private-content";
import { parseChartSpec } from "./chart-spec";
import { InteractiveChart } from "./interactive-chart";
import { activeAttempt } from "./conversation-api";
import { evidenceURL, type Attempt } from "./api";
import { readOutput } from "./read-output";
import { dataPath } from "./workspace-output";

export function ChartContent({
  source,
  title,
  running = false,
  onAsk,
}: {
  source: string;
  title?: string;
  running?: boolean;
  onAsk?: (message: string) => void;
}) {
  const parsed = useMemo(() => {
    try {
      return { spec: parseChartSpec(source, title), error: "" };
    } catch (e) {
      return { spec: undefined, error: (e as Error).message };
    }
  }, [source, title]);
  if (!parsed.spec)
    return running ? (
      <p className="conversation-muted">Preparing chart data…</p>
    ) : (
      <details className="chart-error">
        <summary>Could not draw this chart: {parsed.error}</summary>
        <pre>{source.slice(0, 10000)}</pre>
      </details>
    );
  return <InteractiveChart key={source} spec={parsed.spec} onAsk={onAsk} />;
}
export function WorkspaceChart({
  src,
  title,
  project,
  attempt,
  onAsk,
}: {
  src: string;
  title: string;
  project: string;
  attempt: Attempt;
  onAsk?: (message: string) => void;
}) {
  const path = dataPath(src);
  const artifact = attempt.artifacts?.find(
    (a) => (a.source_path || a.name) === path,
  );
  const url = evidenceURL(artifact?.content_url);
  const query = useQuery({
    ...privateContentOptions,
    queryKey: ["project", project, "message-chart", attempt.id, path, url],
    enabled: !!path && !activeAttempt(attempt.state),
    retry: false,
    staleTime: Infinity,
    queryFn: async ({ signal }) => {
      if (url?.startsWith("/ui-api/")) return readOutput(url, signal);
      const file = await request<{ content: string }>(
        `/work-items/${attempt.work_item_id}/data`,
        { project_id: project, attempt_id: attempt.id, path },
        signal,
      );
      const content = atob(file.content || "");
      if (content.length > 262144)
        throw new Error("Use data smaller than 256 KiB.");
      return new TextDecoder().decode(
        Uint8Array.from(content, (c) => c.charCodeAt(0)),
      );
    },
  });
  if (!path) return <span>Unsupported data location.</span>;
  if (query.isError)
    return (
      <div className="chart-error">
        This data is unavailable. Older runs need a retained data file.{" "}
        <button onClick={() => void query.refetch()}>Retry</button>
      </div>
    );
  if (query.data === undefined)
    return (
      <p className="conversation-muted">
        {activeAttempt(attempt.state)
          ? "Chart data will appear when this run finishes."
          : "Loading chart data…"}
      </p>
    );
  return <ChartContent source={query.data} title={title} onAsk={onAsk} />;
}
