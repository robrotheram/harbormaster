import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { ErrorNotice } from "@/shared/components";
import { request } from "@/shared/http";
import type { Project } from "@/shared/types";

export function ProjectSettings({ project }: { project: Project }) {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
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
    <div className="max-w-3xl space-y-10">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          Project settings
        </h1>
        <p className="mt-2 text-sm text-muted-foreground">
          Manage {project.name} and its workspaces.
        </p>
      </div>
      <section aria-labelledby="project-details" className="space-y-4">
        <h2 id="project-details" className="text-base font-semibold">
          Project details
        </h2>
        <dl className="divide-y rounded-lg border bg-background px-5 text-sm">
          <div className="py-4">
            <dt className="text-muted-foreground">Name</dt>
            <dd className="mt-1 font-medium">{project.name}</dd>
          </div>
          <div className="py-4">
            <dt className="text-muted-foreground">Project ID</dt>
            <dd className="mt-1 break-all">{project.id}</dd>
          </div>
        </dl>
      </section>
      <section aria-labelledby="delete-project" className="border-t pt-6">
        <h2 id="delete-project" className="text-base font-semibold">
          Delete project
        </h2>
        {project.id === "default" ? (
          <p className="mt-2 text-sm text-muted-foreground">
            The default project cannot be deleted.
          </p>
        ) : (
          <>
            <p className="mt-2 max-w-prose text-sm leading-6 text-muted-foreground">
              Remove this project and all its workspaces from Workbench. Compute
              must stop successfully before deletion. Private disks and saved
              evidence remain on the host.
            </p>
            <Button
              className="mt-4"
              variant="destructive"
              disabled={deletion.isPending}
              onClick={() => {
                if (
                  window.confirm(
                    `Delete project ${project.name} and all its workspaces from the workbench? Compute must stop first. Private disks remain on the host.`,
                  )
                )
                  deletion.mutate();
              }}
            >
              {deletion.isPending ? "Deleting project…" : "Delete project"}
            </Button>
            {deletion.isError && (
              <div className="mt-4">
                <ErrorNotice error={deletion.error} />
              </div>
            )}
          </>
        )}
      </section>
    </div>
  );
}
