import { useRef, useState } from "react";
import { Link } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/native-select";
import { ErrorNotice, State, Modal, Observation } from "@/shared/components";
import { APIError, request } from "@/shared/http";
import { workspacesApi, workspaceOptions } from "@/features/workspaces/api";
import { bytes } from "@/shared/format";
import type { Project } from "@/shared/types";

const recipeNames: Record<string, string> = {
  "research-v1": "Research",
  "gogallery-v1": "GoGallery",
};
type Job = { id: string; template: string; state: string; error_code: string };
type Artifact = {
  id: string;
  source_path: string;
  sha256: string;
  size_bytes: number;
};
type Service = { id: string; state: string; expires_at: number };
type Retained = { workspace: string; service: Service };
export function Factory({ project }: { project: Project }) {
  // Project switches unmount all workspace-bound drafts and queries.
  return <FactoryProject key={project.id} project={project} />;
}
function FactoryProject({ project }: { project: Project }) {
  const client = useQueryClient();
  const previewTab = useRef<Window | null>(null);
  const jobDelivery = useRef<string | undefined>(undefined);
  const [previewURL, setPreviewURL] = useState("");
  const workspaces = useQuery(workspaceOptions(project.id));
  const [output, setOutput] = useState<{ name: string; content: string }>();
  const [workspace, setWorkspace] = useState("");
  const [template, setTemplate] = useState("research-v1");
  const [retained, setRetained] = useState<Retained>();
  const [promotion, setPromotion] = useState<{
    workspace: string;
    artifact: string;
    key: string;
  }>();
  const base = `/workspaces/${encodeURIComponent(workspace)}`;
  const jobs = useQuery({
    queryKey: ["factory", project.id, workspace, "jobs"],
    enabled: !!workspace,
    queryFn: ({ signal }) =>
      request<{ items: Job[] }>(base + "/recipe-jobs", undefined, signal),
    refetchInterval: 3000,
  });
  const evidence = useQuery({
    queryKey: ["factory", project.id, workspace, "evidence"],
    enabled: !!workspace,
    queryFn: ({ signal }) =>
      request<{ items: Artifact[] }>(base + "/artifacts", undefined, signal),
    refetchInterval: 5000,
  });
  const status = useQuery({
    queryKey: [
      "factory",
      project.id,
      retained?.workspace,
      retained?.service.id,
    ],
    enabled: !!retained,
    queryFn: () =>
      request<Service>(
        `/workspaces/${retained!.workspace}/services/${retained!.service.id}`,
      ),
    refetchInterval: 3000,
  });
  const action = useMutation({
    mutationFn: async ({
      operation,
      id,
    }: {
      operation: string;
      id?: string;
    }) => {
      if (operation === "read" && id) {
        const data = await request<{ content: string }>(
          base + `/artifacts/${id}/content`,
        );
        try {
          const content = new TextDecoder("utf-8", { fatal: true }).decode(
            Uint8Array.from(atob(data.content), (character) =>
              character.charCodeAt(0),
            ),
          );
          setOutput({
            name:
              evidence.data?.items.find((item) => item.id === id)
                ?.source_path || "Saved output",
            content,
          });
        } catch {
          throw new Error(
            "This file is not readable UTF-8 text. Inspect the original file in the worker instead.",
          );
        }
      }
      if (operation === "start") {
        jobDelivery.current ??= crypto.randomUUID();
        await request(base + "/recipe-jobs", {
          template,
          idempotency_key: jobDelivery.current,
        });
        jobDelivery.current = undefined;
      }
      if (operation === "cancel")
        await request(base + `/recipe-jobs/${id}/cancel`, {});
      if (operation === "retain" && id) {
        let delivery = promotion;
        if (!delivery || delivery.artifact !== id) {
          const key = crypto.randomUUID();
          const target = await workspacesApi.create(
            {
              project_id: project.id,
              name: "Retained gallery",
              agent_kind: "codex",
            },
            key,
          );
          delivery = { workspace: target.id, artifact: id, key };
          setPromotion(delivery);
        }
        const service = await request<Service>(base + "/retained-previews", {
          retained_workspace_id: delivery.workspace,
          artifact_id: id,
          ttl_seconds: 3600,
          idempotency_key: delivery.key,
        });
        setRetained({ workspace: delivery.workspace, service });
      }
      if (operation === "open" && retained) {
        const result = await request<{ preview_url: string }>(
          `/workspaces/${retained.workspace}/services/${retained.service.id}/connect`,
          {},
        );
        setPreviewURL(result.preview_url);
        if (previewTab.current)
          previewTab.current.location.replace(result.preview_url);
      }
      if ((operation === "revoke" || operation === "release") && retained) {
        await request(
          `/workspaces/${retained.workspace}/services/${retained.service.id}/${operation}`,
          {},
        );
        setPreviewURL("");
        if (operation === "release") {
          setRetained(undefined);
          setPromotion(undefined);
        }
      }
      await client.invalidateQueries({ queryKey: ["factory", project.id] });
      if (["retain", "release"].includes(operation)) await workspaces.refetch();
    },
    onError: (_error, variables) => {
      if (variables.operation === "open") {
        previewTab.current?.close();
        previewTab.current = null;
        setPreviewURL("");
      }
    },
  });
  const run = (operation: string, id?: string) => {
    if (operation === "read") setOutput(undefined);
    if (operation === "open") {
      previewTab.current = window.open("about:blank", "_blank");
      if (previewTab.current) previewTab.current.opener = null;
    }
    action.mutate({ operation, id });
  };
  const service = status.data || retained?.service;
  const serviceKnown = status.isSuccess && !status.isError;
  const selectedWorker = workspaces.data?.find(
    (worker) => worker.id === workspace,
  );
  return (
    <div className="space-y-4">
      <div className="page-title">
        <div>
          <h1>Outputs & previews</h1>
        </div>
      </div>
      {workspaces.isError && (
        <ErrorNotice
          error={workspaces.error}
          onRetry={() => void workspaces.refetch()}
          retrying={workspaces.isFetching}
        />
      )}
      <div className="output-controls">
        <label className="min-w-0 flex-[2_1_220px] text-xs">
          Worker
          <NativeSelect
            size="sm"
            className="text-xs"
            aria-label="Worker"
            disabled={
              action.isPending || workspaces.isPending || workspaces.isError
            }
            value={workspace}
            onChange={(e) => {
              setWorkspace(e.target.value);
              setOutput(undefined);
              action.reset();
              jobDelivery.current = undefined;
              setPromotion(undefined);
            }}
          >
            <option value="">Choose a worker</option>
            {workspaces.data?.map((w) => (
              <option key={w.id} value={w.id}>
                {w.name || w.id}
              </option>
            ))}
          </NativeSelect>
        </label>
        <label className="min-w-0 flex-[1_1_140px] text-xs">
          Recipe
          <NativeSelect
            size="sm"
            className="text-xs"
            aria-label="Recipe"
            disabled={action.isPending}
            value={template}
            onChange={(e) => {
              setTemplate(e.target.value);
              jobDelivery.current = undefined;
            }}
          >
            <option value="research-v1">Research</option>
            <option value="gogallery-v1">GoGallery</option>
          </NativeSelect>
        </label>
        <Button
          disabled={!selectedWorker || workspaces.isError || action.isPending}
          onClick={() => run("start")}
        >
          {action.isPending && action.variables?.operation === "start"
            ? "Starting job…"
            : "Start job"}
        </Button>
      </div>
      <p className="text-xs leading-5 text-muted-foreground">
        A recipe runs a predefined task and saves its output files. Starting a
        job starts the selected worker if needed. For custom instructions and
        ongoing changes,{" "}
        <Link
          className="underline"
          to="/projects/$projectId/$section"
          params={{ projectId: project.id, section: "work" }}
        >
          open Work
        </Link>
        .
      </p>
      {action.isError && <ErrorNotice error={action.error} />}
      <section className="output-section">
        <h2>Saved previews</h2>
        {workspaces.isPending ? (
          <p role="status">Loading saved previews…</p>
        ) : !workspaces.isError &&
          !workspaces.data?.some((w) => w.retained_preview) ? (
          <p>
            No saved previews yet. Run the GoGallery recipe, then choose Retain
            gallery for 1 hour on its saved archive. This creates a separate
            worker for the preview.
          </p>
        ) : null}
        {workspaces.data
          ?.filter((w) => w.retained_preview)
          .map((w) => (
            <Button
              key={w.id}
              variant="outline"
              disabled={action.isPending}
              onClick={() => {
                setPreviewURL("");
                action.reset();
                setRetained({
                  workspace: w.id,
                  service: {
                    id: w.retained_preview!.service_id,
                    state: "unknown",
                    expires_at: w.retained_preview!.expires_at,
                  },
                });
              }}
            >
              {w.name} · {w.state}
            </Button>
          ))}
      </section>
      {output && (
        <Modal title="Saved output" onClose={() => setOutput(undefined)} wide>
          <p className="break-all text-sm text-muted-foreground">
            {output.name}
          </p>
          <pre className="max-h-[60dvh] overflow-auto whitespace-pre-wrap break-words rounded-md border p-4 text-xs leading-6">
            {output.content}
          </pre>
        </Modal>
      )}
      <section className="output-section">
        <h2>Recipe jobs</h2>
        {jobs.isError && (
          <ErrorNotice
            error={jobs.error}
            onRetry={() => void jobs.refetch()}
            retrying={jobs.isFetching}
          />
        )}
        {!workspace ? (
          <p>Choose a worker above to see its jobs and outputs.</p>
        ) : jobs.isPending ? (
          <p role="status">Loading jobs…</p>
        ) : !jobs.isError && !jobs.data?.items.length ? (
          <p>
            No recipe jobs on this worker yet. Choose a recipe and select Start
            job.
          </p>
        ) : null}
        {jobs.data?.items.map((j) => (
          <div
            key={j.id}
            className="flex flex-wrap items-center justify-between gap-3 border-b py-3"
          >
            <div className="min-w-0">
              <p className="flex flex-wrap items-center gap-2 text-xs">
                {recipeNames[j.template] || j.template}{" "}
                <State value={j.state} />
              </p>
              <p className="mt-1 break-all text-[11px] text-muted-foreground">
                Job {j.id}
              </p>
            </div>
            {j.error_code && (
              <ErrorNotice error={new APIError(j.error_code, 0)} />
            )}
            {["running", "cancel_requested"].includes(j.state) && (
              <Button
                variant="outline"
                disabled={action.isPending || j.state === "cancel_requested"}
                onClick={() => run("cancel", j.id)}
              >
                {j.state === "cancel_requested"
                  ? "Cancellation requested"
                  : "Cancel job"}
              </Button>
            )}
          </div>
        ))}
      </section>
      <section className="output-section">
        <h2>Saved output files</h2>
        {!workspace ? (
          <p>Saved outputs from the selected worker will appear here.</p>
        ) : evidence.isPending ? (
          <p role="status">Loading saved files…</p>
        ) : !evidence.isError && !evidence.data?.items.length ? (
          <p>
            No saved files yet. Completed recipe jobs can produce reports and
            archives to inspect here.
          </p>
        ) : null}
        {evidence.isError && (
          <ErrorNotice
            error={evidence.error}
            onRetry={() => void evidence.refetch()}
            retrying={evidence.isFetching}
          />
        )}{" "}
        {evidence.data?.items.map((a) => (
          <div
            key={a.id}
            className="flex flex-wrap items-center justify-between gap-3 border-b py-3"
          >
            <div className="min-w-0">
              <p className="break-all text-xs">{a.source_path}</p>
              <p className="break-all text-xs text-muted-foreground">
                {bytes(a.size_bytes)}
              </p>
              <details className="mt-1 text-xs text-muted-foreground">
                <summary className="cursor-pointer">File verification</summary>
                <p className="mt-1 break-all font-mono">SHA-256 {a.sha256}</p>
              </details>
            </div>
            {!a.source_path.endsWith(".tar") && a.size_bytes <= 1048576 && (
              <Button
                variant="outline"
                disabled={action.isPending}
                onClick={() => run("read", a.id)}
              >
                Read output
              </Button>
            )}
            {a.source_path.endsWith("gallery.tar") && (
              <Button
                disabled={action.isPending}
                onClick={() => run("retain", a.id)}
              >
                Retain gallery for 1 hour
              </Button>
            )}
          </div>
        ))}
      </section>
      {previewURL && serviceKnown && service?.state === "active" && (
        <a href={previewURL} target="_blank" rel="noreferrer noopener">
          Open retained preview in a new tab
        </a>
      )}
      {retained && (
        <section className="output-section">
          <h2>Retained preview</h2>
          {serviceKnown && service?.state !== "active" && (
            <Button
              variant="outline"
              disabled={action.isPending}
              onClick={() => {
                setPromotion(undefined);
                setRetained(undefined);
                setPreviewURL("");
              }}
            >
              Prepare another preview
            </Button>
          )}
          <div className="flex flex-wrap items-center gap-2">
            <State
              value={
                status.isError
                  ? "unavailable"
                  : status.isPending
                    ? "checking"
                    : service?.state || "unknown"
              }
            />
            <Observation
              at={status.dataUpdatedAt}
              fetching={status.isFetching}
              error={status.isError}
            />
          </div>
          {serviceKnown && service && (
            <p>
              Expires {new Date(service.expires_at * 1000).toLocaleString()}
            </p>
          )}
          <p>
            Revoke preview ends access. Release environment stops its compute
            after access has ended; it does not delete the worker.
          </p>
          <p className="break-all text-xs text-muted-foreground">
            Environment {retained.workspace}
          </p>
          {status.isError && (
            <ErrorNotice
              error={status.error}
              onRetry={() => void status.refetch()}
              retrying={status.isFetching}
            />
          )}
          <div className="flex flex-wrap gap-3">
            <Button
              disabled={
                action.isPending || !serviceKnown || service?.state !== "active"
              }
              onClick={() => run("open")}
            >
              Open isolated preview
            </Button>
            <Button
              variant="outline"
              disabled={
                action.isPending || !serviceKnown || service?.state !== "active"
              }
              onClick={() => run("revoke")}
            >
              Revoke preview
            </Button>
            <Button
              variant="outline"
              disabled={
                action.isPending ||
                !serviceKnown ||
                !["revoked", "expired"].includes(service?.state || "")
              }
              onClick={() => run("release")}
            >
              Release environment
            </Button>
          </div>
        </section>
      )}
    </div>
  );
}
