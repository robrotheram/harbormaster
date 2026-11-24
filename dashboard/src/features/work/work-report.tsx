import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { FileText, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/native-select";
import { ErrorNotice } from "@/shared/components";
import { privateContentOptions } from "@/shared/private-content";
import { ReportBuilder } from "@/features/analysis/report-builder";
import { prepareReportParts } from "@/features/analysis/report-generation";
import {
  defaultReportDraft,
  reportDraft,
  reportFilename,
  reportFindings,
  reportReference,
  reportReferences,
} from "@/features/analysis/report-model";
import { ReportPreview } from "@/features/analysis/report-preview";
import { workApi, type Attempt, type WorkItem } from "./api";
import {
  reportPathPrefix,
  useCompletedReports,
  type CompletedReport,
} from "./completed-report";
import { usePersonalWork } from "./personal-work";

function reportKey({ worker, path }: CompletedReport) {
  return `${worker}/${path}`;
}

export function WorkReport({
  project,
  work,
  attempts,
  pinnedAttempt,
  loadingAttempts,
  reply,
  replyAttempt,
  canEdit,
}: {
  project: string;
  work: WorkItem;
  attempts: Attempt[];
  pinnedAttempt?: string;
  loadingAttempts: boolean;
  reply: string;
  replyAttempt?: string;
  canEdit: boolean;
}) {
  const client = useQueryClient();
  const personal = usePersonalWork();
  const [selected, setSelected] = useState<string>();
  const [version, setVersion] = useState<number>();
  const [editing, setEditing] = useState<CompletedReport | "new">();
  const newPath = useRef<string>(undefined);
  const { library, queryKey } = useCompletedReports({
    project,
    work,
    attempts,
    pinnedAttempt,
    reply,
    replyAttempt,
    enabled: !loadingAttempts,
  });
  const reports = library.data ?? [];
  const current =
    reports.find((report) => reportKey(report) === selected) ?? reports[0];
  const currentKey = current && reportKey(current);
  const captures = current?.captures ?? [];
  const earlier =
    captures.at(-1) === current?.markdown ? captures.slice(0, -1) : captures;
  const historical = version === undefined ? undefined : earlier[version];
  useEffect(() => {
    if (currentKey) setSelected((previous) => previous ?? currentKey);
  }, [currentKey]);
  const source = editing === "new" ? undefined : editing;
  const editable = canEdit && !pinnedAttempt;
  const showEditor = editable && (!!editing || (!current && library.isFetched));
  const worker = source?.worker ?? work.workspace_id;
  const draftKey = `${work.id}:${source ? reportKey(source) : `${worker}/new`}`;
  const draft =
    personal.reportDrafts[draftKey] ??
    (source
      ? reportDraft(source.markdown, source.path)
      : {
          ...defaultReportDraft,
          findings: reports.length ? "" : reportFindings(reply),
        });

  function clearDraft() {
    personal.setReportDrafts((old) => {
      const next = { ...old };
      delete next[draftKey];
      return next;
    });
  }

  async function save(title: string, markdown: string) {
    if (!source && !newPath.current) {
      const suffix = Array.from(
        crypto.getRandomValues(new Uint8Array(6)),
        (value) => value.toString(16).padStart(2, "0"),
      ).join("");
      newPath.current = `${reportPathPrefix(work.id)}${reportFilename(title)}-${suffix}.md`;
    }
    const path = source?.path ?? newPath.current!;
    if (markdown !== source?.markdown)
      await workApi.saveReport(project, work.id, worker, {
        path,
        markdown,
        previous: source?.markdown,
      });
    const saved = { path, worker, markdown, captures: source?.captures };
    await client.cancelQueries({ queryKey, exact: true });
    client.setQueryData<CompletedReport[]>(queryKey, (old = []) => {
      const index = old.findIndex(
        (report) => reportKey(report) === reportKey(saved),
      );
      return index < 0
        ? [...old, saved]
        : old.map((report, i) => (i === index ? saved : report));
    });
    clearDraft();
    newPath.current = undefined;
    setSelected(reportKey(saved));
    setVersion(undefined);
    setEditing(undefined);
  }

  return (
    <div className="grid min-w-0 lg:grid-cols-[14rem_minmax(0,1fr)]">
      <aside
        aria-label="Reports"
        className="min-w-0 border-b px-3 py-4 lg:min-h-[70vh] lg:border-r lg:border-b-0"
      >
        <div className="mb-3 flex items-center justify-between gap-2">
          <h2 className="px-1 text-sm font-semibold">Reports</h2>
          {editable && (
            <Button
              size="xs"
              variant="outline"
              onClick={() => setEditing("new")}
            >
              <Plus /> New report
            </Button>
          )}
        </div>
        {!!reports.length && (
          <ul className="grid gap-1">
            {reports.map((report) => {
              const key = reportKey(report);
              const active =
                editing !== "new" && current && reportKey(current) === key;
              return (
                <li key={key}>
                  <Button
                    aria-current={active ? "true" : undefined}
                    className="h-auto w-full justify-start px-2 py-2 text-left whitespace-normal"
                    variant={active ? "secondary" : "ghost"}
                    size="sm"
                    title={report.path}
                    onClick={() => {
                      setSelected(key);
                      setVersion(undefined);
                      setEditing(undefined);
                    }}
                  >
                    <FileText className="shrink-0 text-muted-foreground" />
                    <span className="min-w-0 break-words">
                      {reportDraft(report.markdown, report.path).title}
                    </span>
                  </Button>
                </li>
              );
            })}
          </ul>
        )}
      </aside>
      <div className="min-w-0">
        {library.isError && (
          <div className="px-4 pt-4 sm:px-6">
            <ErrorNotice
              error={library.error}
              message="Could not load reports."
              onRetry={() => void library.refetch()}
              retrying={library.isFetching}
            />
          </div>
        )}
        {showEditor ? (
          <ReportBuilder
            key={draftKey}
            project={project}
            work={work.id}
            worker={worker}
            source={source}
            draft={draft}
            onChange={(draft) =>
              personal.setReportDrafts((old) => ({ ...old, [draftKey]: draft }))
            }
            onSave={save}
            onCancel={
              current
                ? () => {
                    clearDraft();
                    newPath.current = undefined;
                    setEditing(undefined);
                    void library.refetch();
                  }
                : undefined
            }
          />
        ) : current ? (
          <WorkspaceReportPreview
            key={reportKey(current)}
            project={project}
            source={
              historical === undefined
                ? current
                : { ...current, markdown: historical }
            }
            versionPicker={
              earlier.length > 0 && (
                <NativeSelect
                  size="sm"
                  aria-label="Report version"
                  value={historical === undefined ? "latest" : version}
                  onChange={(event) =>
                    setVersion(
                      event.target.value === "latest"
                        ? undefined
                        : Number(event.target.value),
                    )
                  }
                >
                  <option value="latest">
                    Version {earlier.length + 1} (latest)
                  </option>
                  {earlier
                    .map((_, index) => (
                      <option key={index} value={index}>
                        Version {index + 1}
                      </option>
                    ))
                    .reverse()}
                </NativeSelect>
              )
            }
            onEdit={
              editable &&
              historical === undefined &&
              !library.isPlaceholderData &&
              current.worker === work.workspace_id
                ? () => setEditing(current)
                : undefined
            }
          />
        ) : loadingAttempts || library.isPending ? (
          <p role="status" className="p-5 text-sm text-muted-foreground">
            Loading reports…
          </p>
        ) : (
          !library.isError && (
            <p className="p-5 text-sm text-muted-foreground">
              No reports are available.
            </p>
          )
        )}
      </div>
    </div>
  );
}

function WorkspaceReportPreview({
  project,
  source,
  versionPicker,
  onEdit,
}: {
  project: string;
  source: CompletedReport;
  versionPicker?: ReactNode;
  onEdit?: () => void;
}) {
  const items = useMemo(
    () => reportReferences(source.markdown, source.worker),
    [source.markdown, source.worker],
  );
  const parts = useQuery({
    ...privateContentOptions,
    queryKey: [
      "project",
      project,
      "report-visuals",
      items.map(reportReference),
    ],
    enabled: items.length > 0,
    retry: false,
    queryFn: ({ signal }) => prepareReportParts({ project, items, signal }),
  });
  return (
    <ReportPreview
      report={{
        title: reportDraft(source.markdown, source.path).title,
        markdown: source.markdown,
        parts: parts.data ?? [],
      }}
      source={source}
      preparing={items.length > 0 && parts.isPending}
      versionPicker={versionPicker}
      onEdit={onEdit}
    />
  );
}
