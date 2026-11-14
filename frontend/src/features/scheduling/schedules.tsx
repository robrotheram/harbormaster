import { scheduleOptions } from "./api";
import { Badge } from "@/components/ui/badge";

import { NativeSelect } from "@/components/ui/native-select";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { keys } from "@/shared/query-keys";

import { workspacesApi } from "@/features/workspaces/api";
import { schedulingApi } from "@/features/scheduling/api";
import { date, label } from "@/shared/format";
import {
  Empty,
  ErrorNotice,
  Form,
  Modal,
  Observation,
  useRequestKey,
} from "@/shared/components";
import type { Project, Schedule } from "@/shared/types";

export function Schedules({ project }: { project: Project }) {
  const client = useQueryClient();
  const query = useQuery(scheduleOptions(project.id));
  const workspaces = useQuery({
    queryKey: keys.workspaces(project.id),
    queryFn: ({ signal }) => workspacesApi.list(project.id, signal),
  });
  const [editing, setEditing] = useState<Schedule | null>(null);
  const [deleting, setDeleting] = useState<Schedule | null>(null);
  const [create, setCreate] = useState(false),
    [message, setMessage] = useState("");
  const actionKey = useRequestKey();
  const [actionAttempt, setActionAttempt] = useState(0);
  const mutation = useMutation({
    mutationFn: ({
      id,
      action,
    }: {
      id: string;
      action: "run-now" | "enable" | "disable";
    }) =>
      schedulingApi.action(
        id,
        action,
        actionKey({ id, action, actionAttempt }),
      ),
    onSuccess: (_, variables) => {
      setActionAttempt((value) => value + 1);
      setMessage(
        variables.action === "run-now"
          ? "Run requested. Follow its state in Scheduled runs."
          : variables.action === "enable"
            ? "Schedule enabled."
            : "Schedule disabled.",
      );
      client.invalidateQueries({ queryKey: keys.project(project.id) });
    },
  });
  return (
    <>
      <div className="page-title">
        <div>
          <h1>Schedules</h1>
        </div>
        <Button
          variant="default"
          onClick={() => setCreate(true)}
          disabled={!workspaces.data?.length}
        >
          Create schedule
        </Button>
      </div>
      <div className="list-toolbar">
        <span>{query.data?.length ?? "—"} schedules</span>
        <div>
          <Observation
            at={query.dataUpdatedAt}
            fetching={query.isFetching}
            error={query.isError}
          />
          <Button
            variant="outline"
            onClick={() => {
              query.refetch();
              workspaces.refetch();
            }}
            disabled={query.isFetching || workspaces.isFetching}
          >
            Refresh
          </Button>
        </div>
      </div>
      {query.isError && <ErrorNotice error={query.error} />}
      {workspaces.isError && <ErrorNotice error={workspaces.error} />}
      {mutation.isError && <ErrorNotice error={mutation.error} />}
      {message && (
        <div className="notice" role="status">
          {message}
        </div>
      )}
      {query.isPending ? (
        <div className="loading" role="status">
          Retrieving schedules…
        </div>
      ) : query.data?.length ? (
        <div className="schedule-list">
          {query.data.map((schedule) => {
            const worker = workspaces.data?.find(
              (w) => w.id === schedule.workspace_id,
            );
            const missingWorker = workspaces.isSuccess && !worker;
            return (
              <article className="schedule-row" key={schedule.id}>
                <div className="schedule-main">
                  <div>
                    <h2>{schedule.name}</h2>
                    <p>
                      {missingWorker
                        ? "Worker unavailable"
                        : worker?.name || schedule.workspace_id}
                    </p>
                  </div>
                  <Badge
                    variant="outline"
                    className={
                      schedule.enabled
                        ? "border-teal-200 bg-teal-50 text-teal-700 dark:border-teal-900 dark:bg-teal-950 dark:text-teal-300"
                        : "text-muted-foreground"
                    }
                  >
                    {schedule.enabled ? "Enabled" : "Disabled"}
                  </Badge>
                </div>
                <dl className="schedule-details">
                  <div>
                    <dt>Instructions</dt>
                    <dd>{schedule.instruction_path}</dd>
                  </div>
                  <div>
                    <dt>Frequency</dt>
                    <dd>{frequency(schedule.interval_seconds)}</dd>
                  </div>
                  <div>
                    <dt>Next occurrence</dt>
                    <dd>
                      {schedule.enabled
                        ? date(schedule.next_run_at || schedule.run_at)
                        : "Paused"}
                    </dd>
                  </div>
                  <div>
                    <dt>Last requested</dt>
                    <dd>{date(schedule.last_run_at)}</dd>
                  </div>
                </dl>
                {missingWorker && (
                  <p className="w-full text-xs text-warning">
                    The assigned worker no longer exists. Edit this schedule to
                    choose a replacement, or delete it.
                  </p>
                )}
                {schedule.last_error_code && (
                  <p className="schedule-error">
                    Last issue: {label(schedule.last_error_code)}. Check the
                    workspace and provider before retrying.
                  </p>
                )}
                <div
                  className="row-actions flex-wrap"
                  aria-label={`Actions for ${schedule.name}`}
                >
                  <Button
                    variant="outline"
                    onClick={() => setEditing(schedule)}
                    disabled={mutation.isPending}
                  >
                    Edit
                  </Button>
                  <Button
                    variant="outline"
                    onClick={() =>
                      mutation.mutate({ id: schedule.id, action: "run-now" })
                    }
                    disabled={mutation.isPending || !worker}
                  >
                    Run now
                  </Button>
                  <Button
                    variant="outline"
                    onClick={() =>
                      mutation.mutate({
                        id: schedule.id,
                        action: schedule.enabled ? "disable" : "enable",
                      })
                    }
                    disabled={
                      mutation.isPending || (!schedule.enabled && !worker)
                    }
                  >
                    {schedule.enabled ? "Disable" : "Enable"}
                  </Button>
                  <Button
                    variant="outline"
                    className="text-destructive"
                    onClick={() => setDeleting(schedule)}
                    disabled={mutation.isPending}
                  >
                    Delete
                  </Button>
                </div>
              </article>
            );
          })}
        </div>
      ) : query.data ? (
        <Empty
          title="Put routine work on a schedule"
          action={
            workspaces.data?.length ? (
              <Button variant="default" onClick={() => setCreate(true)}>
                Create schedule
              </Button>
            ) : undefined
          }
        >
          {workspaces.data?.length
            ? "Keep the instructions in a file inside the workspace. The scheduler holds only its reference and execution metadata."
            : "Create a workspace first, then add its first schedule."}
        </Empty>
      ) : null}
      <details className="page-help">
        <summary>How scheduled execution works</summary>
        <p>
          These schedules request tasks from the worker’s native Codex or Hermes
          agent. Closing a client does not stop accepted work. Provider limits
          keep work queued or unavailable; no API-billing fallback is selected
          automatically.
        </p>
      </details>
      {deleting && (
        <DeleteSchedule
          schedule={deleting}
          onClose={() => setDeleting(null)}
          onDeleted={() => {
            setDeleting(null);
            setMessage("Schedule deleted. Run history is retained.");
            client.invalidateQueries({ queryKey: keys.project(project.id) });
          }}
        />
      )}
      {(create || editing) && (
        <ScheduleForm
          project={project}
          schedule={editing ?? undefined}
          workspaces={workspaces.data || []}
          onClose={() => {
            setCreate(false);
            setEditing(null);
          }}
          onCreated={() => {
            setCreate(false);
            setEditing(null);
            setMessage(editing ? "Schedule updated." : "Schedule created.");
            client.invalidateQueries({ queryKey: keys.schedules(project.id) });
          }}
        />
      )}
    </>
  );
}

