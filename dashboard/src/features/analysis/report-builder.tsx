import { useId, useRef, useState } from "react";
import { Link } from "@tanstack/react-router";
import {
  ChartNoAxesCombined,
  Check,
  Copy,
  Map as MapIcon,
  Table2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { ErrorNotice } from "@/shared/components";
import { DataError } from "./data-state";
import { MarkdownEditor } from "./markdown-editor";
import {
  reportDraft,
  reportReference,
  reportMarkdown,
  reportSnippet,
  type ReportDraft,
} from "./report-model";
import { useVisualizationDetails } from "./use-visualization-details";
import { useWorkerVisualizations } from "./use-worker-visualizations";

const icons = { table: Table2, chart: ChartNoAxesCombined, geo: MapIcon };

export function ReportBuilder({
  project,
  work,
  worker,
  source,
  draft,
  onChange,
  onSave,
  onCancel,
}: {
  project: string;
  work: string;
  worker: string;
  source?: { path: string; markdown: string };
  draft: ReportDraft;
  onChange: (draft: ReportDraft) => void;
  onSave: (title: string, markdown: string) => Promise<void>;
  onCancel?: () => void;
}) {
  const formId = useId();
  const details = useVisualizationDetails(project);
  const catalog = useWorkerVisualizations(project, worker);
  const saving = useRef(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const [copyStatus, setCopyStatus] = useState<{
    ref: string;
    message: string;
    snippet?: string;
  }>();

  const items = (catalog.data?.items ?? []).map((item) => ({
    ...item,
    worker_id: worker,
    title:
      details.data?.titles?.find(
        (edit) =>
          edit.worker_id === worker && edit.artifact_id === item.artifact_id,
      )?.title ?? item.title,
  }));
  const findings = draft.findings ?? "";

  const canSave =
    !busy && !!draft.title.trim() && (!!findings.trim() || !!source);

  async function save() {
    if (!canSave || saving.current) return;
    saving.current = true;
    setBusy(true);
    setError(undefined);
    try {
      const title = draft.title.trim();
      const original = source && reportDraft(source.markdown, source.path);
      const markdown =
        source && original?.title === title && original.findings === findings
          ? source.markdown
          : reportMarkdown(title, findings);
      if (new TextEncoder().encode(markdown).length > 256 * 1024)
        throw new Error("Reports can contain up to 256 KiB of Markdown.");
      await onSave(title, markdown);
    } catch (cause) {
      setError(cause);
    } finally {
      saving.current = false;
      setBusy(false);
    }
  }

  return (
    <div className="min-w-0 px-4 py-5 sm:px-6">
      <header className="mb-6 flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-lg font-semibold">
          {source ? "Edit report" : "New report"}
        </h2>
        <div className="flex flex-wrap items-center gap-2">
          {onCancel && (
            <Button
              type="button"
              size="sm"
              variant="ghost"
              disabled={busy}
              onClick={onCancel}
            >
              Cancel
            </Button>
          )}
          <Button type="submit" size="sm" form={formId} disabled={!canSave}>
            Save and preview
          </Button>
        </div>
      </header>
      {busy && (
        <p role="status" className="mb-4 text-sm text-muted-foreground">
          Saving report…
        </p>
      )}
      {error !== undefined && <ErrorNotice error={error} />}
      <form
        id={formId}
        onSubmit={(event) => {
          event.preventDefault();
          void save();
        }}
      >
        <fieldset
          disabled={busy}
          className="grid min-w-0 items-start gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(13rem,18rem)]"
        >
          <div className="min-w-0 space-y-5">
            <label className="grid gap-2 text-sm font-medium">
              Report title
              <Input
                value={draft.title}
                maxLength={200}
                required
                onChange={(event) =>
                  onChange({ ...draft, title: event.target.value })
                }
              />
            </label>
            <MarkdownEditor
              value={findings}
              onChange={(findings) => onChange({ ...draft, findings })}
            />
          </div>
          <div className="min-w-0 space-y-3 text-sm">
            <h3 className="font-medium">Visualizations</h3>
            {catalog.isError && (
              <DataError
                error={catalog.error}
                retry={() => void catalog.refetch()}
              />
            )}
            {worker && catalog.isPending && (
              <p role="status" className="text-muted-foreground">
                Loading visualizations…
              </p>
            )}
            {(!worker || (catalog.isSuccess && !items.length)) && (
              <p className="text-muted-foreground">No visualizations.</p>
            )}
            {!!items.length && (
              <>
                <p className="text-xs text-muted-foreground">
                  Paste the copied snippet on a blank line in your report.
                </p>
                <ul
                  aria-label="Visualizations"
                  className="divide-y rounded-md border"
                >
                  {items.map((item) => {
                    const Icon = icons[item.type];
                    const ref = reportReference(item);
                    const copied =
                      copyStatus?.ref === ref && !copyStatus.snippet;
                    return (
                      <li
                        key={ref}
                        className="flex items-center gap-2 py-1.5 pr-1.5 pl-3"
                      >
                        <Icon
                          aria-hidden="true"
                          className="size-4 shrink-0 text-muted-foreground"
                        />
                        <Link
                          className="min-w-0 flex-1 break-words underline-offset-2 hover:underline focus-visible:underline"
                          to="/projects/$projectId/$section"
                          params={{ projectId: project, section: "work" }}
                          search={{
                            work,
                            view: "workspace",
                            artifact: item.artifact_id,
                          }}
                        >
                          {item.title}
                        </Link>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon-sm"
                          aria-label={`Copy Markdown for ${item.title}`}
                          title="Copy Markdown"
                          onClick={async () => {
                            const snippet = reportSnippet(item);
                            try {
                              await navigator.clipboard.writeText(snippet);
                              setCopyStatus({
                                ref,
                                message: `Copied Markdown for ${item.title}.`,
                              });
                            } catch {
                              setCopyStatus({
                                ref,
                                message: "Select and copy this snippet.",
                                snippet,
                              });
                            }
                          }}
                        >
                          {copied ? <Check /> : <Copy />}
                        </Button>
                      </li>
                    );
                  })}
                </ul>
                <p role="status" className="text-xs text-muted-foreground">
                  {copyStatus?.message}
                </p>
                {copyStatus?.snippet && (
                  <Textarea
                    aria-label="Markdown snippet"
                    readOnly
                    rows={3}
                    value={copyStatus.snippet}
                    onFocus={(event) => event.target.select()}
                  />
                )}
              </>
            )}
          </div>
        </fieldset>
      </form>
    </div>
  );
}
