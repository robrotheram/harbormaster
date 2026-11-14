import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Check, Loader2, Minus, Play, Square, Trash2, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Modal } from "@/shared/components";
import { keys } from "@/shared/query-keys";
import type { Project, Workspace } from "@/shared/types";
import { workspacesApi } from "@/features/workspaces/api";
import { snapshotOptions } from "./api";

export type WorkerAction = "start" | "stop" | "delete";
export type WorkerBatch = { action: WorkerAction; workers: Workspace[] };
const actionLabels = { start: "Start", stop: "Stop", delete: "Delete" };

export function WorkerCheckbox({
  label,
  checked,
  indeterminate = false,
  disabled = false,
  onChange,
}: {
  label: string;
  checked: boolean;
  indeterminate?: boolean;
  disabled?: boolean;
  onChange: () => void;
}) {
  return (
    <label className="worker-checkbox">
      <input
        type="checkbox"
        aria-label={label}
        checked={checked}
        disabled={disabled}
        ref={(input) => {
          if (input) input.indeterminate = indeterminate;
        }}
        onChange={onChange}
      />
      <span className="worker-checkbox-mark" aria-hidden="true">
        {indeterminate ? <Minus /> : <Check />}
      </span>
    </label>
  );
}

export function WorkerSelection({
  visible,
  checked,
  onChange,
  onAction,
}: {
  visible: Workspace[];
  checked: Set<string>;
  onChange: (ids: Set<string>) => void;
  onAction: (batch: WorkerBatch) => void;
}) {
  const chosen = visible.filter((w) => checked.has(w.id));
  const all = visible.length > 0 && chosen.length === visible.length;
  return (
    <div id="worker-selection-controls" className="worker-selection">
      <div className="worker-selection-summary">
        <WorkerCheckbox
          label="Select all visible workers"
          checked={all}
          disabled={!visible.length}
          indeterminate={chosen.length > 0 && !all}
          onChange={() =>
            onChange(new Set(all ? [] : visible.map((w) => w.id)))
          }
        />
        <span>
          {chosen.length ? `${chosen.length} selected` : "Select workers"}
        </span>
        {!!chosen.length && (
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label="Clear worker selection"
            onClick={() => onChange(new Set())}
          >
            <X />
          </Button>
        )}
      </div>
      <div className="worker-selection-actions">
        {(["start", "stop", "delete"] as const).map((action) => {
          const Icon =
            action === "start" ? Play : action === "stop" ? Square : Trash2;
          return (
            <Button
              key={action}
              size="sm"
              variant={action === "delete" ? "destructive" : "outline"}
              disabled={!chosen.length}
              aria-label={`${actionLabels[action]} selected workers`}
              onClick={() => onAction({ action, workers: chosen })}
            >
              <Icon />
              {actionLabels[action]}
            </Button>
          );
        })}
      </div>
    </div>
  );
}

