import { workspacesApi } from "@/features/workspaces/api";
import { historyOptions } from "./history-api";

import { useState } from "react";
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
  const query = useQuery(historyOptions(project.id, selected, before));
  const start = useMutation({
    mutationFn: () =>
      workspacesApi.start(selected, `history-start-${selected}`),
    onSuccess: () => {
      client.invalidateQueries({ queryKey: ["project", project.id] });
      query.refetch();
    },
  });
  const data = query.data;
  const records = data?.page.records || [];
  return (
    <Modal title="Message history" onClose={onClose} wide>
      <div className="flex flex-wrap gap-2 items-center">
        <NativeSelect
          aria-label="History workspace"
          value={selected}
          onChange={(e) => {
            setSelected(e.target.value);
            setBefore(0);
          }}
        >
          <option value="">Choose a workspace</option>
          {workspaces.map((w) => (
            <option value={w.id} key={w.id}>
              {w.name || w.id}
            </option>
          ))}
        </NativeSelect>
        <Button
          variant="outline"
          disabled={!selected}
          onClick={() => {
            setBefore(0);
            query.refetch();
          }}
        >
          Newest
        </Button>
        <Button
          variant="outline"
          disabled={!data?.page.next_before || query.isError}
          onClick={() => setBefore(data!.page.next_before)}
        >
          Older
        </Button>
        <Button
          disabled={!data || query.isError}
          onClick={() =>
            window.location.assign(
              `/ui-api/workspaces/${selected}/history/export`,
            )
          }
        >
          Export JSONL
        </Button>
      </div>
      {data?.page.capture_limited && (
        <p role="alert" className="notice">
          History storage is full. New messages are not being recorded.
        </p>
      )}
      {query.isError && (
        <>
          <ErrorNotice error={query.error} />
          {query.error instanceof APIError &&
            query.error.code === "HISTORY_WORKSPACE_STOPPED" && (
              <Button disabled={start.isPending} onClick={() => start.mutate()}>
                Start workspace to view messages
              </Button>
            )}
        </>
      )}
      {start.isError && <ErrorNotice error={start.error} />}
      <div className="max-h-[60vh] overflow-y-auto px-1 py-4">
        {!selected ? (
          <p className="text-sm text-muted-foreground">
            Choose a workspace to see its conversations.
          </p>
        ) : query.isPending ? (
          <p className="text-sm text-muted-foreground">Loading messages…</p>
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
                    <div className="mb-1.5 flex items-baseline gap-3 text-xs text-muted-foreground">
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
        JSONL exports include tool activity and metadata. Review before sharing.
      </p>
    </Modal>
  );
}