function frequency(seconds: number) {
  return seconds === 0
    ? "One time"
    : seconds % 86400 === 0
      ? "Every " + seconds / 86400 + " day(s)"
      : seconds % 3600 === 0
        ? "Every " + seconds / 3600 + " hour(s)"
        : "Every " + seconds / 60 + " minutes";
}
function localDate() {
  return dateInput(new Date(Date.now() + 60000).toISOString());
}
function dateInput(value: string) {
  const now = new Date(value);
  return new Date(now.valueOf() - now.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16);
}

function ScheduleForm({
  project,
  schedule,
  workspaces,
  onClose,
  onCreated,
}: {
  project: Project;
  schedule?: Schedule;
  workspaces: { id: string; name?: string; agent_kind?: string }[];
  onClose: () => void;
  onCreated: () => void;
}) {
  const [name, setName] = useState(schedule?.name ?? ""),
    [workspace, setWorkspace] = useState(schedule?.workspace_id ?? ""),
    [path, setPath] = useState(schedule?.instruction_path ?? "");
  const [runAt, setRunAt] = useState(() =>
      schedule ? dateInput(schedule.run_at) : localDate(),
    ),
    [interval, setInterval] = useState(schedule?.interval_seconds ?? 0);
  const key = useRequestKey();
  const mutation = useMutation({
    mutationFn: () => {
      const payload = {
        project_id: project.id,
        workspace_id: workspace,
        name: name.trim(),
        instruction_path: path.trim(),
        provider:
          workspaces.find((w) => w.id === workspace)?.agent_kind === "hermes"
            ? "hermes"
            : "codex",
        run_at:
          schedule && runAt === dateInput(schedule.run_at)
            ? schedule.run_at
            : new Date(runAt).toISOString(),
        interval_seconds: interval,
      };
      return schedule
        ? schedulingApi.update(
            schedule.id,
            payload,
            schedule.revision ?? 0,
            key(payload),
          )
        : schedulingApi.create(payload, key(payload));
    },
    onSuccess: onCreated,
  });
  return (
    <Modal
      title={schedule ? "Edit schedule" : "Create schedule"}
      onClose={() => {
        if (!mutation.isPending) onClose();
      }}
    >
      <Form onSubmit={() => mutation.mutate()}>
        <div className="form-project">
          Project <strong>{project.name}</strong>
        </div>
        <label>
          Schedule name
          <Input
            autoFocus
            required
            value={name}
            onChange={(event) => setName(event.target.value)}
            maxLength={80}
            placeholder="Nightly project checks"
          />
        </label>
        <label>
          Workspace
          <NativeSelect
            aria-label="Workspace"
            required
            value={workspace}
            onChange={(event) => setWorkspace(event.target.value)}
          >
            <option value="">Choose a workspace</option>
            {schedule &&
              !workspaces.some((w) => w.id === schedule.workspace_id) && (
                <option value={schedule.workspace_id} disabled>
                  Worker unavailable — choose a replacement
                </option>
              )}
            {workspaces.map((item) => (
              <option key={item.id} value={item.id}>
                {item.name || item.id}
              </option>
            ))}
          </NativeSelect>
        </label>
        <label>
          Instruction file
          <Input
            aria-label="Instruction file"
            required
            maxLength={512}
            value={path}
            onChange={(event) => setPath(event.target.value)}
            placeholder="tasks/nightly-checks.md"
          />
          <small>
            Create this file in the workspace before the schedule runs. The
            scheduler does not store its contents.
          </small>
        </label>
        <div className="form-columns">
          <label>
            First run (your local time)
            <Input
              required
              type="datetime-local"
              value={runAt}
              onChange={(event) => setRunAt(event.target.value)}
            />
          </label>
          <label>
            Repeat
            <NativeSelect
              aria-label="Repeat"
              value={interval}
              onChange={(event) => setInterval(Number(event.target.value))}
            >
              {![0, 300, 3600, 86400].includes(interval) && (
                <option value={interval}>{frequency(interval)}</option>
              )}
              <option value={0}>One time</option>
              <option value={300}>Every 5 minutes</option>
              <option value={3600}>Every hour</option>
              <option value={86400}>Every day</option>
            </NativeSelect>
          </label>
        </div>
        <p className="form-note">
          Uses the selected worker’s native Codex or Hermes configuration.
          {schedule &&
            " Saving preserves whether this schedule is enabled or disabled. Changing the first run or repeat interval resets its next occurrence."}
        </p>
        {mutation.isError && <ErrorNotice error={mutation.error} />}
        <div className="form-actions">
          <Button
            variant="outline"
            type="button"
            onClick={onClose}
            disabled={mutation.isPending}
          >
            Cancel
          </Button>
          <Button
            variant="default"
            disabled={
              mutation.isPending || !workspaces.some((w) => w.id === workspace)
            }
          >
            {mutation.isPending
              ? "Saving…"
              : schedule
                ? "Save changes"
                : "Create schedule"}
          </Button>
        </div>
      </Form>
    </Modal>
  );
}

