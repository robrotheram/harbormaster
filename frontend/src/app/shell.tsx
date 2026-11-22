import { PersonalWorkProvider } from "@/features/work/personal-work";
import { lazy, Suspense, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import {
  Boxes,
  Plus,
  Server,
  HeartPulse,
  BookOpen,
  Menu,
  PanelLeftClose,
  PanelLeftOpen,
  ChevronRight,
  ChevronDown,
  Check,
  Sun,
  Moon,
  FolderGit2,
  Factory as FactoryIcon,
  Settings2,
  Layers3,
  ListChecks,
  X,
} from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuItem,
  DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  Empty,
  ErrorNotice,
  Form,
  Modal,
  useRequestKey,
  State,
} from "@/shared/components";
import { keys } from "@/shared/query-keys";
import { projectsApi } from "@/features/projects/api";
import { snapshotOptions } from "@/features/platform/api";
import { cn } from "@/lib/utils";
import { UserProfileMenu } from "@/features/auth/user-profile";
import type { Project, Section } from "@/shared/types";
const ProjectSettings = lazy(() =>
  import("@/features/projects/settings").then((m) => ({
    default: m.ProjectSettings,
  })),
);
const WorkerPage = lazy(() =>
  import("@/features/workspaces/worker-page").then((m) => ({
    default: m.WorkerPage,
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
const Infrastructure = lazy(() =>
  import("@/features/platform/infrastructure").then((m) => ({
    default: m.Infrastructure,
  })),
);
const PlatformServices = lazy(() =>
  import("@/features/platform/services").then((m) => ({
    default: m.PlatformServices,
  })),
);
const Projects = lazy(() =>
  import("@/features/projects/projects").then((m) => ({ default: m.Projects })),
);
const GettingStarted = lazy(() =>
  import("@/features/platform/platform").then((m) => ({
    default: m.GettingStarted,
  })),
);
const Work = lazy(() =>
  import("@/features/work/work").then((m) => ({ default: m.Work })),
);
const Delivery = lazy(() =>
  import("@/features/delivery/delivery").then((m) => ({ default: m.Delivery })),
);
const Connections = lazy(() =>
  import("@/features/connections/connections").then((m) => ({
    default: m.Connections,
  })),
);
const sections = [
  { id: "team", label: "Overview", icon: Layers3 },
  { id: "work", label: "Workspace", icon: ListChecks },
  { id: "workspaces", label: "Workers", icon: Boxes },
  { id: "settings", label: "Settings", icon: Settings2 },
] as const;

export function ProjectShell({
  projectID,
  globalSettings = false,
  section = "work",
  globalPage,
  workerID,
  workerPanel,
}: {
  projectID?: string;
  workerID?: string;
  workerPanel?: "environment" | "settings" | "activity";
  globalSettings?: boolean;
  section?: string;
  globalPage?:
    "infrastructure" | "services" | "help" | "repositories" | "connections";
}) {
  const projects = useQuery({
    queryKey: keys.projects,
    queryFn: ({ signal }) => projectsApi.list(signal),
    staleTime: 15000,
  });
  const navigate = useNavigate();
  const [create, setCreate] = useState(false);
  const [mobile, setMobile] = useState(false);
  const [systemOpen, setSystemOpen] = useState(false);
  const healthQuery = useQuery({ ...snapshotOptions(), enabled: systemOpen });
  const observedServices = healthQuery.data?.services?.items;
  const healthStatus = healthQuery.isError
    ? "unavailable"
    : healthQuery.isPending
      ? "checking"
      : observedServices?.some((service) =>
            ["degraded", "unavailable", "unhealthy", "failed", "down"].includes(
              service.status.toLowerCase(),
            ),
          )
        ? "degraded"
        : observedServices?.length &&
            observedServices.every(
              (service) => service.status.toLowerCase() === "healthy",
            )
          ? "healthy"
          : "unknown";
  const [collapsed, setCollapsed] = useState(() => {
    try {
      return localStorage.getItem("workbench.sidebar-collapsed") !== "false";
    } catch {
      return false;
    }
  });
  useEffect(() => {
    try {
      localStorage.setItem("workbench.sidebar-collapsed", String(collapsed));
    } catch {}
  }, [collapsed]);
  const [dark, setDark] = useState(() => {
    try {
      return localStorage.getItem("workbench.theme") !== "light";
    } catch {
      return true;
    }
  });
  useEffect(() => {
    document.documentElement.classList.toggle("dark", dark);
    try {
      localStorage.setItem("workbench.theme", dark ? "dark" : "light");
    } catch {}
  }, [dark]);
  const project = projects.data?.find((p) => p.id === projectID);
  // Old bookmarked runtime/help routes remain global views.
  const infrastructure =
    globalPage === "infrastructure" || section === "overview";
  const help = globalPage === "help" || section === "help";
  const services = globalPage === "services";
  const repositories =
    globalPage === "connections" ||
    globalPage === "repositories" ||
    section === "repositories";
  const isGlobal =
    globalSettings ||
    infrastructure ||
    services ||
    repositories ||
    help ||
    !projectID;
  const selected = sections.find(
    (s) =>
      s.id ===
      (section === "homes"
        ? "workspaces"
        : section === "board"
          ? "team"
          : section),
  );
  const heading = globalSettings
    ? "Network profiles"
    : repositories
      ? "Connections"
      : services
        ? "Platform services"
        : infrastructure
          ? "Workers"
          : help
            ? "Docs"
            : !projectID
              ? "Projects"
              : project?.name || "Project";
  const inProject = !isGlobal && !!project;
  const projectsBlocked =
    projects.isError && (!projects.data || (!isGlobal && !project));
  const independentPage = globalSettings || services || help || repositories;
  const [lastProjectID, setLastProjectID] = useState(() => {
    try {
      return localStorage.getItem("workbench.last-project") || "";
    } catch {
      return "";
    }
  });
  // Global pages retain the user's project context. Resolve it against the
  // current list so a removed or inaccessible project never leaves dead links.
  const navigationProject =
    project ||
    projects.data?.find((p) => p.id === lastProjectID) ||
    projects.data?.[0];
  useEffect(() => {
    if (!navigationProject) return;
    setLastProjectID(navigationProject.id);
    try {
      localStorage.setItem("workbench.last-project", navigationProject.id);
    } catch {}
  }, [navigationProject?.id]);
  const factoryNavigation = (
    <nav
      aria-label="Factory navigation"
      className="shrink-0 px-2.5 pt-3 space-y-1"
    >
      <Link
        to="/"
        aria-label="Projects"
        title="Projects"
        aria-current={
          !projectID && !globalSettings && !globalPage ? "page" : undefined
        }
        className={cn(
          "factory-nav",
          !projectID && !globalSettings && !globalPage && "is-active",
        )}
        onClick={() => setMobile(false)}
      >
        <FolderGit2 />
        <span className="rail-label">Projects</span>
      </Link>
      {navigationProject && (
        <>
          <Link
            to="/workers"
            aria-label="Workers"
            title="Workers"
            aria-current={infrastructure ? "page" : undefined}
            className={cn("factory-nav", infrastructure && "is-active")}
            onClick={() => setMobile(false)}
          >
            <Boxes />
            <span className="rail-label">Workers</span>
          </Link>
          <DropdownMenu open={systemOpen} onOpenChange={setSystemOpen}>
            <DropdownMenuTrigger asChild>
              <button
                type="button"
                className={cn(
                  "factory-nav system-menu-trigger",
                  (globalSettings || services || repositories) && "is-active",
                )}
                aria-label="Platform"
                title="Platform"
              >
                <Server />
                <span className="rail-label">Platform</span>
                <ChevronDown className="rail-label ml-auto" />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent
              side="top"
              align="start"
              className="system-menu w-60"
            >
              <DropdownMenuLabel>Platform</DropdownMenuLabel>
              {(
                [
                  {
                    to: "/connections",
                    label: "Connections",
                    icon: FolderGit2,
                    active: repositories,
                  },
                  {
                    to: "/connection-profiles",
                    label: "Network profiles",
                    icon: Settings2,
                    active: globalSettings,
                  },
                  {
                    to: "/platform-services",
                    label: "Health",
                    icon: HeartPulse,
                    active: services,
                  },
                ] as const
              ).map((item) => (
                <DropdownMenuItem key={item.to} asChild>
                  <Link
                    to={item.to}
                    aria-label={item.label}
                    aria-description={
                      item.to === "/platform-services"
                        ? `Service health: ${healthStatus}`
                        : undefined
                    }
                    aria-current={item.active ? "page" : undefined}
                    onClick={() => setMobile(false)}
                  >
                    <item.icon size={16} />
                    <span className="flex-1">{item.label}</span>
                    {item.to === "/platform-services" && (
                      <span className="system-health-badge">
                        <State value={healthStatus} />
                      </span>
                    )}
                    {item.active && <Check size={14} />}
                  </Link>
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        </>
      )}
    </nav>
  );
  const navigation = (
    <div className="factory-rail">
      <div className="factory-brand-row">
        <Link
          to="/"
          className="factory-brand"
          aria-label="Workbench Software factory"
          title="Workbench"
          onClick={() => setMobile(false)}
        >
          <span className="brand-symbol">
            <FactoryIcon size={18} />
          </span>
          <span className="rail-label">
            Workbench <small>Software factory</small>
          </span>
        </Link>
        {!mobile && (
          <Button
            variant="ghost"
            size="icon-sm"
            className="sidebar-collapse ml-auto hidden lg:inline-flex"
            aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
            title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
            aria-expanded={!collapsed}
            aria-controls="factory-sidebar"
            onClick={() => setCollapsed((value) => !value)}
          >
            {collapsed ? <PanelLeftOpen /> : <PanelLeftClose />}
          </Button>
        )}
      </div>
      {navigationProject ? (
        <>
          <div className="project-switcher">
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  className="project-switcher-button"
                  aria-label={`Switch project: ${navigationProject.name}`}
                  title={`Switch project: ${navigationProject.name}`}
                >
                  <span className="project-glyph" aria-hidden="true">
                    <FolderGit2 size={14} />
                  </span>
                  <span className="rail-label truncate">
                    {navigationProject.name}
                  </span>
                  <ChevronDown className="rail-label ml-auto size-4 shrink-0" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" className="w-64">
                <DropdownMenuLabel>Projects</DropdownMenuLabel>
                {projects.data?.map((p) => (
                  <DropdownMenuItem key={p.id} asChild>
                    <Link
                      to="/projects/$projectId/$section"
                      params={{
                        projectId: p.id,
                        section: inProject ? selected?.id || "work" : "work",
                      }}
                      onClick={() => setMobile(false)}
                    >
                      <span className="flex-1 truncate">{p.name}</span>
                      {p.id === navigationProject.id && <Check size={14} />}
                    </Link>
                  </DropdownMenuItem>
                ))}
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  onSelect={() => {
                    setCreate(true);
                    setMobile(false);
                  }}
                >
                  <Plus size={16} /> New project
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
          <nav aria-label="Project navigation" className="project-section-list">
            {sections.map((item) => (
              <Link
                key={item.id}
                to="/projects/$projectId/$section"
                params={{ projectId: navigationProject.id, section: item.id }}
                className={cn(
                  "factory-nav project-section-nav",
                  inProject && section === item.id && "is-active",
                )}
                aria-label={item.label}
                title={item.label}
                aria-current={
                  inProject && section === item.id ? "page" : undefined
                }
                onClick={() => setMobile(false)}
              >
                <item.icon size={16} />
                <span className="rail-label">{item.label}</span>
              </Link>
            ))}
          </nav>
        </>
      ) : null}
      <div className="rail-bottom">
        <div className="project-global-links">{factoryNavigation}</div>
        <UserProfileMenu />
      </div>
    </div>
  );
  return (
    <div className="factory-app" data-sidebar-collapsed={collapsed}>
      <a className="skip" href="#main">
        Skip to content
      </a>
      <aside id="factory-sidebar" className="factory-sidebar">
        {!mobile && navigation}
      </aside>
      <Sheet open={mobile} onOpenChange={setMobile}>
        <SheetContent
          side="left"
          className="w-[272px] max-w-[calc(100vw-32px)] p-0"
          showCloseButton={false}
          aria-describedby={undefined}
          onCloseAutoFocus={(e) => {
            e.preventDefault();
            document
              .querySelector<HTMLButtonElement>(
                '[aria-label="Open navigation"]',
              )
              ?.focus();
          }}
        >
          <SheetTitle className="sr-only">Factory navigation</SheetTitle>
          {mobile && navigation}
          <SheetClose
            className="absolute top-[5px] right-2 inline-flex size-11 items-center justify-center text-muted-foreground hover:bg-muted hover:text-foreground"
            aria-label="Close navigation"
          >
            <X className="size-4" />
          </SheetClose>
        </SheetContent>
      </Sheet>
      <div className="factory-body">
        <header className="factory-header">
          <div className="project-identity flex min-w-0 items-center gap-2.5">
            <Button
              variant="ghost"
              size="icon-lg"
              className="shell-toggle size-[44px] lg:hidden [&_svg]:size-[18px]"
              aria-label="Open navigation"
              onClick={() => setMobile(true)}
            >
              <Menu />
            </Button>
            {isGlobal && (
              <>
                <span className="hidden text-muted-foreground sm:inline">
                  Factory
                </span>
                <ChevronRight className="hidden size-4 shrink-0 text-muted-foreground sm:block" />
              </>
            )}
            <span className="truncate font-medium">{heading}</span>
            {inProject && selected && (
              <>
                <ChevronRight className="hidden size-4 shrink-0 text-muted-foreground sm:block" />
                <span className="hidden truncate text-muted-foreground sm:inline">
                  {selected.label}
                </span>
              </>
            )}
          </div>

          <div className="shell-utilities flex shrink-0 items-center gap-2">
            <Button
              asChild
              variant="ghost"
              className="docs-link h-auto min-h-[44px] w-[44px] gap-2 p-0 sm:w-auto sm:px-2.5 lg:min-h-[36px]"
            >
              <Link
                to="/help"
                aria-label="Docs"
                title="Docs"
                aria-current={help ? "page" : undefined}
              >
                <BookOpen size={16} />
                <span className="hidden sm:inline">Docs</span>
              </Link>
            </Button>
            <Button
              variant="ghost"
              size="icon-lg"
              className="size-[44px] lg:size-[36px] [&_svg]:size-[18px]"
              aria-label={
                dark ? "Switch to light theme" : "Switch to dark theme"
              }
              onClick={() => setDark(!dark)}
            >
              {dark ? <Sun /> : <Moon />}
            </Button>
          </div>
        </header>

        <main id="main" tabIndex={-1} className="factory-main">
          <Suspense
            fallback={
              <Skeleton className="h-80 w-full" role="status">
                <span className="sr-only">Loading page…</span>
              </Skeleton>
            }
          >
            {projects.isError && (independentPage || !projectsBlocked) && (
              <section
                aria-label="Project list status"
                tabIndex={0}
                className="project-list-status mb-5"
              >
                <h2 className="mb-2 text-sm font-medium">
                  Project list unavailable
                </h2>
                <ErrorNotice
                  error={projects.error}
                  message={
                    projects.data
                      ? "Previously loaded projects remain available. Your current page and edits are preserved."
                      : "Project navigation is unavailable. You can continue using this page while the project list recovers."
                  }
                  onRetry={() => void projects.refetch()}
                  retrying={projects.isFetching}
                />
              </section>
            )}
            {globalSettings ? (
              <ConnectionProfiles />
            ) : services ? (
              <PlatformServices />
            ) : help ? (
              <GettingStarted />
            ) : repositories ? (
              <Connections
                projects={projects.data || []}
                repositories={
                  globalPage === "repositories" || section === "repositories"
                }
              />
            ) : projects.isPending ? (
              <div
                className="loading-placeholder"
                role="status"
                aria-label="Loading projects"
              >
                <p className="mb-4 text-sm text-muted-foreground">
                  Loading projects…
                </p>
                <Skeleton aria-hidden="true" className="h-80 w-full" />
              </div>
            ) : projectsBlocked ? (
              <section aria-labelledby="projects-unavailable">
                <div className="page-title">
                  <div>
                    <h1 id="projects-unavailable">Projects unavailable</h1>
                    <p>
                      Workbench could not retrieve the project list. Try again
                      to continue.
                    </p>
                  </div>
                </div>
                <ErrorNotice
                  error={projects.error}
                  onRetry={() => void projects.refetch()}
                  retrying={projects.isFetching}
                />
                <Button asChild variant="outline">
                  <Link to="/help">Open help</Link>
                </Button>
              </section>
            ) : infrastructure ? (
              <Infrastructure projects={projects.data || []} />
            ) : !projectID ? (
              <Projects
                projects={projects.data || []}
                onCreate={() => setCreate(true)}
              />
            ) : !project ? (
              <section aria-labelledby="unavailable-project">
                <div className="page-title">
                  <h1 id="unavailable-project">Project unavailable</h1>
                </div>
                <Empty
                  title="Choose another project"
                  action={
                    <Button asChild>
                      <Link to="/">Open Projects</Link>
                    </Button>
                  }
                >
                  This project may have been removed or your access may have
                  changed. Open Projects to choose an available project.
                </Empty>
              </section>
            ) : selected ? (
              <PersonalWorkProvider key={project.id}>
                {workerID ? (
                  <WorkerPage
                    key={workerID}
                    project={project}
                    workerID={workerID}
                    panel={workerPanel}
                  />
                ) : (
                  <ProjectView
                    key={project.id}
                    project={project}
                    section={selected.id}
                  />
                )}
              </PersonalWorkProvider>
            ) : (
              <section aria-labelledby="unavailable-page">
                <div className="page-title">
                  <h1 id="unavailable-page">Page unavailable</h1>
                </div>
                <Empty
                  title="Return to project work"
                  action={
                    <Button asChild>
                      <Link
                        to="/projects/$projectId/$section"
                        params={{ projectId: project.id, section: "work" }}
                      >
                        Open Workspace
                      </Link>
                    </Button>
                  }
                >
                  This page is no longer available. Your project's work is
                  available from the navigation.
                </Empty>
              </section>
            )}
          </Suspense>
        </main>
      </div>
      {create && (
        <CreateProject
          onClose={() => setCreate(false)}
          onCreated={(p) => {
            setCreate(false);
            navigate({
              to: "/projects/$projectId/$section",
              params: { projectId: p.id, section: "work" },
            });
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
  if (section === "team") return <Delivery project={project} />;
  if (section === "settings") return <ProjectSettings project={project} />;
  if (section === "work") return <Work project={project} />;
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
  const close = () => {
    if (!mutation.isPending) onClose();
  };
  const canCreate = !!name.trim() && !mutation.isPending;
  return (
    <Modal
      title="Create project"
      onClose={close}
      closeOnEscape={!mutation.isPending}
      closeDisabled={mutation.isPending}
    >
      <Form
        onSubmit={() => {
          if (canCreate) mutation.mutate();
        }}
      >
        <p className="form-note">
          A project keeps its workers, conversations, memory and credentials
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
            disabled={mutation.isPending}
            onChange={(event) => {
              setName(event.target.value);
              mutation.reset();
            }}
          />
        </label>
        {mutation.isError && <ErrorNotice error={mutation.error} />}
        <div className="form-actions">
          <Button
            variant="outline"
            type="button"
            onClick={close}
            disabled={mutation.isPending}
          >
            Cancel
          </Button>
          <Button variant="default" disabled={!canCreate}>
            {mutation.isPending ? "Creating…" : "Create project"}
          </Button>
        </div>
      </Form>
    </Modal>
  );
}
