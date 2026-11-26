import { useUserSession } from "@/features/auth/session";
import { memberOptions } from "@/features/work/responsibility";
import { ProjectAudit } from "@/features/history/project-audit";
import { ProjectMembers } from "./members";
import { useMutation, useQueryClient, useQuery } from "@tanstack/react-query";
import { Tabs } from "radix-ui";
import { ClipboardList, Settings2 } from "lucide-react";
import { projectsApi } from "./api";
import { keys } from "@/shared/query-keys";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useState } from "react";
import { ErrorNotice, Modal, Form } from "@/shared/components";
import { request } from "@/shared/http";
import type { Project } from "@/shared/types";

export function ProjectSettings({ project }: { project: Project }) {
  const search = useSearch({ from: "/projects/$projectId/$section" });
  const navigate = useNavigate();
  return (
    <div className="space-y-6">
      <div className="page-title">
        <h1>Project settings</h1>
      </div>
      <Tabs.Root
        value={search.tab === "audit" ? "audit" : "general"}
        onValueChange={(tab) =>
          void navigate({
            to: "/projects/$projectId/$section",
            params: { projectId: project.id, section: "settings" },
            search: {
              tab: tab === "audit" ? tab : undefined,
            },
          })
        }
      >
        <Tabs.List aria-label="Project settings" className="settings-tabs">
          <Tabs.Trigger value="general">
            <Settings2 aria-hidden="true" className="size-4 shrink-0" />
            General
          </Tabs.Trigger>
          <Tabs.Trigger value="audit">
            <ClipboardList aria-hidden="true" className="size-4 shrink-0" />
            Audit
          </Tabs.Trigger>
        </Tabs.List>
        <Tabs.Content
          value="general"
          className="pt-6 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <GeneralSettings key={project.id} project={project} />
        </Tabs.Content>
        <Tabs.Content
          value="audit"
          className="pt-6 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <ProjectAudit key={project.id} project={project} />
        </Tabs.Content>
      </Tabs.Root>
    </div>
  );
}
function GeneralSettings({ project }: { project: Project }) {
  const queryClient = useQueryClient();
  const session = useUserSession();
  const members = useQuery(memberOptions(project.id));
  const canManage =
    session.status === "local" ||
    (members.data?.can_manage === true && !members.isError);
  const navigate = useNavigate();
  const [name, setName] = useState(project.name);
  const [saved, setSaved] = useState(false);
  const validName =
    !!name.trim() &&
    new TextEncoder().encode(name.trim()).length <= 80 &&
    !/[\r\n\0]/.test(name);
  const rename = useMutation({
    mutationFn: () => projectsApi.rename(project.id, name.trim()),
    onSuccess: async (updated) => {
      setName(updated.name);
      setSaved(true);
      queryClient.setQueryData<Project[]>(keys.projects, (items) =>
        items?.map((p) => (p.id === updated.id ? updated : p)),
      );
      await queryClient.invalidateQueries({ queryKey: keys.projects });
    },
  });
  const [confirming, setConfirming] = useState(false);
  const [confirmation, setConfirmation] = useState("");
  const namespace = useMutation({
    mutationFn: () =>
      request(`/projects/${encodeURIComponent(project.id)}/namespace`, {}),
    onSuccess: () => queryClient.invalidateQueries(),
  });
  const deletion = useMutation({
    mutationFn: () =>
      request(`/projects/${encodeURIComponent(project.id)}/delete`, {
        confirm_id: project.id,
      }),
    onSuccess: async () => {
      await queryClient.cancelQueries({ queryKey: keys.project(project.id) });
      queryClient.removeQueries({ queryKey: keys.project(project.id) });
      queryClient.setQueryData<Project[]>(keys.projects, (old) =>
        old?.filter((p) => p.id !== project.id),
      );
      try {
        if (localStorage.getItem("workbench.last-project") === project.id)
          localStorage.removeItem("workbench.last-project");
      } catch {}
      await navigate({ to: "/" });
      await queryClient.invalidateQueries({ queryKey: keys.projects });
    },
  });
  return (
    <div className="space-y-4">
      {!canManage && !members.isPending && (
        <p className="text-sm text-muted-foreground">
          Project administrator access is required to rename or delete this
          project.
        </p>
      )}
      <section aria-labelledby="project-details" className="settings-panel">
        <h2 id="project-details" className="settings-panel-title">
          Project details
        </h2>
        <Form
          onSubmit={() => {
            if (
              canManage &&
              validName &&
              !rename.isPending &&
              name.trim() !== project.name
            )
              rename.mutate();
          }}
        >
          <label>
            Project name
            <Input
              value={name}
              onChange={(e) => {
                setName(e.target.value);
                setSaved(false);
              }}
              disabled={!canManage || rename.isPending}
              required
              aria-describedby="project-name-help"
            />
          </label>
          <p id="project-name-help" className="form-note">
            Choose a short, descriptive name. Renaming keeps your workers and
            history.
          </p>
          {!validName && name.trim() && (
            <p role="alert" className="text-sm text-destructive">
              Use a shorter name without line breaks.
            </p>
          )}
          {rename.isError && <ErrorNotice error={rename.error} />}
          {saved && <p role="status">Project renamed.</p>}
          <Button
            disabled={
              !canManage ||
              !validName ||
              name.trim() === project.name ||
              rename.isPending
            }
          >
            {rename.isPending ? "Saving…" : "Save name"}
          </Button>
        </Form>
        <dl className="project-setting-facts">
          <div>
            <dt className="text-muted-foreground">Project ID</dt>
            <dd className="identifier">{project.id}</dd>
          </div>
          {project.deployment_namespace && (
            <div>
              <dt className="text-muted-foreground">Application namespace</dt>
              <dd className="identifier">{project.deployment_namespace}</dd>
              <dd className="text-sm text-muted-foreground">
                {project.namespace_ready
                  ? "Ready for application deployments. The agent workspace is shared."
                  : "Namespace setup is incomplete."}
              </dd>
              {!project.namespace_ready && (
                <Button
                  onClick={() => namespace.mutate()}
                  disabled={namespace.isPending}
                >
                  {namespace.isPending
                    ? "Preparing namespace…"
                    : "Retry namespace setup"}
                </Button>
              )}
              {namespace.isError && <ErrorNotice error={namespace.error} />}
            </div>
          )}
        </dl>
      </section>
      <ProjectMembers project={project.id} />
      <section aria-labelledby="delete-project" className="settings-panel">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 id="delete-project" className="settings-panel-title">
              Delete project
            </h2>
            <p className="mt-2 max-w-prose text-sm leading-6 text-muted-foreground">
              {project.deployment_namespace
                ? "Remove this project from Workbench. The shared agent, deployment namespace and deployed applications are retained."
                : "Remove this project and stop all its workers, including any active runs. Private disks and saved evidence remain on the host."}
            </p>
          </div>
          <Button
            className="text-destructive"
            variant="outline"
            disabled={!canManage || deletion.isPending}
            onClick={() => {
              setConfirmation("");
              deletion.reset();
              setConfirming(true);
            }}
          >
            {deletion.isPending ? "Deleting project…" : "Delete project"}
          </Button>
        </div>
      </section>
      {confirming && (
        <Modal
          title="Delete project?"
          onClose={() => {
            if (!deletion.isPending) setConfirming(false);
          }}
          closeOnEscape={!deletion.isPending}
          closeDisabled={deletion.isPending}
        >
          <p className="break-words text-sm">
            Remove <strong>{project.name}</strong> from Workbench.
            {project.deployment_namespace
              ? " The shared agent, deployment namespace and deployed applications are retained."
              : " All its workers will stop, including any active runs."}
          </p>
          <p className="text-sm text-muted-foreground">
            Private disks and saved evidence remain on the host. This action
            does not erase that data. You cannot restore the project from this
            screen.
          </p>
          <Form
            onSubmit={() => {
              if (
                canManage &&
                confirmation === project.name &&
                !deletion.isPending
              )
                deletion.mutate();
            }}
          >
            <label className="block space-y-2 text-sm">
              <span>Type the project name to confirm</span>
              <Input
                value={confirmation}
                onChange={(event) => setConfirmation(event.target.value)}
                autoComplete="off"
                disabled={!canManage || deletion.isPending}
              />
            </label>
            {deletion.isError && <ErrorNotice error={deletion.error} />}
            <div className="flex flex-wrap justify-end gap-2">
              <Button
                type="button"
                variant="outline"
                disabled={deletion.isPending}
                onClick={() => setConfirming(false)}
              >
                Cancel
              </Button>
              <Button
                type="submit"
                variant="destructive"
                disabled={
                  !canManage ||
                  deletion.isPending ||
                  confirmation !== project.name
                }
              >
                {deletion.isPending ? "Deleting project…" : "Delete project"}
              </Button>
            </div>
          </Form>
        </Modal>
      )}
    </div>
  );
}