function DeleteSchedule({
  schedule,
  onClose,
  onDeleted,
}: {
  schedule: Schedule;
  onClose: () => void;
  onDeleted: () => void;
}) {
  const key = useRequestKey();
  const mutation = useMutation({
    mutationFn: () =>
      schedulingApi.remove(
        schedule.id,
        schedule.revision ?? 0,
        key({ id: schedule.id, revision: schedule.revision }),
      ),
    onSuccess: onDeleted,
  });
  return (
    <Modal
      title="Delete schedule"
      onClose={() => {
        if (!mutation.isPending) onClose();
      }}
    >
      <p>
        Delete <strong>{schedule.name}</strong>?
      </p>
      <p className="text-sm text-muted-foreground">
        Future occurrences stop and runs awaiting dispatch are cancelled. Work
        already being dispatched or running continues. Run history is retained.
      </p>
      {mutation.isError && <ErrorNotice error={mutation.error} />}
      <div className="form-actions">
        <Button
          variant="outline"
          disabled={mutation.isPending}
          onClick={onClose}
        >
          Cancel
        </Button>
        <Button
          variant="destructive"
          disabled={mutation.isPending}
          onClick={() => mutation.mutate()}
        >
          {mutation.isPending ? "Deleting…" : "Delete schedule"}
        </Button>
      </div>
    </Modal>
  );
}
