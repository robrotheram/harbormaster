import {
  Link,
  Outlet,
  createRootRoute,
  createRoute,
  createRouter,
} from "@tanstack/react-router";
import { ProjectShell } from "./shell";
const rootRoute = createRootRoute({
  component: () => <Outlet />,
  notFoundComponent: () => (
    <div className="standalone">
      <h1>Page unavailable</h1>
      <p>Choose a project to return to the workbench.</p>
      <Link to="/">Open workbench</Link>
    </div>
  ),
  errorComponent: () => (
    <div className="standalone">
      <h1>The workbench could not load</h1>
      <p>Reload to retrieve the current project state.</p>
      <a href="/app/">Reload workbench</a>
    </div>
  ),
});
const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: () => <ProjectShell />,
});
const projectRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/projects/$projectId/$section",
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
const toolsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/tool-profiles",
  component: () => <ProjectShell globalSettings="tools" />,
});
export const router = createRouter({
  routeTree: rootRoute.addChildren([
    indexRoute,
    projectRoute,
    connectionsRoute,
    toolsRoute,
  ]),
  basepath: "/app",
});
declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
