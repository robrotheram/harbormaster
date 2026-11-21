import { useUserSession } from "@/features/auth/session";
import { memberOptions } from "@/features/work/responsibility";
import { ProjectMembers } from "./members";
import { useMutation, useQueryClient, useQuery } from "@tanstack/react-query";
import { Tabs } from "radix-ui";
import { Bot, Settings2 } from "lucide-react";
import { lazy, Suspense } from "react";
import { projectsApi } from "./api";
import { keys } from "@/shared/query-keys";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useState } from "react";
import { ErrorNotice, Modal, Form } from "@/shared/components";
import { request } from "@/shared/http";
import type { Project } from "@/shared/types";

const tabClassName =
  "inline-flex shrink-0 items-center justify-center gap-2 border-b-2 border-transparent px-3 py-3 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset data-[state=active]:border-primary data-[state=active]:text-foreground";

const HermesSettings = lazy(() =>
  import("@/features/project-assistant/hermes").then((m) => ({
    default: m.HermesSettings,
  })),
);

export function ProjectSettings({ project }: { project: Project }) {
  const search = useSearch({ from: "/projects/$projectId/$section" });
  const navigate = useNavigate();
  return (
    <div className="space-y-6">
      <div className="page-title">
        <h1>Project settings</h1>
      </div>
      <Tabs.Root
        value={search.tab === "hermes" ? "hermes" : "general"}
        onValueChange={(tab) =>
          void navigate({
            to: "/projects/$projectId/$section",
            params: { projectId: project.id, section: "settings" },
            search: { tab: tab === "hermes" ? "hermes" : "general" },
          })
        }
      >
        <Tabs.List
          aria-label="Project settings"
          className="flex max-w-full overflow-x-auto border-b"
        >
          <Tabs.Trigger value="general" className={tabClassName}>
            <Settings2 aria-hidden="true" className="size-4 shrink-0" />
            General
          </Tabs.Trigger>
          <Tabs.Trigger value="hermes" className={tabClassName}>
            <Bot aria-hidden="true" className="size-4 shrink-0" />
            Hermes setup
          </Tabs.Trigger>
        </Tabs.List>
        <Tabs.Content
          value="general"
          className="pt-6 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <GeneralSettings key={project.id} project={project} />
          <div className="mt-8 border-t pt-6">
            <ProjectMembers project={project.id} />
          </div>
        </Tabs.Content>
        <Tabs.Content
          value="hermes"
          className="pt-6 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Suspense fallback={<p role="status">Loading Hermes setup…</p>}>
            <HermesSettings project={project} />
          </Suspense>
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
    <div className="max-w-3xl space-y-6">
      {!canManage && !members.isPending && (
        <p className="text-sm text-muted-foreground">
          Project administrator access is required to rename or delete this
          project.
        </p>
      )}
      <section aria-labelledby="project-details" className="space-y-4">
        <h2 id="project-details" className="text-sm font-medium">
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
      <section aria-labelledby="delete-project" className="border-t pt-6">
        <h2 id="delete-project" className="text-sm font-medium">
          Delete project
        </h2>
        <p className="mt-2 max-w-prose text-sm leading-6 text-muted-foreground">
          {project.deployment_namespace
            ? "Remove this project from Workbench. The shared agent, deployment namespace and deployed applications are retained."
            : "Remove this project and stop all its workers, including any active runs. Private disks and saved evidence remain on the host."}
        </p>
        <Button
          className="mt-4"
          variant="destructive"
          disabled={!canManage || deletion.isPending}
          onClick={() => {
            setConfirmation("");
            deletion.reset();
            setConfirming(true);
          }}
        >
          {deletion.isPending ? "Deleting project…" : "Delete project"}
        </Button>
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