type Result = {
  status: "queued" | "running" | "succeeded" | "failed";
  error?: string;
};
export function BulkWorkerAction({
  batch,
  projects,
  onSucceeded,
  onClose,
}: {
  batch: WorkerBatch;
  projects: Project[];
  onSucceeded: (id: string) => void;
  onClose: () => void;
}) {
  const client = useQueryClient();
  const [results, setResults] = useState<Record<string, Result>>({});
  const [running, setRunning] = useState(false);
  const inFlight = useRef(false);
  const mounted = useRef(true);
  const deliveryKeys = useRef(new Map<string, string>());
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const { action, workers } = batch;
  const succeeded = Object.values(results).filter(
    (r) => r.status === "succeeded",
  ).length;
  const failed = Object.values(results).filter(
    (r) => r.status === "failed",
  ).length;
  const attempted = Object.keys(results).length > 0;
  async function execute() {
    if (inFlight.current) return;
    const targets = workers.filter(
      (w) => results[w.id]?.status !== "succeeded",
    );
    inFlight.current = true;
    setRunning(true);
    setResults((current) => ({
      ...current,
      ...Object.fromEntries(
        targets.map((w) => [w.id, { status: "queued" as const }]),
      ),
    }));
    let next = 0;
    async function consume() {
      while (mounted.current && next < targets.length) {
        const worker = targets[next++];
        setResults((current) => ({
          ...current,
          [worker.id]: { status: "running" },
        }));
        let key = deliveryKeys.current.get(worker.id);
        if (!key) {
          key = crypto.randomUUID();
          deliveryKeys.current.set(worker.id, key);
        }
        try {
          if (action === "start") await workspacesApi.start(worker.id, key);
          else if (action === "stop") await workspacesApi.stop(worker.id, key);
          else await workspacesApi.delete(worker.id);
          if (mounted.current) {
            setResults((current) => ({
              ...current,
              [worker.id]: { status: "succeeded" },
            }));
            onSucceeded(worker.id);
          }
          void client.invalidateQueries({
            queryKey: keys.project(worker.project_id),
          });
        } catch (error) {
          if (mounted.current)
            setResults((current) => ({
              ...current,
              [worker.id]: {
                status: "failed",
                error:
                  error instanceof Error
                    ? error.message
                    : "The request could not finish. Retry this worker.",
              },
            }));
        }
      }
    }
    try {
      // Bound simultaneous lifecycle requests; a failed worker does not stop the batch.
      await Promise.all(
        Array.from({ length: Math.min(3, targets.length) }, consume),
      );
    } finally {
      inFlight.current = false;
      if (mounted.current) setRunning(false);
      void client.invalidateQueries({ queryKey: snapshotOptions().queryKey });
    }
  }
  const close = () => {
    if (!inFlight.current) onClose();
  };
  return (
    <Modal
      title={`${actionLabels[action]} ${workers.length} ${workers.length === 1 ? "worker" : "workers"}`}
      onClose={close}
    >
      <p className="form-note">
        {action === "delete"
          ? "Remove these workers from Workbench. Compute must stop successfully first. Private disks and saved evidence remain on the host."
          : action === "stop"
            ? "Stop compute for these workers while keeping files and history. Workers with active work may refuse to stop."
            : "Start these workers using their current settings and the available runtime capacity."}
      </p>
      <div className="bulk-worker-list">
        {workers.map((worker) => {
          const result = results[worker.id];
          return (
            <div
              key={worker.id}
              className="bulk-worker-result"
              data-result={result?.status || "selected"}
            >
              <div>
                <strong>{worker.name || worker.id}</strong>
                <span>
                  {projects.find((p) => p.id === worker.project_id)?.name ||
                    worker.project_id}
                </span>
                {result?.error && <p role="alert">{result.error}</p>}
              </div>
              <span className="bulk-worker-status">
                {result?.status === "running" ? (
                  <>
                    <Loader2 className="size-3 animate-spin" />
                    Working…
                  </>
                ) : result?.status === "succeeded" ? (
                  <>
                    <Check className="size-3" />
                    {action === "delete"
                      ? "Deleted"
                      : `${actionLabels[action]} requested`}
                  </>
                ) : result?.status === "failed" ? (
                  "Failed"
                ) : result?.status === "queued" ? (
                  "Queued"
                ) : (
                  worker.state.toLowerCase()
                )}
              </span>
            </div>
          );
        })}
      </div>
      {attempted && (
        <p role="status" className="text-xs text-muted-foreground">
          {running
            ? `${succeeded + failed} of ${workers.length} processed`
            : `${succeeded} succeeded${failed ? `, ${failed} failed` : ""}`}
        </p>
      )}
      {running && (
        <p className="form-note">
          Keep this dialog open until the requests finish.
        </p>
      )}
      <div className="form-actions">
        <Button variant="outline" disabled={running} onClick={close}>
          {attempted ? "Close" : "Cancel"}
        </Button>
        {(!attempted || failed > 0 || running) && (
          <Button
            variant={action === "delete" ? "destructive" : "default"}
            disabled={running}
            onClick={execute}
          >
            {running
              ? "Processing…"
              : attempted
                ? `Retry ${failed} failed`
                : `${actionLabels[action]} ${workers.length} ${workers.length === 1 ? "worker" : "workers"}`}
          </Button>
        )}
      </div>
    </Modal>
  );
}
