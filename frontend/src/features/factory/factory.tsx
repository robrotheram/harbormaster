import { useRef, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/native-select";
import { ErrorNotice } from "@/shared/components";
import { request } from "@/shared/http";
import { workspacesApi, workspaceOptions } from "@/features/workspaces/api";
import type { Project } from "@/shared/types";

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
  const [output, setOutput] = useState("");
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
        setOutput(atob(data.content));
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
      if ((operation === "revoke" || operation === "release") && retained)
        await request(
          `/workspaces/${retained.workspace}/services/${retained.service.id}/${operation}`,
          {},
        );
      await client.invalidateQueries({ queryKey: ["factory", project.id] });
    },
  });
  const run = (operation: string, id?: string) => {
    if (operation === "open") {
      previewTab.current = window.open("about:blank", "_blank");
      if (previewTab.current) previewTab.current.opener = null;
    }
    action.mutate({ operation, id });
  };
  const service = status.data || retained?.service;
  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">Jobs and previews</h2>
        <p className="text-sm text-muted-foreground">
          Run an approved recipe, inspect archived evidence, and retain a
          gallery in its own environment.
        </p>
      </div>
      <div className="flex flex-wrap items-end gap-3">
        <label>
          Workspace
          <NativeSelect
            aria-label="Factory workspace"
            value={workspace}
            onChange={(e) => {
              setWorkspace(e.target.value);
              jobDelivery.current = undefined;
              setPromotion(undefined);
            }}
          >
            <option value="">Choose workspace</option>
            {workspaces.data?.map((w) => (
              <option key={w.id} value={w.id}>
                {w.name}
              </option>
            ))}
          </NativeSelect>
        </label>
        <label>
          Recipe
          <NativeSelect
            aria-label="Recipe"
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
          disabled={!workspace || action.isPending}
          onClick={() => run("start")}
        >
          Start job
        </Button>
      </div>
      {action.isError && <ErrorNotice error={action.error} />}
      {jobs.isError && <ErrorNotice error={jobs.error} />}
      <section className="space-y-3">
        <h3 className="font-semibold">Retained environments</h3>
        {workspaces.data
          ?.filter((w) => w.retained_preview)
          .map((w) => (
            <Button
              key={w.id}
              variant="outline"
              onClick={() =>
                setRetained({
                  workspace: w.id,
                  service: {
                    id: w.retained_preview!.service_id,
                    state: "unknown",
                    expires_at: w.retained_preview!.expires_at,
                  },
                })
              }
            >
              {w.name} · {w.state}
            </Button>
          ))}
      </section>
      {output && (
        <pre className="overflow-auto whitespace-pre-wrap border p-4">
          {output}
        </pre>
      )}
      <section className="space-y-3">
        <h3 className="font-semibold">Recipe jobs</h3>
        {jobs.data?.items.map((j) => (
          <div
            key={j.id}
            className="flex flex-wrap items-center justify-between gap-3 border-b py-3"
          >
            <div>
              <p>
                {j.template} · {j.state}
              </p>
              <p className="text-sm text-muted-foreground">
                {j.id} {j.error_code}
              </p>
            </div>
            {["running", "cancel_requested"].includes(j.state) && (
              <Button
                variant="outline"
                disabled={action.isPending}
                onClick={() => run("cancel", j.id)}
              >
                Cancel job
              </Button>
            )}
          </div>
        ))}
      </section>
      <section className="space-y-3">
        <h3 className="font-semibold">Archived evidence</h3>
        {evidence.isError && <ErrorNotice error={evidence.error} />}{" "}
        {evidence.data?.items.map((a) => (
          <div
            key={a.id}
            className="flex flex-wrap items-center justify-between gap-3 border-b py-3"
          >
            <div>
              <p>{a.source_path}</p>
              <p className="break-all text-xs text-muted-foreground">
                {a.size_bytes} bytes · SHA-256 {a.sha256}
              </p>
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
      {previewURL && (
        <a href={previewURL} target="_blank" rel="noreferrer noopener">
          Open retained preview in a new tab
        </a>
      )}
      {retained && (
        <section className="space-y-3">
          <h3 className="font-semibold">Retained preview</h3>
          {service?.state !== "active" && (
            <Button
              variant="outline"
              onClick={() => {
                setPromotion(undefined);
                setRetained(undefined);
                setPreviewURL("");
              }}
            >
              Prepare another preview
            </Button>
          )}
          <p>
            {service?.state} · expires{" "}
            {service && new Date(service.expires_at * 1000).toLocaleString()}
          </p>
          <p className="text-sm text-muted-foreground">
            Environment {retained.workspace}
          </p>
          {status.isError && <ErrorNotice error={status.error} />}
          <div className="flex gap-3">
            <Button
              disabled={action.isPending || service?.state !== "active"}
              onClick={() => run("open")}
            >
              Open isolated preview
            </Button>
            <Button
              variant="outline"
              disabled={action.isPending || service?.state !== "active"}
              onClick={() => run("revoke")}
            >
              Revoke preview
            </Button>
            <Button
              variant="outline"
              disabled={action.isPending || service?.state === "active"}
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
