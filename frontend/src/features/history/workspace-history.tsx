import { workspacesApi } from "@/features/workspaces/api";
import { historyOptions } from "./history-api";
import { historyExport } from "./history-export";

import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { APIError } from "@/shared/http";
import { Modal, ErrorNotice } from "@/shared/components";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/native-select";
import type { Project, Workspace } from "@/shared/types";

function replyName(source: string) {
  if (source === "hermes" || source === "matrix") return "Hermes";
  if (source === "codex") return "Codex";
  if (source === "claude") return "Claude";
  return "Agent";
}
export function WorkspaceHistory({
  project,
  workspace,
  workspaces,
  onClose,
}: {
  project: Project;
  workspace?: Workspace;
  workspaces: Workspace[];
  onClose: () => void;
}) {
  const client = useQueryClient();
  const [selected, setSelected] = useState(workspace?.id || "");
  const [before, setBefore] = useState(0);
  const [startKey, setStartKey] = useState(() => crypto.randomUUID());
  const downloadController = useRef<AbortController | null>(null);
  useEffect(() => () => downloadController.current?.abort(), []);
  const query = useQuery(historyOptions(project.id, selected, before));
  const start = useMutation({
    mutationFn: () => workspacesApi.start(selected, startKey),
    onSuccess: async () => {
      setStartKey(crypto.randomUUID());
      await client.invalidateQueries({ queryKey: ["project", project.id] });
      await query.refetch();
    },
  });
  const download = useMutation({
    mutationFn: () => {
      downloadController.current = new AbortController();
      return historyExport(
        project.id,
        selected,
        downloadController.current.signal,
      );
    },
    onSuccess: (blob) => {
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = `${selected}-history.jsonl`;
      link.click();
      setTimeout(() => URL.revokeObjectURL(url), 30000);
    },
  });
  const busy = start.isPending || download.isPending;
  const data = query.data;
  const records = data?.page.records || [];
  return (
    <Modal
      title="Message history"
      onClose={onClose}
      wide
      closeDisabled={start.isPending}
    >
      <div className="flex flex-wrap gap-2 items-center">
        <NativeSelect
          aria-label="History worker"
          wrapperClassName="w-full sm:w-auto sm:flex-1"
          disabled={busy}
          value={selected}
          onChange={(e) => {
            setSelected(e.target.value);
            setBefore(0);
            start.reset();
            download.reset();
          }}
        >
          <option value="">Choose a worker</option>
          {workspaces.map((w) => (
            <option value={w.id} key={w.id}>
              {w.name || w.id}
            </option>
          ))}
        </NativeSelect>
        <Button
          variant="outline"
          disabled={!selected || query.isFetching || busy}
          onClick={() => {
            if (before) setBefore(0);
            else void query.refetch();
          }}
        >
          Newest
        </Button>
        <Button
          variant="outline"
          disabled={
            !data?.page.next_before || query.isError || query.isFetching || busy
          }
          onClick={() => setBefore(data!.page.next_before)}
        >
          Older
        </Button>
        <Button
          disabled={!data || query.isError || busy}
          onClick={() => download.mutate()}
        >
          {download.isPending ? "Preparing export…" : "Export JSONL"}
        </Button>
      </div>
      {data?.cache && (
        <p role="status" className="text-xs text-muted-foreground">
          Cached history as of {new Date(data.cache.synced_at).toLocaleString()}
          .
          {data.cache.cached_through < data.cache.source_high_watermark &&
            " Some recorded activity has yet to sync."}
        </p>
      )}
      {data?.page.capture_limited && (
        <p role="alert" className="notice">
          History storage is full. New messages are not being recorded.
        </p>
      )}
      {query.isError && (
        <>
          <ErrorNotice
            error={query.error}
            onRetry={() => void query.refetch()}
            retrying={query.isFetching || busy}
          />
          {query.error instanceof APIError &&
            query.error.code === "HISTORY_WORKSPACE_STOPPED" && (
              <Button disabled={busy} onClick={() => start.mutate()}>
                {start.isPending
                  ? "Starting worker…"
                  : "Start worker to view messages"}
              </Button>
            )}
        </>
      )}
      {start.isError && <ErrorNotice error={start.error} />}
      {download.isError && <ErrorNotice error={download.error} />}
      {download.isSuccess && (
        <p role="status" className="text-xs text-muted-foreground">
          History export prepared. Check your browser downloads.
        </p>
      )}
      <div
        className="max-h-[60vh] overflow-y-auto px-1 py-4"
        role="region"
        aria-label="History messages"
        tabIndex={0}
      >
        {!selected ? (
          <p className="text-sm text-muted-foreground">
            Choose a worker to see its conversations.
          </p>
        ) : query.isPending ? (
          <p role="status" className="text-sm text-muted-foreground">
            Loading messages…
          </p>
        ) : !records.length && !query.isError ? (
          <p className="text-sm text-muted-foreground">
            {data?.page.next_before
              ? "No messages in this part of the history. Select Older to continue."
              : "No messages recorded here yet."}
          </p>
        ) : (
          <ol aria-label="Conversation" className="space-y-6">
            {records.map((r) => {
              const user = r.kind === "user.message";
              const date = new Date(r.at);
              return (
                <li
                  key={r.id}
                  className={`flex ${user ? "justify-end" : "justify-start"}`}
                >
                  <div
                    className={`min-w-0 max-w-[90%] sm:max-w-[85%] ${user ? "rounded-2xl bg-muted/60 px-4 py-3" : "px-2 py-1"}`}
                  >
                    <div className="mb-1.5 flex flex-wrap items-baseline gap-x-3 gap-y-1 text-xs text-muted-foreground">
                      <span className="font-medium text-foreground/75">
                        {user ? "Message" : replyName(r.source)}
                      </span>
                      {r.purpose === "test" && (
                        <span className="rounded bg-amber-500/10 px-1.5 py-0.5 font-medium text-amber-800 dark:text-amber-200">
                          Test run
                        </span>
                      )}
                      <time dateTime={r.at} title={date.toLocaleString()}>
                        {date.toLocaleString(undefined, {
                          month: "short",
                          day: "numeric",
                          hour: "2-digit",
                          minute: "2-digit",
                        })}
                      </time>
                    </div>
                    <div className="whitespace-pre-wrap break-words text-sm leading-6 [overflow-wrap:anywhere]">
                      {r.content}
                    </div>
                    {r.truncated && (
                      <p className="mt-2 text-xs text-muted-foreground">
                        Message shortened in the recording.
                      </p>
                    )}
                  </div>
                </li>
              );
            })}
          </ol>
        )}
      </div>
      <p className="text-xs text-muted-foreground">
        Exports use JSONL: one JSON record per line, including tool activity and
        metadata. Review before sharing.
      </p>
    </Modal>
  );
}
