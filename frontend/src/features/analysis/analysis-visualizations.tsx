import { useRef, useState } from "react";
import { useQueries } from "@tanstack/react-query";
import {
  BarChart3,
  Map as MapIcon,
  Menu,
  MessageCircleQuestionMark,
  MessageSquare,
  Table2,
  Trash2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Empty, ErrorNotice, Modal } from "@/shared/components";
import type {
  VisualizationReference,
  VisualizationRevision,
  VisualizationSummary,
} from "./api";
import { DataError } from "./data-state";
import { useVisualizationDetails } from "./use-visualization-details";
import {
  useDeleteVisualization,
  useWorkerVisualizations,
} from "./use-worker-visualizations";
import { Visualization } from "./visualization";
import { reportReference } from "./report-model";
import { visualizationSpecOptions } from "./use-visualization-spec";

export type AnalysisReference = VisualizationReference & {
  usedInReport?: boolean;
  reportPaths?: string[];
};

type AnalysisResult = AnalysisReference &
  Pick<VisualizationSummary, "title"> & {
    type?: VisualizationSummary["type"];
  };

const icons = { chart: BarChart3, geo: MapIcon, table: Table2 };

export function AnalysisVisualizations({
  project,
  worker,
  initialArtifact,
  references,
  pinned,
  status,
  emptyMessage,
  canDelete,
  onAsk,
  onRevise,
}: {
  project: string;
  worker: string;
  initialArtifact?: string;
  references: AnalysisReference[];
  pinned: boolean;
  status?: string;
  emptyMessage: string;
  canDelete: boolean;
  onAsk?: (item: VisualizationReference) => void;
  onRevise: (item: VisualizationRevision) => boolean;
}) {
  const list = useWorkerVisualizations(project, worker, !pinned);
  const details = useVisualizationDetails(project);
  const [selection, setSelection] = useState(
    initialArtifact
      ? reportReference({ worker_id: worker, artifact_id: initialArtifact })
      : "",
  );
  const actionMovedFocus = useRef(false);
  const actionsTrigger = useRef<HTMLButtonElement>(null);
  const deleteTrigger = useRef<HTMLElement | null>(null);
  const [unavailable, setUnavailable] = useState("");
  const [deleting, setDeleting] = useState<AnalysisResult>();
  const remove = useDeleteVisualization(project, deleting?.worker_id ?? worker);

  const catalog = new Map(
    (pinned ? [] : (list.data?.items ?? [])).map((item) => [
      item.artifact_id,
      item,
    ]),
  );
  const withoutSummary = references.filter(
    (ref) => ref.worker_id !== worker || !catalog.has(ref.artifact_id),
  );
  const specs = useQueries({
    queries: withoutSummary.map((ref) => ({
      ...visualizationSpecOptions(project, ref.worker_id, ref.artifact_id),
      enabled: pinned || !worker || ref.worker_id !== worker || !list.isPending,
    })),
  });
  const items = new Map<string, AnalysisResult>();
  for (const [index, ref] of withoutSummary.entries()) {
    const spec = specs[index].data;
    items.set(reportReference(ref), {
      ...ref,
      title: spec?.title ?? ref.artifact_id,
      type: spec?.type,
    });
  }
  const results = references.map((ref) => {
    const summary = ref.worker_id === worker && catalog.get(ref.artifact_id);
    if (summary) {
      catalog.delete(ref.artifact_id);
      return { ...ref, ...summary };
    }
    return items.get(reportReference(ref))!;
  });
  results.push(
    ...[...catalog.values()].map((item) => ({ ...item, worker_id: worker })),
  );
  const selected =
    results.find((item) => reportReference(item) === selection) ?? results[0];
  const selectedKey = selected && reportReference(selected);
  const missingLink =
    !pinned &&
    list.isSuccess &&
    !!initialArtifact &&
    selection === `${worker}/${initialArtifact}` &&
    selectedKey !== selection;

  function title(item: AnalysisResult) {
    return (
      details.data?.titles?.find(
        (edit) =>
          edit.worker_id === item.worker_id &&
          edit.artifact_id === item.artifact_id,
      )?.title ?? item.title
    );
  }

  return (
    <div className="min-w-0 px-4 py-5 sm:px-6">
      {missingLink && (
        <p role="status" className="mb-3 text-sm text-muted-foreground">
          The linked visualization is unavailable on this worker.
        </p>
      )}
      {status && (
        <p role="status" className="mb-3 text-sm text-muted-foreground">
          {status}
        </p>
      )}
      {selected ? (
        <div className="grid min-w-0 overflow-hidden rounded-md border bg-card lg:grid-cols-[clamp(16rem,22vw,18rem)_minmax(0,1fr)]">
          <aside className="min-w-0 border-b lg:border-r lg:border-b-0">
            <h2 className="border-b px-3 py-3 text-sm font-semibold">
              Visualizations
            </h2>
            <ul
              aria-label="Visualization list"
              className="max-h-52 divide-y overflow-auto lg:max-h-[65vh]"
            >
              {results.map((item) => {
                const key = reportReference(item);
                const Icon = item.type ? icons[item.type] : BarChart3;
                return (
                  <li
                    key={key}
                    data-selected={key === selectedKey}
                    className="flex items-center hover:bg-muted/50 data-[selected=true]:bg-muted"
                  >
                    <button
                      type="button"
                      aria-pressed={key === selectedKey}
                      className="flex min-w-0 flex-1 self-stretch items-center gap-2 px-3 py-2.5 text-left text-sm focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring aria-pressed:font-medium"
                      onClick={() => {
                        setSelection(key);
                      }}
                    >
                      <Icon
                        aria-hidden="true"
                        className="size-4 shrink-0"
                      />
                      <span className="min-w-0 [overflow-wrap:anywhere]">
                        {title(item)}
                        {item.usedInReport && (
                          <span className="mt-0.5 block text-xs font-normal text-muted-foreground">
                            Used in report
                          </span>
                        )}
                      </span>
                    </button>
                    {canDelete && item.worker_id === worker && item.type && (
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        className="mr-2 shrink-0 text-muted-foreground hover:text-destructive"
                        aria-label={`Delete visualization: ${title(item)}`}
                        title="Delete visualization"
                        onClick={(event) => {
                          deleteTrigger.current = event.currentTarget;
                          remove.reset();
                          setDeleting(item);
                        }}
                      >
                        <Trash2 />
                      </Button>
                    )}
                  </li>
                );
              })}
            </ul>
          </aside>
          <div className="analysis-result flex min-w-0 flex-col">
            <Visualization
              key={selectedKey}
              className="m-0 flex flex-1 flex-col rounded-none border-0 bg-transparent"
              project={project}
              worker={selected.worker_id}
              id={selected.artifact_id}
              fallbackTitle={title(selected)}
              canEditDetails
              actions={
                selected.type && (
                  <DropdownMenu modal={false}>
                    <DropdownMenuTrigger asChild>
                      <Button
                        ref={actionsTrigger}
                        type="button"
                        variant="outline"
                        size="icon-sm"
                        aria-label="Actions"
                        title="Actions"
                      >
                        <Menu />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent
                      align="end"
                      onCloseAutoFocus={(event) => {
                        if (actionMovedFocus.current) event.preventDefault();
                        actionMovedFocus.current = false;
                      }}
                    >
                      {onAsk && selected.worker_id === worker && (
                        <DropdownMenuItem
                          onSelect={() => {
                            actionMovedFocus.current = true;
                            onAsk(selected);
                          }}
                        >
                          <MessageCircleQuestionMark /> Ask about this result
                        </DropdownMenuItem>
                      )}
                      <DropdownMenuItem
                        onSelect={() => {
                          actionMovedFocus.current = onRevise({
                            worker_id: selected.worker_id,
                            artifact_id: selected.artifact_id,
                            report_paths:
                              pinned || selected.worker_id !== worker
                                ? undefined
                                : selected.reportPaths,
                            title: title(selected),
                          });
                          if (!actionMovedFocus.current)
                            setUnavailable(reportReference(selected));
                        }}
                      >
                        <MessageSquare />
                        {selected.reportPaths?.length &&
                        !pinned &&
                        selected.worker_id === worker
                          ? "Ask agent to edit in report"
                          : "Ask agent to edit"}
                      </DropdownMenuItem>
                      {canDelete && selected.worker_id === worker && (
                        <>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            variant="destructive"
                            onSelect={() => {
                              actionMovedFocus.current = true;
                              deleteTrigger.current = actionsTrigger.current;
                              remove.reset();
                              setDeleting(selected);
                            }}
                          >
                            <Trash2 /> Delete visualization
                          </DropdownMenuItem>
                        </>
                      )}
                    </DropdownMenuContent>
                  </DropdownMenu>
                )
              }
            />
            {unavailable === selectedKey && (
              <p
                role="status"
                className="px-4 pb-3 text-sm text-muted-foreground"
              >
                An available data science task on the original worker is needed
                to request an edit.
              </p>
            )}
          </div>
        </div>
      ) : !pinned && worker && list.isPending ? (
        !status && (
          <p role="status" className="text-sm text-muted-foreground">
            Loading visualizations…
          </p>
        )
      ) : !status && !pinned && list.isError ? (
        <DataError error={list.error} retry={() => void list.refetch()} />
      ) : !status ? (
        <Empty title="No visualizations yet">{emptyMessage}</Empty>
      ) : null}
      {deleting && (
        <Modal
          title="Delete visualization"
          onClose={() => setDeleting(undefined)}
          onCloseAutoFocus={(event) => {
            const trigger = deleteTrigger.current?.isConnected
              ? deleteTrigger.current
              : actionsTrigger.current;
            if (trigger) {
              event.preventDefault();
              trigger.focus();
            }
            deleteTrigger.current = null;
          }}
          closeDisabled={remove.isPending}
        >
          <p className="text-sm">
            Delete <strong>{title(deleting)}</strong> from this worker? Reports
            and replies that reference it will show it as unavailable. The
            underlying data stays on the worker.
          </p>
          {remove.isError && <ErrorNotice error={remove.error} />}
          <div className="form-actions pt-4">
            <Button
              type="button"
              variant="outline"
              disabled={remove.isPending}
              onClick={() => setDeleting(undefined)}
            >
              Cancel
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={remove.isPending}
              onClick={() =>
                remove.mutate(deleting.artifact_id, {
                  onSuccess: () => setDeleting(undefined),
                })
              }
            >
              {remove.isPending ? "Deleting…" : "Delete visualization"}
            </Button>
          </div>
        </Modal>
      )}
    </div>
  );
}
