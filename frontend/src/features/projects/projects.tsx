import { projectsApi } from "./api";
import { keys } from "@/shared/query-keys";
import { ProjectInvitations } from "./members";
import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { ArrowUpRight, Plus, Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Empty, ErrorNotice, useRequestKey } from "@/shared/components";
import type { Project } from "@/shared/types";
import { workOptions } from "@/features/work/api";
import { workStage } from "@/features/work/stages";
import { workspaceOptions } from "@/features/workspaces/api";

export function Projects({
  projects,
  onCreate,
}: {
  projects: Project[];
  onCreate: () => void;
}) {
  const client = useQueryClient();
  const navigate = useNavigate();
  const requestKey = useRequestKey();
  const sample = useMutation({
    mutationFn: () =>
      projectsApi.create(
        "Sample project",
        requestKey({ name: "Sample project", sample: true }),
        true,
      ),
    onSuccess: async (project) => {
      client.setQueryData<Project[]>(keys.projects, (old) => [
        ...(old || []).filter((p) => p.id !== project.id),
        project,
      ]);
      await navigate({
        to: "/projects/$projectId/$section",
        params: { projectId: project.id, section: "team" },
      });
      await client.invalidateQueries({ queryKey: keys.projects });
    },
  });
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const pageSize = 20;
  const visible = projects.filter((p) =>
    p.name.toLowerCase().includes(search.trim().toLowerCase()),
  );
  const pageCount = Math.ceil(visible.length / pageSize);
  const currentPage = Math.min(page, Math.max(0, pageCount - 1));
  const start = currentPage * pageSize;
  const shown = visible.slice(start, start + pageSize);
  const changeSearch = (value: string) => {
    setSearch(value);
    setPage(0);
  };
  return (
    <>
      <ProjectInvitations />
      <div className="page-title">
        <div>
          <h1>Projects</h1>
          <p>Open a project to continue work or review its results.</p>
        </div>
        <Button onClick={onCreate}>
          <Plus />
          New project
        </Button>
      </div>
      {!!projects.length && (
        <div className="inventory-toolbar">
          <span aria-live="polite">
            {search.trim()
              ? `${visible.length} of ${projects.length}`
              : projects.length}{" "}
            {projects.length === 1 ? "project" : "projects"}
          </span>
          <div className="compact-search">
            <Search size={14} />
            <Input
              className="h-8 pl-8 text-xs md:text-xs shadow-none"
              aria-label="Search projects"
              placeholder="Find a project…"
              value={search}
              onChange={(e) => changeSearch(e.target.value)}
            />
          </div>
        </div>
      )}
      {!projects.length && sample.isError && (
        <ErrorNotice error={sample.error} />
      )}
      {!projects.length ? (
        <Empty
          title="Getting started"
          action={
            <div className="flex flex-wrap justify-center gap-2">
              <Button onClick={onCreate} disabled={sample.isPending}>
                <Plus />
                Create project
              </Button>
              <Button
                variant="outline"
                disabled={sample.isPending}
                onClick={() => sample.mutate()}
              >
                {sample.isPending
                  ? "Adding sample project…"
                  : "Add sample project"}
              </Button>
            </div>
          }
        >
          Create your own project, or add a sample project with three planned
          tasks in Overview to explore ownership, priorities and dependencies.
          No agents run automatically.
        </Empty>
      ) : !visible.length ? (
        <Empty
          title="No matching projects"
          action={
            <Button variant="outline" onClick={() => changeSearch("")}>
              Clear search
            </Button>
          }
        >
          Try a different project name.
        </Empty>
      ) : (
        <div className="project-inventory">
          <div className="project-inventory-head">
            <span>Project</span>
            <span>Work pipeline</span>
            <span>Workers</span>
            <span />
          </div>
          {shown.map((p) => (
            <ProjectRow key={p.id} project={p} />
          ))}
        </div>
      )}
      {pageCount > 1 && (
        <nav
          aria-label="Project pages"
          className="mt-4 flex flex-wrap items-center justify-between gap-3 text-xs text-muted-foreground"
        >
          <span aria-live="polite">
            Showing {start + 1}–{start + shown.length} of {visible.length}{" "}
            projects
          </span>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              disabled={currentPage === 0}
              onClick={() => setPage(currentPage - 1)}
            >
              Previous page
            </Button>
            <Button
              variant="outline"
              disabled={currentPage + 1 === pageCount}
              onClick={() => setPage(currentPage + 1)}
            >
              Next page
            </Button>
          </div>
        </nav>
      )}
    </>
  );
}
function ProjectRow({ project }: { project: Project }) {
  const work = useQuery(workOptions(project.id));
  const workers = useQuery(workspaceOptions(project.id));
  const counts = (id: string) =>
    work.data?.filter((w) => workStage(w.state) === id).length ?? 0;
  return (
    <Link
      to="/projects/$projectId/$section"
      params={{ projectId: project.id, section: "work" }}
      className="project-inventory-row"
    >
      <div className="flex min-w-0 items-center gap-3">
        <span className="project-tile">
          {project.name.slice(0, 2).toUpperCase()}
        </span>
        <div className="min-w-0">
          <h2 className="truncate font-medium" title={project.name}>
            {project.name}
          </h2>
          <p className="mt-1 text-[11px] text-muted-foreground">
            {work.isError
              ? "Work status unavailable"
              : work.isPending
                ? "Loading work…"
                : `${work.data?.length ?? 0} work item${work.data?.length === 1 ? "" : "s"}`}
          </p>
        </div>
      </div>
      <div className="project-pipeline">
        {work.isPending || work.isError ? (
          <span className="text-muted-foreground">
            {work.isError ? "Status unavailable" : "Loading…"}
          </span>
        ) : (
          <>
            <span>
              <i className="stage-dot stage-running" />
              {counts("running")} active
            </span>
            <span>
              <i className="stage-dot stage-review" />
              {counts("review")} review
            </span>
            <span>
              <i className="stage-dot stage-done" />
              {work.data?.filter((item) =>
                ["completed", "accepted"].includes(item.state.toLowerCase()),
              ).length ?? 0}{" "}
              accepted
            </span>
            {!!work.data?.some((item) => item.state === "closed") && (
              <span>
                {work.data.filter((item) => item.state === "closed").length}{" "}
                closed
              </span>
            )}
            {counts("blocked") > 0 && (
              <span className="text-amber-600 dark:text-amber-400">
                {counts("blocked")} need attention
              </span>
            )}
          </>
        )}
      </div>
      <span className="project-worker-count">
        {workers.isError
          ? "Unavailable"
          : workers.isPending
            ? "Loading…"
            : `${workers.data?.filter((w) => w.ready).length ?? 0} / ${workers.data?.length ?? 0} ready`}
      </span>
      <ArrowUpRight size={15} className="text-muted-foreground" />
    </Link>
  );
}
