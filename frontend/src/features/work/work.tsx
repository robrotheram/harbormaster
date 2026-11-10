import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/native-select";
import {
  Empty,
  ErrorNotice,
  Form,
  Modal,
  Observation,
  State,
  useRequestKey,
} from "@/shared/components";
import { keys } from "@/shared/query-keys";
import { date, label } from "@/shared/format";
import type { Project } from "@/shared/types";
import { workspaceOptions } from "@/features/workspaces/api";
import {
  workApi,
  workKeys,
  workOptions,
  workActions,
  evidenceURL,
  type WorkCreate,
} from "./api";

export function Work({ project }: { project: Project }) {
  return <WorkProject key={project.id} project={project} />;
}
function WorkProject({ project }: { project: Project }) {
  const query = useQuery(workOptions(project.id));
  const [selected, select] = useState("");
  const [creating, create] = useState(false);
  return (
    <>
      <div className="page-title">
        <div>
          <h1>Work</h1>
          <p>
            Run software checks and research from recorded specifications.
            Review retained evidence before accepting results.
          </p>
        </div>
        <Button onClick={() => create(true)}>Create work item</Button>
      </div>
      <div className="list-toolbar">
        <span>{query.data?.length ?? "—"} work items</span>
        <Observation
          at={query.dataUpdatedAt}
          fetching={query.isFetching}
          error={query.isError}
        />
      </div>
      {query.isError && <ErrorNotice error={query.error} />}
      {query.isPending ? (
        <p role="status">Loading work…</p>
      ) : query.data?.length ? (
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                <th>Specification</th>
                <th>Kind</th>
                <th>State</th>
                <th>Updated</th>
              </tr>
            </thead>
            <tbody>
              {query.data.map((item) => (
                <tr key={item.id}>
                  <td>
                    <Button variant="link" onClick={() => select(item.id)}>
                      {item.specification_path}
                    </Button>
                    <span className="detail">{item.id}</span>
                  </td>
                  <td>{label(item.kind)}</td>
                  <td>
                    <State value={item.state} />
                  </td>
                  <td>{date(item.updated_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : query.data ? (
        <Empty title="No work items yet">
          Write an instruction or select a specification in this project's
          workspace.
        </Empty>
      ) : null}
      {creating && (
        <Create
          project={project.id}
          close={() => create(false)}
          created={(id) => {
            create(false);
            select(id);
          }}
        />
      )}
      {selected && (
        <Details
          key={selected}
          project={project.id}
          id={selected}
          close={() => select("")}
        />
      )}
    </>
  );
}
function Create({
  project,
  close,
  created,
}: {
  project: string;
  close: () => void;
  created: (id: string) => void;
}) {
  const workspaces = useQuery(workspaceOptions(project));
  const [workspace, setWorkspace] = useState("");
  const [mode, setMode] = useState("inline");
  const [instruction, setInstruction] = useState("");
  const [path, setPath] = useState("");
  const [digest, setDigest] = useState("");
  const [artifacts, setArtifacts] = useState("");
  const [kind, setKind] = useState<WorkCreate["kind"]>("software");
  const key = useRequestKey(),
    client = useQueryClient();
  const mutation = useMutation({
    mutationFn: () => {
      const payload = {
        project_id: project,
        workspace_id: workspace,
        ...(mode === "inline"
          ? { instruction }
          : {
              specification_path: path.trim(),
              specification_digest: digest.trim(),
            }),
        kind,
        artifact_paths: artifacts
          .split(/\r?\n/)
          .map((p) => p.trim())
          .filter(Boolean),
      };
      return workApi.create(payload, key(payload));
    },
    onSuccess: async (item) => {
      await Promise.all([
        client.invalidateQueries({ queryKey: workKeys.list(project) }),
        client.invalidateQueries({ queryKey: keys.workspaces(project) }),
      ]);
      created(item.id);
    },
  });
  return (
    <Modal title="Create work item" onClose={close}>
      <Form onSubmit={() => mutation.mutate()}>
        <p className="form-note">
          Save a private instruction or reference an existing file. Running work
          is a separate action.
        </p>
        {workspaces.isError && <ErrorNotice error={workspaces.error} />}
        <label>
          Input mode
          <NativeSelect value={mode} onChange={(e) => setMode(e.target.value)}>
            <option value="inline">Write instruction</option>
            <option value="file">Existing file</option>
          </NativeSelect>
        </label>
        {mode === "inline" && (
          <label>
            Instruction
            <Textarea
              required
              value={instruction}
              onChange={(e) => setInstruction(e.target.value)}
            />
          </label>
        )}
        <label>
          Workspace
          <NativeSelect
            required={mode === "file"}
            value={workspace}
            onChange={(e) => setWorkspace(e.target.value)}
          >
            <option value="">Create approved Codex worker</option>
            {workspaces.data?.map((w) => (
              <option key={w.id} value={w.id}>
                {w.name}
              </option>
            ))}
          </NativeSelect>
        </label>
        <label>
          Kind
          <NativeSelect
            value={kind}
            onChange={(e) => setKind(e.target.value as WorkCreate["kind"])}
          >
            <option value="software">Software</option>
            <option value="research">Research</option>
            <option value="digest">Digest</option>
          </NativeSelect>
        </label>
        {mode === "file" && (
          <>
            <label>
              Specification path
              <Input
                required
                value={path}
                onChange={(e) => setPath(e.target.value)}
                placeholder="specifications/gallery.md"
              />
            </label>
            <label>
              Specification digest
              <Input
                required
                value={digest}
                onChange={(e) => setDigest(e.target.value)}
              />
            </label>
          </>
        )}
        <label>
          Retained output paths
          <Textarea
            value={artifacts}
            onChange={(e) => setArtifacts(e.target.value)}
            placeholder="outputs/report.json"
          />
          <span className="form-note">
            One workspace file per line, up to 32. Outputs are published for
            each attempt.
          </span>
        </label>
        {mutation.isError && <ErrorNotice error={mutation.error} />}
        <div className="form-actions">
          <Button type="button" variant="outline" onClick={close}>
            Cancel
          </Button>
          <Button
            disabled={
              mutation.isPending ||
              (mode === "inline"
                ? !instruction ||
                  new TextEncoder().encode(instruction).length > 65536
                : !workspace || !path.trim() || !digest.trim())
            }
          >
            Create draft
          </Button>
        </div>
      </Form>
    </Modal>
  );
}
function Details({
  project,
  id,
  close,
}: {
  project: string;
  id: string;
  close: () => void;
}) {
  const client = useQueryClient();
  const query = useQuery({
    queryKey: workKeys.detail(project, id),
    queryFn: ({ signal }) => workApi.detail(project, id, signal),
    refetchInterval: 5000,
  });
  const attempts = useQuery({
    queryKey: workKeys.attempts(project, id),
    queryFn: ({ signal }) => workApi.attempts(project, id, signal),
    enabled: !!query.data && !query.isError,
    refetchInterval: 5000,
  });
  const [runKey, setRunKey] = useState(() => crypto.randomUUID());
  const mutation = useMutation({
    mutationFn: (action: "run" | "cancel" | "accept" | "reject") =>
      action === "run"
        ? workApi.run(project, id, runKey)
        : action === "cancel"
          ? workApi.cancel(project, id)
          : workApi.review(
              project,
              id,
              action === "accept",
              query.data?.latest_attempt_id,
            ),
    onSuccess: async () => {
      setRunKey(crypto.randomUUID());
      await client.invalidateQueries({ queryKey: workKeys.list(project) });
    },
  });
  const item = query.isError ? undefined : query.data;
  const actions = item && workActions(item.state);
  return (
    <Modal title="Work details" onClose={close} wide>
      {query.isError && <ErrorNotice error={query.error} />}
      {query.isPending && <p role="status">Loading work details…</p>}
      {item && (
        <div className="space-y-5">
          <div>
            <h2 className="break-all">{item.specification_path}</h2>
            <State value={item.state} />
            <p className="detail break-all">
              {item.id} · {item.kind}
            </p>
          </div>
          <p className="break-all">
            Specification digest: {item.specification_digest}
          </p>
          <Link
            to="/projects/$projectId/$section"
            params={{ projectId: project, section: "workspaces" }}
          >
            Open workspaces
          </Link>
          <p className="detail">Workspace: {item.workspace_id}</p>
          {item.error_code && <p role="alert">{label(item.error_code)}</p>}
          {item.state.toLowerCase() === "blocked" && (
            <p role="status">
              Work is blocked. Inspect attempt diagnostics before retrying.
            </p>
          )}
          {mutation.isError && <ErrorNotice error={mutation.error} />}
          <div className="flex flex-wrap gap-2">
            {actions?.run && (
              <Button
                disabled={mutation.isPending || query.isFetching}
                onClick={() => mutation.mutate("run")}
              >
                {item.latest_attempt_id ? "Retry with new attempt" : "Run work"}
              </Button>
            )}
            {actions?.cancel && (
              <Button
                variant="outline"
                disabled={mutation.isPending || query.isFetching}
                onClick={() => mutation.mutate("cancel")}
              >
                Cancel work
              </Button>
            )}
            {actions?.review && (
              <>
                <p className="w-full">
                  Inspect the evidence below before reviewing this result.
                </p>
                <Button
                  disabled={mutation.isPending || query.isFetching}
                  onClick={() => mutation.mutate("accept")}
                >
                  Accept result
                </Button>
                <Button
                  variant="outline"
                  disabled={mutation.isPending || query.isFetching}
                  onClick={() => mutation.mutate("reject")}
                >
                  Request changes
                </Button>
              </>
            )}
          </div>
          <h2>Attempts and evidence</h2>
          {attempts.isError && <ErrorNotice error={attempts.error} />}
          {attempts.isPending ? (
            <p role="status">Loading attempts…</p>
          ) : !attempts.isError && attempts.data?.length ? (
            attempts.data.map((attempt) => (
              <section
                key={attempt.id}
                className="space-y-2 rounded-lg border p-4"
              >
                <h3 className="break-all">
                  {attempt.id}
                  {attempt.id === item.latest_attempt_id ? " (latest)" : ""}
                </h3>
                <State value={attempt.state} />
                <p className="detail">
                  Updated {date(attempt.updated_at)} · Created{" "}
                  {date(attempt.created_at)}
                </p>
                <p className="detail break-all">
                  Task: {attempt.task_id || "Awaiting task acceptance"}
                </p>
                {attempt.error_code && (
                  <p role="alert">{label(attempt.error_code)}</p>
                )}
                {attempt.artifacts?.length ? (
                  <ul>
                    {attempt.artifacts.map((artifact, i) => (
                      <li key={artifact.id || i}>
                        {evidenceURL(artifact.content_url) ? (
                          <a
                            href={evidenceURL(artifact.content_url)}
                            target="_blank"
                            rel="noreferrer"
                          >
                            {artifact.name || artifact.kind || "Evidence"}
                          </a>
                        ) : (
                          <span>
                            {artifact.name || artifact.kind || "Evidence"} —
                            content link unavailable
                          </span>
                        )}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p>No retained evidence reported for this attempt.</p>
                )}
              </section>
            ))
          ) : (
            !attempts.isError && (
              <p>No attempts yet. Run this work to create an attempt.</p>
            )
          )}
        </div>
      )}
    </Modal>
  );
}
