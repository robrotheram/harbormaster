import {
  Outlet,
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
  validateSearch: (search: Record<string, unknown>): { work?: string } => ({
    work: typeof search.work === "string" ? search.work : undefined,
  }),
  component: () => {
    const { projectId, section } = projectRoute.useParams();
    return <ProjectShell projectID={projectId} section={section} />;
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
    connectionsRoute,
    repositoriesRoute,
    managementRoute,
    infrastructureRoute,
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
