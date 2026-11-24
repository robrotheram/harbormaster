import { useMemo } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { privateContentOptions } from "@/shared/private-content";
import { activeAttempt } from "./conversation-api";
import {
  evidenceURL,
  workApi,
  type Attempt,
  type TaskChanges,
  type WorkItem,
} from "./api";
import { filePatches, wholeFileFromPatch } from "./file-diff";
import { readOutput } from "./read-output";
import { workspacePath } from "./workspace-output";

export type CompletedReport = {
  path: string;
  worker: string;
  markdown: string;
  captures?: string[];
};

export function reportPathPrefix(work: string) {
  return `reports/${work}-`;
}

function markdownPath(value: string) {
  const path = workspacePath(value);
  if (path && /\.(?:md|markdown)$/i.test(path)) return path;
}

function isReport(path: string) {
  return (
    /(?:^|\/)reports\//i.test(path) ||
    /(?:^|[./_-])report(?:[._-]|$)/i.test(path)
  );
}

export function reportPathsInReply(reply: string) {
  const paths = new Set<string>();
  for (const match of reply.matchAll(/`([^`\r\n]+)`|\]\(<?([^\s)>]+)>?\)/g)) {
    const path = markdownPath(match[1] || match[2]);
    if (path) paths.add(path);
  }
  return [...paths];
}

function decodeReport(source: string) {
  const raw = atob(source);
  if (raw.length > 262144)
    throw new Error("This report is too large to preview.");
  return new TextDecoder("utf-8", { fatal: true, ignoreBOM: true }).decode(
    Uint8Array.from(raw, (ch) => ch.charCodeAt(0)),
  );
}

export async function loadCompletedReports({
  project,
  work,
  attempts,
  replyAttempt,
  mentionedPaths,
  live,
  signal,
}: {
  project: string;
  work: WorkItem;
  attempts: Attempt[];
  replyAttempt?: string;
  mentionedPaths: string[];
  live: boolean;
  signal: AbortSignal;
}): Promise<CompletedReport[]> {
  const knownPaths = new Set<string>();
  const deletedPaths = new Set<string>();
  const reports = new Map<string, CompletedReport>();
  const liveFiles = live
    ? workApi.files(project, work.id, signal).catch(() => undefined)
    : undefined;
  let failed: unknown;
  for (const attempt of attempts) {
    signal.throwIfAborted();
    const mentioned = attempt.id === replyAttempt ? mentionedPaths : [];
    let changes: TaskChanges | null = null;
    try {
      if (attempt.task_id)
        changes = await workApi.changes(project, work.id, attempt.id, signal);
    } catch (error) {
      signal.throwIfAborted();
      failed ??= error;
    }
    const paths = new Set<string>();
    const files = new Map(
      (changes?.files ?? []).map((file) => [file.path, file]),
    );
    const retained = new Map<string, string>();
    for (const artifact of attempt.artifacts ?? []) {
      const path = markdownPath(artifact.source_path || artifact.name || "");
      const url = evidenceURL(artifact.content_url);
      if (path && url?.startsWith("/ui-api/")) retained.set(path, url);
    }
    for (const path of mentioned) {
      if (files.has(path) || retained.has(path) || isReport(path))
        paths.add(path);
    }
    for (const path of [...files.keys(), ...retained.keys()]) {
      if (markdownPath(path) === path && isReport(path)) paths.add(path);
    }
    const current =
      attempt.id === work.latest_attempt_id &&
      attempt.workspace_id === work.workspace_id;
    const patches =
      changes?.diff && !changes.truncated
        ? filePatches(changes.diff, [...files.keys()])
        : undefined;
    for (const path of paths) {
      const key = `${attempt.workspace_id}/${path}`;
      if (files.get(path)?.status === "D") {
        if (!knownPaths.has(key)) deletedPaths.add(key);
        knownPaths.add(key);
        continue;
      }
      knownPaths.add(key);
      if (deletedPaths.has(key)) continue;
      try {
        let markdown: string | undefined;
        const source = files.get(path)?.source;
        const retainedURL = retained.get(path);
        const patch = patches?.get(path);
        if (source !== undefined) markdown = decodeReport(source);
        else if (retainedURL) markdown = await readOutput(retainedURL, signal);
        else if (patch)
          markdown = wholeFileFromPatch(patch, files.get(path)?.status || "");
        if (markdown === undefined && current) {
          const file = await workApi.file(project, work.id, path, signal);
          if (file.path === path && file.content !== undefined)
            markdown = decodeReport(file.content);
        }
        if (markdown !== undefined) {
          const report = reports.get(key);
          if (!report) {
            reports.set(key, {
              path,
              worker: attempt.workspace_id,
              markdown,
              captures: [markdown],
            });
          } else if (report.captures?.[0] !== markdown) {
            report.captures?.unshift(markdown);
          }
        } else {
          failed ??= new Error("The captured report could not be read.");
        }
      } catch (error) {
        signal.throwIfAborted();
        failed ??= error;
      }
    }
  }
  const files = await liveFiles;
  signal.throwIfAborted();
  const worker = work.workspace_id;
  const paths = (files?.paths ?? []).filter(
    (path) =>
      markdownPath(path) === path &&
      (knownPaths.has(`${worker}/${path}`) ||
        path.startsWith(reportPathPrefix(work.id)) ||
        // Reports created in older editors have a random suffix and no task ID.
        /^reports\/(?!work_)[^/]+-[0-9a-f]{12}\.md$/.test(path)),
  );
  for (let offset = 0; offset < paths.length; offset += 6) {
    const results = await Promise.allSettled(
      paths.slice(offset, offset + 6).map(async (path) => {
        signal.throwIfAborted();
        const file = await workApi.file(project, work.id, path, signal);
        if (file.path !== path || file.content === undefined)
          throw new Error("The report could not be read from the worker.");
        return { path, worker, markdown: decodeReport(file.content) };
      }),
    );
    for (const result of results) {
      if (result.status === "fulfilled") {
        const key = `${worker}/${result.value.path}`;
        reports.set(key, { ...reports.get(key), ...result.value });
      } else failed ??= result.reason;
    }
    signal.throwIfAborted();
  }
  if (!reports.size && failed) throw failed;
  return [...reports.values()];
}

export function useCompletedReports({
  project,
  work,
  attempts,
  pinnedAttempt,
  reply,
  replyAttempt,
  enabled,
}: {
  project: string;
  work?: WorkItem;
  attempts: Attempt[];
  pinnedAttempt?: string;
  reply: string;
  replyAttempt?: string;
  enabled: boolean;
}) {
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
    work?.id,
    work?.workspace_id,
    pinnedAttempt,
    candidates.map((attempt) => attempt.id),
    replyAttempt,
    mentionedPaths,
  ] as const;
  const library = useQuery({
    ...privateContentOptions,
    queryKey,
    enabled: enabled && !!work,
    placeholderData: keepPreviousData,
    retry: false,
    queryFn: ({ signal }) =>
      loadCompletedReports({
        project,
        work: work!,
        attempts: candidates,
        replyAttempt,
        mentionedPaths,
        live: !pinnedAttempt,
        signal,
      }),
  });
  return { library, queryKey };
}
