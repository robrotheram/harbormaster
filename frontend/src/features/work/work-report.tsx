import { useEffect, useMemo, useRef, useState } from "react";
import {
  keepPreviousData,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { FileText, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
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
  type ReportRevision,
} from "@/features/analysis/report-model";
import { ReportPreview } from "@/features/analysis/report-preview";
import { workApi, type Attempt, type WorkItem } from "./api";
import {
  loadCompletedReports,
  reportPathsInReply,
  type CompletedReport,
} from "./completed-report";
import { activeAttempt } from "./conversation-api";
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
  onAskAgent,
}: {
  project: string;
  work: WorkItem;
  attempts: Attempt[];
  pinnedAttempt?: string;
  loadingAttempts: boolean;
  reply: string;
  replyAttempt?: string;
  canEdit: boolean;
  onAskAgent?: (report: ReportRevision) => boolean;
}) {
  const client = useQueryClient();
  const personal = usePersonalWork();
  const [selected, setSelected] = useState<string>();
  const [editing, setEditing] = useState<CompletedReport | "new">();
  const [agentUnavailable, setAgentUnavailable] = useState(false);
  const newPath = useRef<string>(undefined);
  const mentionedPaths = useMemo(() => reportPathsInReply(reply), [reply]);
  const candidates = attempts
    .filter(
      (attempt) =>
        !activeAttempt(attempt.state) &&
        (!pinnedAttempt || attempt.id === pinnedAttempt),
    )
    .reverse();
  const queryKey = [
    "project",
    project,
    "work-reports",
    work.id,
    work.workspace_id,
    pinnedAttempt,
    candidates.map((attempt) => attempt.id),
    replyAttempt,
    mentionedPaths,
  ] as const;
  const library = useQuery({
    ...privateContentOptions,
    queryKey,
    enabled: !loadingAttempts,
    placeholderData: keepPreviousData,
    retry: false,
    queryFn: ({ signal }) =>
      loadCompletedReports({
        project,
        work,
        attempts: candidates,
        replyAttempt,
        mentionedPaths,
        live: !pinnedAttempt,
        signal,
      }),
  });
  const reports = library.data ?? [];
  const current =
    reports.find((report) => reportKey(report) === selected) ?? reports[0];
  const currentKey = current && reportKey(current);
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

  async function save(title: string, markdown: string, askAgent: boolean) {
    if (!source && !newPath.current) {
      const suffix = Array.from(
        crypto.getRandomValues(new Uint8Array(6)),
        (value) => value.toString(16).padStart(2, "0"),
      ).join("");
      newPath.current = `reports/${reportFilename(title)}-${suffix}.md`;
    }
    const path = source?.path ?? newPath.current!;
    if (markdown !== source?.markdown)
      await workApi.saveReport(project, work.id, worker, {
        path,
        markdown,
        previous: source?.markdown,
      });
    const saved = { path, worker, markdown };
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
    setEditing(undefined);
    setAgentUnavailable(
      askAgent && !onAskAgent?.({ path, title, worker_id: worker }),
    );
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
              onClick={() => {
                setAgentUnavailable(false);
                setEditing("new");
              }}
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
                      setAgentUnavailable(false);
                      setSelected(key);
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
        {agentUnavailable && (
          <p
            role="status"
            className="px-4 pt-4 text-sm text-muted-foreground sm:px-6"
          >
            This report is saved. Open an available task on its worker to
            request an agent edit.
          </p>
        )}
        {showEditor ? (
          <ReportBuilder
            key={draftKey}
            project={project}
            source={source}
            draft={draft}
            onChange={(draft) =>
              personal.setReportDrafts((old) => ({ ...old, [draftKey]: draft }))
            }
            onSave={save}
            onAskAgent={!!onAskAgent}
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
            source={current}
            onEdit={
              editable &&
              !library.isPlaceholderData &&
              current.worker === work.workspace_id
                ? () => {
                    setAgentUnavailable(false);
                    setEditing(current);
                  }
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
  onEdit,
}: {
  project: string;
  source: CompletedReport;
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
      onEdit={onEdit}
    />
  );
}
