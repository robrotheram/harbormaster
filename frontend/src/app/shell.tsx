import { Input } from "@/components/ui/input";
import { lazy, Suspense, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, Navigate, useNavigate } from "@tanstack/react-router";
import { keys } from "@/shared/query-keys";

import { projectsApi } from "@/features/projects/api";
import {
  Empty,
  ErrorNotice,
  Form,
  Modal,
  useRequestKey,
} from "@/shared/components";
import {
  Layers3,
  Plus,
  Network,
  Activity,
  BookOpen,
  ShieldAlert,
  ShieldCheck,
  Menu,
  ChevronRight,
  Sun,
  Moon,
  Boxes,
  Bot,
  Fingerprint,
  CalendarClock,
  ListChecks,
  type LucideIcon,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { NativeSelect } from "@/components/ui/native-select";
import { Skeleton } from "@/components/ui/skeleton";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { cn } from "@/lib/utils";
import type { Project, Section } from "@/shared/types";
const HermesSettings = lazy(() =>
  import("@/features/project-assistant/hermes").then((m) => ({
    default: m.HermesSettings,
  })),
);
const Workspaces = lazy(() =>
  import("@/features/workspaces/workspaces").then((m) => ({
    default: m.Workspaces,
  })),
);
const ConnectionProfiles = lazy(() =>
  import("@/features/connection-profiles/connection-profiles").then((m) => ({
    default: m.ConnectionProfiles,
  })),
);
const RuntimeOverview = lazy(() =>
  import("@/features/platform/platform").then((m) => ({
    default: m.RuntimeOverview,
  })),
);
const GettingStarted = lazy(() =>
  import("@/features/platform/platform").then((m) => ({
    default: m.GettingStarted,
  })),
);
const AgentHomes = lazy(() =>
  import("@/features/workspaces/agent-homes").then((m) => ({
    default: m.AgentHomes,
  })),
);
const Runs = lazy(() =>
  import("@/features/scheduling/runs").then((m) => ({ default: m.Runs })),
);
const Schedules = lazy(() =>
  import("@/features/scheduling/schedules").then((m) => ({
    default: m.Schedules,
  })),
);
const ToolProfiles = lazy(() =>
  import("@/features/tool-profiles/tool-profiles").then((m) => ({
    default: m.ToolProfiles,
  })),
);

const sections: { id: Section; label: string; icon: LucideIcon }[] = [
  { id: "workspaces", label: "Workspaces", icon: Boxes },
  { id: "homes", label: "Agent homes", icon: Fingerprint },
  { id: "schedules", label: "Schedules", icon: CalendarClock },
  { id: "runs", label: "Scheduled runs", icon: ListChecks },
  { id: "hermes", label: "Hermes setup", icon: Bot },
];
function savedProject() {
  try {
    return localStorage.getItem("workbench.project") || "";
  } catch {
    return "";
  }
}
function rememberProject(project: string) {
  try {
    localStorage.setItem("workbench.project", project);
  } catch {
    /* Private browsing may disable local preferences. */
  }
}

export function ProjectShell({
  projectID,
  globalSettings = false,
  section = "workspaces",
}: {
  projectID?: string;
  globalSettings?: boolean | "tools";
  section?: string;
}) {
  const projects = useQuery({
    queryKey: keys.projects,
    queryFn: ({ signal }) => projectsApi.list(signal),
    staleTime: 15000,
  });
  const navigate = useNavigate();
  const [create, setCreate] = useState(false);
  const [mobile, setMobile] = useState(false);
  const [dark, setDark] = useState(() => {
    try {
      return localStorage.getItem("workbench.theme") === "dark";
    } catch {
      return false;
    }
  });
  useEffect(() => {
    document.documentElement.classList.toggle("dark", dark);
    try {
      localStorage.setItem("workbench.theme", dark ? "dark" : "light");
    } catch {}
  }, [dark]);
  const target =
    projects.data?.find((p) => p.id === savedProject()) ||
    projects.data?.find((p) => p.id !== "default") ||
    projects.data?.[0];
  const project =
    projects.data?.find((p) => p.id === projectID) ||
    (globalSettings ? target : undefined);
  const selected: Section | undefined = globalSettings
    ? undefined
    : [
          "workspaces",
          "homes",
          "schedules",
          "runs",
          "hermes",
          "overview",
          "help",
        ].includes(section)
      ? (section as Section)
      : undefined;
  useEffect(() => {
    if (project) rememberProject(project.id);
  }, [project?.id]);
  const go = (value: string) => {
    rememberProject(value);
    setMobile(false);
    navigate({
      to: "/projects/$projectId/$section",
      params: { projectId: value, section: "workspaces" },
    });
  };
  const name = project?.id === "default" ? "Workspaces" : project?.name;
  const globalView = globalSettings || selected === "overview";
  const heading = globalSettings
    ? globalSettings === "tools"
      ? "Tool profiles"
      : "Connection profiles"
    : selected === "overview"
      ? "Runtime overview"
      : selected === "help"
        ? "Getting started"
        : sections.find((s) => s.id === selected)?.label || "Workspaces";
  const navigation = (
    <div className="flex h-full flex-col bg-[#171b26] text-slate-300">
      <Link
        to="/"
        className="flex h-16 items-center gap-3 border-b border-white/8 px-5 font-semibold tracking-tight text-white"
        onClick={() => setMobile(false)}
      >
        <span className="flex size-8 items-center justify-center rounded-lg bg-indigo-500">
          <Layers3 className="size-4 text-white" />
        </span>
        <span>Workbench</span>
        <span className="ml-auto rounded border border-white/15 px-1.5 py-0.5 text-[10px] font-medium text-slate-400">
          local
        </span>
      </Link>
      <div className="px-4 py-5">
        <label
          htmlFor={mobile ? "mobile-project" : "active-project"}
          className="mb-2 block text-xs font-medium text-slate-400"
        >
          Project
        </label>
        <NativeSelect
          id={mobile ? "mobile-project" : "active-project"}
          aria-label="Current project"
          value={project?.id || ""}
          disabled={projects.isPending}
          onChange={(e) => go(e.target.value)}
          className="w-full border-white/15 bg-white/5 text-slate-100 dark:bg-white/5"
        >
          <option value="" disabled>
            {projects.isPending ? "Loading projects…" : "Choose a project"}
          </option>
          {projects.data?.map((p) => (
            <option
              key={p.id}
              value={p.id}
              className="bg-slate-900 text-slate-100"
            >
              {p.id === "default" ? "Workspaces" : p.name}
            </option>
          ))}
        </NativeSelect>
        <Button
          variant="ghost"
          className="mt-2 h-8 w-full justify-start px-2 text-xs text-slate-400 hover:bg-white/5 hover:text-white"
          onClick={() => {
            setCreate(true);
            setMobile(false);
          }}
        >
          <Plus className="size-3.5" />
          New project
        </Button>
      </div>
      <nav aria-label="Project navigation" className="space-y-1 px-3">
        {sections.map((item) => {
          const Icon = item.icon;
          return project ? (
            <Link
              key={item.id}
              to="/projects/$projectId/$section"
              params={{ projectId: project.id, section: item.id }}
              onClick={() => setMobile(false)}
              className={cn(
                "rail-link",
                selected === item.id && "rail-link-active",
              )}
              aria-current={selected === item.id ? "page" : undefined}
            >
              <Icon className="size-4" />
              {item.label}
            </Link>
          ) : (
            <span key={item.id} className="rail-link opacity-40">
              <Icon className="size-4" />
              {item.label}
            </span>
          );
        })}
      </nav>
      <div className="mx-4 my-5 h-px bg-white/8" />
      <div className="mb-2 px-5 text-xs font-medium text-slate-500">
        Platform
      </div>
      <nav aria-label="Platform navigation" className="space-y-1 px-3">
        <Link
          to="/connection-profiles"
          className={cn(
            "rail-link",
            globalSettings === true && "rail-link-active",
          )}
          onClick={() => setMobile(false)}
          aria-current={globalSettings === true ? "page" : undefined}
        >
          <Network className="size-4" />
          Connection profiles
        </Link>
        <Link
          to="/tool-profiles"
          className={cn(
            "rail-link",
            globalSettings === "tools" && "rail-link-active",
          )}
          onClick={() => setMobile(false)}
          aria-current={globalSettings === "tools" ? "page" : undefined}
        >
          <Network className="size-4" />
          Tool profiles
        </Link>
        {project && (
          <>
            <Link
              to="/projects/$projectId/$section"
              params={{ projectId: project.id, section: "overview" }}
              className={cn(
                "rail-link",
                selected === "overview" && "rail-link-active",
              )}
              onClick={() => setMobile(false)}
            >
              <Activity className="size-4" />
              Runtime overview
            </Link>
            <Link
              to="/projects/$projectId/$section"
              params={{ projectId: project.id, section: "help" }}
              className={cn(
                "rail-link",
                selected === "help" && "rail-link-active",
              )}
              onClick={() => setMobile(false)}
            >
              <BookOpen className="size-4" />
              Getting started
            </Link>
          </>
        )}
      </nav>
      <div className="mt-auto px-4 pt-8 pb-4">
        <div className="mb-4 flex items-start gap-2 rounded-lg border border-white/10 p-3 text-xs leading-5 text-slate-400">
          <ShieldAlert className="mt-0.5 size-3.5 shrink-0 text-amber-400/80" />
          <span>
            Development runtime
            <br />
            <span className="text-slate-500">VM isolation unverified</span>
          </span>
        </div>
        <div className="flex items-center gap-3 border-t border-white/8 pt-4">
          <span className="flex size-8 items-center justify-center rounded-full bg-slate-700 text-xs font-semibold text-slate-200">
            LO
          </span>
          <div className="text-xs">
            <p className="font-medium text-slate-200">Local operator</p>
            <p className="mt-0.5 text-slate-500">Workspace administration</p>
          </div>
        </div>
      </div>
    </div>
  );
  if (!globalSettings && !projectID && target)
    return (
      <Navigate
        to="/projects/$projectId/$section"
        params={{ projectId: target.id, section: "workspaces" }}
        replace
      />
    );
  return (
    <div className="min-h-dvh bg-muted/40">
      <a className="skip" href="#main">
        Skip to content
      </a>
      <aside className="fixed inset-y-0 left-0 z-30 hidden w-[232px] lg:block">
        {!mobile && navigation}
      </aside>
      <Sheet open={mobile} onOpenChange={setMobile}>
        <SheetContent
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            document
              .querySelector<HTMLButtonElement>(
                '[aria-label="Open navigation"]',
              )
              ?.focus();
          }}
          side="left"
          className="w-[260px] border-0 p-0 [&>button]:text-slate-300"
          aria-describedby={undefined}
        >
          <SheetTitle className="sr-only">Workbench navigation</SheetTitle>
          {mobile && navigation}
        </SheetContent>
      </Sheet>
      <div className="min-w-0 lg:pl-[232px]">
        <header className="sticky top-0 z-20 flex h-16 items-center justify-between gap-4 border-b bg-background/95 px-4 backdrop-blur-sm sm:px-8">
          <div className="flex min-w-0 items-center gap-3">
            <Button
              variant="ghost"
              size="icon"
              aria-label="Open navigation"
              className="lg:hidden"
              onClick={() => setMobile(true)}
            >
              <Menu />
            </Button>
            <span className="hidden text-sm text-muted-foreground sm:inline">
              {globalView ? "Platform" : name || "Projects"}
            </span>
            <ChevronRight className="hidden size-3.5 text-muted-foreground/60 sm:block" />
            <div className="min-w-0">
              <span className="block truncate text-[10px] text-muted-foreground sm:hidden">
                {globalView ? "Platform" : name || "Projects"}
              </span>
              <span className="block truncate text-sm font-medium">
                {heading}
              </span>
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-3">
            <Badge
              variant="outline"
              className="hidden gap-1.5 text-muted-foreground sm:inline-flex"
            >
              <span className="size-1.5 rounded-full bg-teal-500" />
              Local deployment
            </Badge>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={
                dark ? "Switch to light theme" : "Switch to dark theme"
              }
              onClick={() => setDark(!dark)}
            >
              {dark ? <Sun /> : <Moon />}
            </Button>
          </div>
        </header>
        <main id="main" className="mx-auto max-w-[1560px] p-4 sm:p-8 lg:p-9">
          <Suspense fallback={<Skeleton className="h-72 w-full" />}>
            {projects.isError && <ErrorNotice error={projects.error} />}
            {globalSettings ? (
              globalSettings === "tools" ? (
                <ToolProfiles />
              ) : (
                <ConnectionProfiles />
              )
            ) : projects.isPending ? (
              <div className="space-y-5">
                <Skeleton className="h-10 w-56" />
                <Skeleton className="h-14 w-full" />
                <Skeleton className="h-72 w-full" />
              </div>
            ) : project && selected ? (
              <ProjectView
                key={project.id}
                project={project}
                section={selected}
              />
            ) : projectID && !project ? (
              <Empty title="Project unavailable">
                Choose a project from the navigation to continue.
              </Empty>
            ) : !project ? (
              <Empty
                title="Create your first project"
                action={
                  <Button onClick={() => setCreate(true)}>
                    <Plus />
                    Create project
                  </Button>
                }
              >
                Give your agents a separate place for files, workspaces and
                history.
              </Empty>
            ) : (
              <Empty title="Page unavailable">
                Choose a page from the navigation.
              </Empty>
            )}
          </Suspense>
        </main>
        <footer className="mx-4 flex flex-wrap items-center justify-between gap-2 border-t py-5 text-xs text-muted-foreground sm:mx-8 lg:mx-9">
          <span className="flex items-center gap-1.5">
            <ShieldCheck className="size-3.5" />
            {globalView
              ? "Deployment-wide settings"
              : "Workspace data stays within its project"}
          </span>
          <span>Agent Workbench</span>
        </footer>
      </div>
      {create && (
        <CreateProject
          onClose={() => setCreate(false)}
          onCreated={(value) => {
            setCreate(false);
            go(value.id);
          }}
        />
      )}
    </div>
  );
}

