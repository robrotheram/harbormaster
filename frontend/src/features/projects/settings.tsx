import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useState } from "react";
import { ErrorNotice, Modal, Form } from "@/shared/components";
import { request } from "@/shared/http";
import type { Project } from "@/shared/types";

export function ProjectSettings({ project }: { project: Project }) {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
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
      await queryClient.invalidateQueries();
      await navigate({ to: "/" });
    },
  });
  return (
    <div className="max-w-3xl space-y-6">
      <div className="page-title">
        <div>
          <h1>Project settings</h1>
        </div>
      </div>
      <section aria-labelledby="project-details" className="space-y-4">
        <h2 id="project-details" className="text-sm font-medium">
          Project details
        </h2>
        <dl className="project-setting-facts">
          <div>
            <dt className="text-muted-foreground">Name</dt>
            <dd>{project.name}</dd>
          </div>
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
        {project.id === "default" ? (
          <p className="mt-2 text-sm text-muted-foreground">
            The default project cannot be deleted.
          </p>
        ) : (
          <>
            <p className="mt-2 max-w-prose text-sm leading-6 text-muted-foreground">
              {project.deployment_namespace
                ? "Remove this project from Workbench. The shared agent, deployment namespace and deployed applications are retained."
                : "Remove this project and all its workers from Workbench. Compute must stop successfully before deletion. Private disks and saved evidence remain on the host."}
            </p>
            <Button
              className="mt-4"
              variant="destructive"
              disabled={deletion.isPending}
              onClick={() => {
                setConfirmation("");
                deletion.reset();
                setConfirming(true);
              }}
            >
              {deletion.isPending ? "Deleting project…" : "Delete project"}
            </Button>
          </>
        )}
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
              : " Its workers must stop successfully before deletion."}
          </p>
          <p className="text-sm text-muted-foreground">
            Private disks and saved evidence remain on the host. This action
            does not erase that data. You cannot restore the project from this
            screen.
          </p>
          <Form
            onSubmit={() => {
              if (confirmation === project.name && !deletion.isPending)
                deletion.mutate();
            }}
          >
            <label className="block space-y-2 text-sm">
              <span>Type the project name to confirm</span>
              <Input
                value={confirmation}
                onChange={(event) => setConfirmation(event.target.value)}
                autoComplete="off"
                disabled={deletion.isPending}
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
                disabled={deletion.isPending || confirmation !== project.name}
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
