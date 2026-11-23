import {
  Outlet,
  redirect,
  createRootRoute,
  createRoute,
  createRouter,
} from "@tanstack/react-router";
import { ProjectShell } from "./shell";
import { PageAccessibility } from "./page-accessibility";
import { RouteRecovery } from "./route-recovery";
const rootRoute = createRootRoute({
  component: () => (
    <>
      <PageAccessibility />
      <Outlet />
    </>
  ),
  notFoundComponent: () => <RouteRecovery missing />,
  errorComponent: () => <RouteRecovery />,
});
const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: () => <ProjectShell />,
});
const projectRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/projects/$projectId/$section",
  validateSearch: (
    search: Record<string, unknown>,
  ): {
    work?: string;
    attempt?: string;
    view?: "workspace" | "files";
    worker?: string;
    tab?: "general" | "hermes" | "audit";
  } => ({
    view:
      search.view === "workspace" || search.view === "files"
        ? search.view
        : undefined,
    tab:
      search.tab === "hermes" || search.tab === "audit"
        ? search.tab
        : undefined,
    attempt: typeof search.attempt === "string" ? search.attempt : undefined,
    work: typeof search.work === "string" ? search.work : undefined,
    worker: typeof search.worker === "string" ? search.worker : undefined,
  }),
  beforeLoad: ({ params, search }) => {
    if (params.section === "reviews" || params.section === "factory")
      throw redirect({
        to: "/projects/$projectId/$section",
        params: { projectId: params.projectId, section: "work" },
        search: {
          work: search.work,
          attempt: search.attempt,
          view: "workspace",
        },
        replace: true,
      });
    if (params.section === "hermes")
      throw redirect({
        to: "/projects/$projectId/$section",
        params: { projectId: params.projectId, section: "settings" },
        search: { tab: "hermes" },
        replace: true,
      });
  },
  component: () => {
    const { projectId, section } = projectRoute.useParams();
    return <ProjectShell projectID={projectId} section={section} />;
  },
});
const workerRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/projects/$projectId/workspaces/$workerId",
  validateSearch: (
    search: Record<string, unknown>,
  ): {
    panel?: "environment" | "settings" | "activity";
    tab?: "subscriptions";
  } => ({
    tab:
      search.panel === "settings" && search.tab === "subscriptions"
        ? "subscriptions"
        : undefined,
    panel:
      search.panel === "setup" || search.panel === "environment"
        ? "environment"
        : search.panel === "settings" || search.panel === "activity"
          ? search.panel
          : undefined,
  }),
  component: () => {
    const { projectId, workerId } = workerRoute.useParams();
    const { panel } = workerRoute.useSearch();
    return (
      <ProjectShell
        projectID={projectId}
        section="workspaces"
        workerID={workerId}
        workerPanel={panel}
      />
    );
  },
});
const connectionsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/connection-profiles",
  component: () => <ProjectShell globalSettings />,
});
const managementRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/connections",
  validateSearch: (
    search: Record<string, unknown>,
  ): { tab?: "agents" | "repositories" } => ({
    tab: search.tab === "repositories" ? "repositories" : "agents",
  }),
  component: () => <ProjectShell globalPage="connections" />,
});
const repositoriesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/repositories",
  component: () => <ProjectShell globalPage="repositories" />,
});
const infrastructureRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/infrastructure",
  component: () => <ProjectShell globalPage="infrastructure" />,
});
const workersRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workers",
  component: () => <ProjectShell globalPage="infrastructure" />,
});
const servicesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/platform-services",
  component: () => <ProjectShell globalPage="services" />,
});
const helpRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/help",
  component: () => <ProjectShell globalPage="help" />,
});
export const router = createRouter({
  routeTree: rootRoute.addChildren([
    indexRoute,
    projectRoute,
    workerRoute,
    connectionsRoute,
    repositoriesRoute,
    managementRoute,
    infrastructureRoute,
    workersRoute,
    servicesRoute,
    helpRoute,
  ]),
  basepath: "/app",
});
declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