function ProjectView({
  project,
  section,
}: {
  project: Project;
  section: Section;
}) {
  if (section === "homes") return <AgentHomes project={project} />;
  if (section === "overview") return <RuntimeOverview />;
  if (section === "help") return <GettingStarted />;
  if (section === "schedules") return <Schedules project={project} />;
  if (section === "runs") return <Runs project={project} />;
  if (section === "hermes") return <HermesSettings project={project} />;
  return <Workspaces project={project} />;
}

function CreateProject({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: (project: Project) => void;
}) {
  const [name, setName] = useState(""),
    key = useRequestKey(),
    queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: () => {
      const payload = { name: name.trim() };
      return projectsApi.create(payload.name, key(payload));
    },
    onSuccess: async (project) => {
      await queryClient.invalidateQueries({ queryKey: keys.projects });
      onCreated(project);
    },
  });
  return (
    <Modal title="Create project" onClose={onClose}>
      <Form onSubmit={() => mutation.mutate()}>
        <p className="form-note">
          A project keeps its workspaces, conversations, memory and credentials
          separate. Give it a name that makes that boundary clear.
        </p>
        <label>
          Project name
          <Input
            autoFocus
            required
            maxLength={80}
            placeholder="Vulnerability research"
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </label>
        {mutation.isError && <ErrorNotice error={mutation.error} />}
        <div className="form-actions">
          <Button variant="outline" type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="default" disabled={mutation.isPending}>
            {mutation.isPending ? "Creating…" : "Create project"}
          </Button>
        </div>
      </Form>
    </Modal>
  );
}
