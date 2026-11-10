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
import type { Project } from "@/shared/types";

export function Schedules({ project }: { project: Project }) {
  const client = useQueryClient();
  const query = useQuery(scheduleOptions(project.id));
  const workspaces = useQuery({
    queryKey: keys.workspaces(project.id),
    queryFn: ({ signal }) => workspacesApi.list(project.id, signal),
  });
  const [create, setCreate] = useState(false),
    [message, setMessage] = useState("");
  const mutation = useMutation({
    mutationFn: ({
      id,
      action,
    }: {
      id: string;
      action: "run-now" | "enable" | "disable";
    }) => schedulingApi.action(id, action, crypto.randomUUID()),
    onSuccess: (_, variables) => {
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
          <p>Wake a workspace and run a project-local instruction file.</p>
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
        <Observation
          at={query.dataUpdatedAt}
          fetching={query.isFetching}
          error={query.isError}
        />
      </div>
      {query.isError && <ErrorNotice error={query.error} />}
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
          {query.data.map((schedule) => (
            <article className="schedule-row" key={schedule.id}>
              <div className="schedule-main">
                <div>
                  <h2>{schedule.name}</h2>
                  <p>
                    {workspaces.data?.find(
                      (w) => w.id === schedule.workspace_id,
                    )?.name || schedule.workspace_id}
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
              {schedule.last_error_code && (
                <p className="schedule-error">
                  Last issue: {label(schedule.last_error_code)}. Check the
                  workspace and provider before retrying.
                </p>
              )}
              <div className="row-actions">
                <Button
                  variant="outline"
                  onClick={() =>
                    mutation.mutate({ id: schedule.id, action: "run-now" })
                  }
                  disabled={mutation.isPending}
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
                  disabled={mutation.isPending}
                >
                  {schedule.enabled ? "Disable" : "Enable"}
                </Button>
              </div>
            </article>
          ))}
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
      <div className="explanation">
        <h2>One execution authority</h2>
        <p>
          These workbench schedules request native Codex tasks. Closing a client
          does not stop accepted work. Provider limits keep work queued or
          unavailable; no API-billing fallback is selected automatically.
        </p>
      </div>
      {create && (
        <ScheduleForm
          project={project}
          workspaces={workspaces.data || []}
          onClose={() => setCreate(false)}
          onCreated={() => {
            setCreate(false);
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
  const now = new Date(Date.now() + 60000);
  return new Date(now.valueOf() - now.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16);
}

function ScheduleForm({
  project,
  workspaces,
  onClose,
  onCreated,
}: {
  project: Project;
  workspaces: { id: string; name?: string; agent_kind?: string }[];
  onClose: () => void;
  onCreated: () => void;
}) {
  const [name, setName] = useState(""),
    [workspace, setWorkspace] = useState(""),
    [path, setPath] = useState("");
  const [runAt, setRunAt] = useState(localDate),
    [interval, setInterval] = useState(0);
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
        run_at: new Date(runAt).toISOString(),
        interval_seconds: interval,
      };
      return schedulingApi.create(payload, key(payload));
    },
    onSuccess: onCreated,
  });
  return (
    <Modal title="Create schedule" onClose={onClose}>
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
              <option value={0}>One time</option>
              <option value={300}>Every 5 minutes</option>
              <option value={3600}>Every hour</option>
              <option value={86400}>Every day</option>
            </NativeSelect>
          </label>
        </div>
        <p className="form-note">
          Uses the workspace's native Codex subscription configuration. Claude
          and Hermes scheduling are not enabled by this form.
        </p>
        {mutation.isError && <ErrorNotice error={mutation.error} />}
        <div className="form-actions">
          <Button variant="outline" type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="default" disabled={mutation.isPending}>
            {mutation.isPending ? "Creating…" : "Create schedule"}
          </Button>
        </div>
      </Form>
    </Modal>
  );
}
