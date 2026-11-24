import {
  tasksApi,
  taskKey,
  taskOptions,
  resultOptions,
  type Task,
} from "./api";

import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { keys } from "@/shared/query-keys";
import {
  ErrorNotice,
  Form,
  Modal,
  State,
  useRequestKey,
} from "@/shared/components";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import type { Project, Workspace } from "@/shared/types";
export function TaskPanel({
  project,
  workspace,
  onClose,
}: {
  project: Project;
  workspace: Workspace;
  onClose: () => void;
}) {
  const client = useQueryClient(),
    key = useRequestKey();
  const [instruction, setInstruction] = useState(""),
    [task, setTask] = useState<Task>();
  const currentTaskKey = taskKey(project.id, workspace.id, task?.id);
  const mutation = useMutation({
    mutationFn: () => {
      const payload = {
        instruction,
        provider: workspace.agent_kind === "hermes" ? "hermes" : "codex",
      };
      return tasksApi.submit(
        workspace.id,
        payload.instruction,
        payload.provider,
        key(payload),
      );
    },
    onSuccess: (value) => {
      setTask(value);
      client.invalidateQueries({ queryKey: keys.workspaces(project.id) });
    },
  });
  const status = useQuery(taskOptions(project.id, workspace.id, task));
  const finished =
    !!status.data &&
    !["queued", "running", "cancel_requested"].includes(status.data.state);
  const resultQuery = resultOptions(
    project.id,
    workspace.id,
    task?.id,
    finished,
  );
  const resultKey = resultQuery.queryKey;
  const result = useQuery(resultQuery);
  const cancellation = useMutation({
    mutationFn: () => tasksApi.cancel(workspace.id, task!.id),
    onSuccess: () => client.invalidateQueries({ queryKey: currentTaskKey }),
  });
  useEffect(
    () => () => {
      client.removeQueries({ queryKey: currentTaskKey });
      client.removeQueries({ queryKey: resultKey });
    },
    [client, project.id, workspace.id, task?.id],
  );
  return (
    <Modal title="Run a development task" onClose={onClose} wide>
      <p className="form-note">
        {workspace.name || "Workspace"} in {project.name}. The native{" "}
        {workspace.agent_kind === "hermes" ? "Hermes" : "Codex"}
        agent runs inside this workspace using the configured subscription.
        Closing this dialog does not cancel an accepted task.
      </p>
      {!task ? (
        <Form onSubmit={() => mutation.mutate()}>
          <label>
            Instructions
            <Textarea
              rows={6}
              required
              value={instruction}
              onChange={(event) => setInstruction(event.target.value)}
              maxLength={64000}
              placeholder="Implement the next change, run the relevant checks and report the result."
            />
          </label>
          {mutation.isError && <ErrorNotice error={mutation.error} />}
          <div className="form-actions">
            <Button variant="outline" type="button" onClick={onClose}>
              Cancel
            </Button>
            <Button variant="default" disabled={mutation.isPending}>
              {mutation.isPending ? "Submitting…" : "Run task"}
            </Button>
          </div>
        </Form>
      ) : (
        <>
          <div className="task-status">
            <State value={status.data?.state || task.state} />
            <span className="identifier">{task.id}</span>
          </div>
          {status.isError && <ErrorNotice error={status.error} />}
          {status.data?.error_code && (
            <div className="notice error">
              Task reported {status.data.error_code}. Check provider
              availability and workspace state.
            </div>
          )}
          {cancellation.isError && <ErrorNotice error={cancellation.error} />}
          {result.isError && <ErrorNotice error={result.error} />}
          {result.data && (
            <>
              <pre className="task-result">
                {result.data.content || "No result text was returned."}
              </pre>
              {result.data.next_offset !== -1 && (
                <p className="form-note">
                  This view shows the first 64 KiB. Retrieve remaining result
                  pages through the task API.
                </p>
              )}
            </>
          )}
          <div className="form-actions">
            {!finished && (
              <Button
                variant="outline"
                onClick={() => cancellation.mutate()}
                disabled={
                  cancellation.isPending ||
                  status.data?.state === "cancel_requested"
                }
              >
                {cancellation.isPending ||
                status.data?.state === "cancel_requested"
                  ? "Cancellation requested"
                  : "Request cancellation"}
              </Button>
            )}
            <Button variant="outline" onClick={onClose}>
              Close
            </Button>
          </div>
        </>
      )}
    </Modal>
  );
}
