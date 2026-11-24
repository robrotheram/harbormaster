import { Visualization } from "@/features/analysis/visualization";
import { ArtifactViewer } from "@/features/delivery/review-viewer";
import type { Artifact, Attempt } from "./api";
import { ChartContent, WorkspaceChart } from "./chart-content";
import { activeAttempt } from "./conversation-api";
import { WorkspaceImage } from "./message-content";
import type { ReplyOutput } from "./reply-outputs";
import { dataPath, imagePath } from "./workspace-output";

export function retainedVisuals(attempt: Attempt, sources: ReplyOutput[]) {
  const linked = new Set(sources.map(({ path }) => path));
  return (attempt.artifacts || []).filter((file) => {
    const name = file.source_path || file.name || "";
    const path = dataPath(name) || imagePath(name);
    return !!path && !linked.has(path);
  });
}

export function WorkOutput({
  project,
  attempt,
  sources,
  files,
  onAsk,
  editVisualizations = false,
}: {
  project: string;
  attempt: Attempt;
  sources: ReplyOutput[];
  files: Artifact[];
  onAsk: (message: string) => void;
  editVisualizations?: boolean;
}) {
  return (
    <section className="workspace-output" aria-label="Agent output">
      {sources.map(({ name, path, label, source, artifact }) =>
        artifact ? (
          <Visualization
            key={artifact}
            project={project}
            worker={attempt.workspace_id}
            id={artifact}
            canEditDetails={editVisualizations}
          />
        ) : path ? (
          imagePath(path) ? (
            <WorkspaceImage
              key={path}
              src={path}
              alt={label || path}
              project={project}
              attempt={attempt}
            />
          ) : (
            <WorkspaceChart
              key={path}
              src={path}
              title={label || path}
              project={project}
              attempt={attempt}
              onAsk={onAsk}
            />
          )
        ) : source ? (
          <ChartContent
            key={name}
            source={source}
            running={activeAttempt(attempt.state)}
            onAsk={onAsk}
          />
        ) : null,
      )}
      {files.map((file) => (
        <section
          key={file.id || file.source_path || file.name}
          className="workspace-output-file"
        >
          <ArtifactViewer
            project={project}
            attempt={attempt}
            artifact={file}
            onAsk={onAsk}
            showChart
          />
        </section>
      ))}
    </section>
  );
}
