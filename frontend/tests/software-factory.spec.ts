import { test, expect, type Page } from "@playwright/test";
const timestamp = "2026-11-11T14:32:00Z";
const projects = [
  { id: "atlas", name: "Atlas platform", created_at: timestamp },
  { id: "sdk", name: "Developer SDK", created_at: timestamp },
  { id: "docs", name: "Documentation", created_at: timestamp },
];
const worker = (
  id: string,
  project_id: string,
  name: string,
  runtime_type: string,
  agent_kind = "codex",
  state = "READY",
) => ({
  id,
  project_id,
  name,
  runtime_type,
  agent_kind,
  state,
  ready: state === "READY" || state === "BUSY",
  active_runs: state === "BUSY" ? 1 : 0,
  background_jobs: 0,
  cpu_limit: 2,
  memory_limit_bytes: 2147483648,
  cpu_cores: null,
  memory_bytes: null,
  isolation_status: "unverified",
  network_status: "unverified",
  created_at: timestamp,
  updated_at: timestamp,
});
const workers = [
  worker(
    "ws_atlas",
    "atlas",
    "atlas / implementation",
    "kata",
    "codex",
    "BUSY",
  ),
  worker("ws_review", "atlas", "atlas / review", "kata"),
  worker("ws_hermes", "atlas", "Hermes", "firecracker", "hermes"),
  worker("ws_sdk", "sdk", "sdk / implementation", "kata", "codex", "BUSY"),
  worker("ws_docs", "docs", "docs / maintenance", "docker", "codex", "STOPPED"),
];
const work = [
  {
    id: "work_api",
    specification_path: "specs/api-pagination.md",
    state: "running",
  },
  {
    id: "work_auth",
    specification_path: "specs/session-refresh.md",
    state: "awaiting_review",
  },
  {
    id: "work_tests",
    specification_path: "specs/integration-tests.md",
    state: "queued",
  },
  {
    id: "work_release",
    specification_path: "specs/release-workflow.md",
    state: "accepted",
  },
  {
    id: "work_access",
    specification_path: "specs/accessibility-audit.md",
    state: "draft",
  },
  {
    id: "work_retry",
    specification_path: "specs/webhook-retries.md",
    state: "failed",
  },
  {
    id: "work_schema",
    specification_path: "specs/schema-validation.md",
    state: "accepted",
  },
].map((w) => ({
  ...w,
  project_id: "atlas",
  workspace_id: "ws_atlas",
  specification_digest: "sha256:fixture",
  kind: "software",
  created_at: timestamp,
  updated_at: timestamp,
}));
async function fixture(page: Page, failedProject?: string) {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.route("**/dashboard-api/v1/snapshot", (r) =>
    r.fulfill({
      json: {
        generated_at: timestamp,
        usage: {
          running_workspaces: 4,
          max_running_workspaces: 12,
          cpu_allocated: 8,
          memory_allocated_bytes: 8589934592,
        },
        services: {
          items: [
            {
              name: "controlplane",
              status: "healthy",
              reason_code: "available",
            },
            {
              name: "runtime-controller",
              status: "healthy",
              reason_code: "available",
            },
            {
              name: "inference-gateway",
              status: "healthy",
              reason_code: "available",
            },
            {
              name: "credential-broker",
              status: "degraded",
              reason_code: "upstream_unavailable",
            },
          ],
        },
      },
    }),
  );
  await page.route("**/ui-api/**", async (route) => {
    const url = new URL(route.request().url()),
      path = url.pathname,
      project = url.searchParams.get("project_id");
    let value: unknown = { items: [] };
    if (path === "/ui-api/projects") value = { items: projects };
    if (path === "/ui-api/workspaces") {
      if (project === failedProject)
        return route.fulfill({ status: 503, json: { code: "UNAVAILABLE" } });
      value = { items: workers.filter((w) => w.project_id === project) };
    }
    if (path === "/ui-api/work-items")
      value = { items: project === "atlas" ? work : [] };
    if (path.endsWith("/hermes")) {
      const id = path.split("/")[3];
      value = {
        config: {
          project_id: id,
          provider: "openai-codex",
          model: "project-model",
          mode: "workspace",
          revision: 1,
          matrix: null,
          connection_profile_id: "",
          updated_at: null,
        },
        saved: id === "atlas",
        runtime_available: true,
        runtime_status: "available",
        matrix_available: false,
        workspace_id: id === "atlas" ? "ws_hermes" : undefined,
        workspace_ready: id === "atlas",
      };
    }
    await route.fulfill({ json: value });
  });
  return errors;
}

test("project factory filters work and clears project-specific UI when switching", async ({
  page,
}) => {
  const errors = await fixture(page);
  await page.goto("/app/projects/atlas/work");
  await expect(
    page.getByRole("button", { name: /^api-pagination\.md / }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Filter threads by status" }).click();
  await page
    .getByRole("menuitemradio", { name: "Needs review (1)", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: /^session-refresh\.md / }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: /^api-pagination\.md / }),
  ).toHaveCount(0);
  await page
    .getByRole("button", { name: "Clear filters", exact: true })
    .click();
  await page.getByLabel("Search threads", { exact: true }).fill("schema");
  await expect(
    page.getByRole("button", {
      name: /^schema-validation\.md /,
    }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: /^api-pagination\.md / }),
  ).toHaveCount(0);
  await page
    .getByRole("button", { name: "Project assistant", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Work with Hermes", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toContainText(
    "Hermes in Atlas platform",
  );
  await page.keyboard.press("Escape");
  await page.keyboard.press("Escape");
  await page
    .getByRole("button", {
      name: "Switch project: Atlas platform",
      exact: true,
    })
    .click();
  await page
    .getByRole("menuitem", { name: "Developer SDK", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "What are we working on?" }),
  ).toBeVisible();
  await expect(page.getByLabel("Search threads", { exact: true })).toHaveValue(
    "",
  );
  await expect(
    page.getByRole("button", { name: "Work with Hermes", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByText("specs/schema-validation.md", { exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("button", { name: "Project assistant", exact: true })
    .click();
  await expect(
    page.getByRole("link", { name: "Set up Hermes", exact: true }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});

test("global infrastructure reports actual runtimes and filters across projects", async ({
  page,
}) => {
  const errors = await fixture(page);
  await page.goto("/app/infrastructure");
  await expect(
    page.getByRole("button", { name: "atlas / implementation", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "sdk / implementation", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("cell", { name: "Kata Containers", exact: true }).first(),
  ).toBeVisible();
  await page.getByLabel("Filter workers by project").selectOption("sdk");
  await expect(
    page.getByRole("button", { name: "atlas / implementation", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "sdk / implementation", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Clear filters", exact: true })
    .click();
  await page.getByLabel("Filter workers by runtime").selectOption("docker");
  await expect(
    page.getByRole("button", { name: "docs / maintenance", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "sdk / implementation", exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("button", { name: "docs / maintenance", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toContainText("unverified");
  await expect(page.getByRole("dialog")).toContainText("Docker");
  await page
    .getByRole("link", { name: "Manage worker & desktop connection" })
    .click();
  await expect(page).toHaveURL(/projects\/docs\/workspaces$/);
  expect(errors).toEqual([]);
});

test("failed project observations remain explicitly partial", async ({
  page,
}) => {
  await fixture(page, "sdk");
  await page.goto("/app/infrastructure");
  await expect(page.getByRole("alert")).toContainText(
    "Worker inventory unavailable for Developer SDK",
  );
  await expect(
    page.getByText("Partial inventory", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("cell", { name: "Kata Containers", exact: true }).first(),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "sdk / implementation", exact: true }),
  ).toHaveCount(0);
});

test("factory layout review at desktop and mobile widths", async ({ page }) => {
  const errors = await fixture(page);
  await page.setViewportSize({ width: 1440, height: 900 });
  for (const [name, path, ready] of [
    ["projects", "/app/", "Projects"],
    ["project-work", "/app/projects/atlas/work", "Project work"],
    ["infrastructure", "/app/infrastructure", "Infrastructure"],
    ["services", "/app/platform-services", "Platform services"],
  ]) {
    await page.goto(path);
    await expect(
      page.getByRole("heading", { name: ready, exact: true }),
    ).toBeVisible();
    if (name === "project-work")
      await expect(
        page.getByRole("button", { name: "Project assistant", exact: true }),
      ).toBeVisible();
    if (name === "infrastructure")
      await expect(
        page
          .getByRole("cell", { name: "Kata Containers", exact: true })
          .first(),
      ).toBeVisible();
    if (name === "services")
      await expect(
        page.getByRole("heading", { name: "controlplane", exact: true }),
      ).toBeVisible();
    if (name === "projects")
      await expect(
        page.locator(".project-inventory-row").first(),
      ).toContainText("7 work items");
    await page.evaluate(() => document.fonts.ready);
    await page.screenshot({
      path: `artifacts/factory-${name}-desktop.png`,
      fullPage: true,
    });
    await page.setViewportSize({ width: 390, height: 844 });
    await expect(page.locator(".factory-body")).toHaveCSS(
      "padding-left",
      "0px",
    );
    await page.screenshot({
      path: `artifacts/factory-${name}-mobile.png`,
      fullPage: true,
    });
    const overflow = await page.evaluate(() =>
      [...document.querySelectorAll("body *")]
        .filter(
          (e) =>
            e.getBoundingClientRect().right > innerWidth &&
            getComputedStyle(e).position !== "fixed",
        )
        .map((e) => ({
          tag: e.tagName,
          class: e.className,
          width: e.getBoundingClientRect().width,
        }))
        .slice(0, 12),
    );
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
      JSON.stringify(overflow),
    ).toBe(true);
    await page.setViewportSize({ width: 1440, height: 900 });
  }
  await page.getByRole("button", { name: "Switch to light theme" }).click();
  await page.screenshot({
    path: "artifacts/factory-infrastructure-light.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "Open navigation" }).click();
  await page
    .getByRole("dialog")
    .getByRole("link", { name: "Workspace", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Project work", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  for (const width of [320, 768, 1024, 1280]) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto("/app/projects/atlas/work");
    if (width < 1200)
      await page
        .getByRole("button", { name: "Switch thread", exact: true })
        .click();
    await expect(
      page.getByRole("button", {
        name: /^api-pagination\.md /,
      }),
    ).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
      `Project work at ${width}px`,
    ).toBe(true);
    if (width < 1200) await page.keyboard.press("Escape");
    await page.goto("/app/infrastructure");
    await expect(
      page.getByRole("cell", { name: "Kata Containers", exact: true }).first(),
    ).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
      `Infrastructure at ${width}px`,
    ).toBe(true);
  }
  expect(errors).toEqual([]);
});

test("headers align in both sidebar states and services have a separate page", async ({
  page,
}) => {
  const errors = await fixture(page);
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/app/projects/atlas/work");
  await expect(
    page.getByRole("heading", { name: "Project work", exact: true }),
  ).toBeVisible();
  for (const state of ["expanded", "collapsed"]) {
    if (state === "collapsed")
      await page
        .getByRole("button", { name: "Collapse sidebar", exact: true })
        .click();
    await expect(page.locator(".factory-sidebar")).toHaveCSS(
      "width",
      state === "collapsed" ? "64px" : "228px",
    );
    const brand = await page
      .locator(".factory-sidebar .factory-brand-row")
      .boundingBox();
    const header = await page.locator(".factory-header").boundingBox();
    const toggle = page.locator("#factory-sidebar .sidebar-collapse");
    await expect(toggle).toBeVisible();
    await expect(
      page
        .locator(".factory-header")
        .getByRole("button", { name: /Collapse sidebar|Expand sidebar/ }),
    ).toHaveCount(0);
    const toggleBounds = (await toggle.boundingBox())!;
    const railBounds = (await page.locator("#factory-sidebar").boundingBox())!;
    expect(toggleBounds.x).toBeGreaterThanOrEqual(railBounds.x);
    expect(toggleBounds.x + toggleBounds.width).toBeLessThanOrEqual(
      railBounds.x + railBounds.width,
    );
    expect(toggleBounds.y).toBeGreaterThanOrEqual(brand!.y);
    expect(toggleBounds.y + toggleBounds.height).toBeLessThanOrEqual(
      brand!.y + brand!.height,
    );
    await expect(
      page.locator("#factory-sidebar .rail-bottom .sidebar-collapse"),
    ).toHaveCount(0);
    await expect(
      page.getByLabel("Current project", { exact: true }),
    ).toHaveCount(0);
    expect(brand!.y + brand!.height).toBe(header!.y + header!.height);

    await page.screenshot({
      path: `artifacts/shell-${state}-header.png`,
      clip: { x: 0, y: 0, width: 1440, height: 150 },
    });
  }
  await page.setViewportSize({ width: 1280, height: 600 });
  await expect(
    page.getByRole("button", { name: "Expand sidebar", exact: true }),
  ).toBeInViewport();
  await page
    .getByRole("button", { name: "Expand sidebar", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Collapse sidebar", exact: true }),
  ).toBeInViewport();
  await page
    .getByRole("button", { name: "Collapse sidebar", exact: true })
    .click();
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.getByRole("button", { name: "Platform", exact: true }).click();
  await page
    .getByRole("menuitem", { name: "Infrastructure", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "atlas / implementation", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Execution architecture", { exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("heading", { name: "Platform services", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByText("credential-broker", { exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Platform", exact: true }).click();
  await page.getByRole("menuitem", { name: "Health", exact: true }).click();
  await expect(page).toHaveURL(/\/app\/platform-services$/);
  await expect(
    page.getByRole("heading", { name: "Platform services", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "credential-broker", exact: true }),
  ).toBeVisible();
  await expect(page.getByText("Degraded", { exact: true })).toBeVisible();
  await page.reload();
  await expect(
    page.getByRole("button", { name: "Expand sidebar", exact: true }),
  ).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await page
    .getByRole("button", { name: "Open navigation", exact: true })
    .click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Platform", exact: true })
    .click();
  await expect(
    page.getByRole("menuitem", { name: "Health", exact: true }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});

test("project sections stay in the sidebar and threads belong only to Workspace", async ({
  page,
}) => {
  const errors = await fixture(page);
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/app/projects/atlas/work");
  const sidebar = page.locator("#factory-sidebar");
  const projectNav = sidebar.getByRole("navigation", {
    name: "Project navigation",
  });
  await expect(
    projectNav.getByRole("link", { name: "Workspace", exact: true }),
  ).toHaveAttribute("aria-current", "page");
  const initial = (await projectNav.boundingBox())!;
  await expect(page.locator(".project-tabs")).toHaveCount(0);
  await expect(page.locator(".conversation-thread-rail")).toBeVisible();
  for (const [label, heading] of [
    ["Workers", "Workers"],
    ["Settings", "Project settings"],
  ]) {
    await projectNav.getByRole("link", { name: label, exact: true }).click();
    await expect(
      page.getByRole("heading", { name: heading, exact: true }),
    ).toBeVisible();
    await expect(page.locator(".conversation-thread-rail")).toHaveCount(0);
    await expect(
      page.getByRole("button", {
        name: /Show threads|Hide threads|Switch thread/,
      }),
    ).toHaveCount(0);
    const current = (await projectNav.boundingBox())!;
    expect(current).toEqual(initial);
    await page.screenshot({
      path: `artifacts/project-sidebar-${label.toLowerCase()}.png`,
    });
  }
  await page.setViewportSize({ width: 390, height: 844 });
  await page
    .getByRole("button", { name: "Open navigation", exact: true })
    .click();
  const drawer = page.getByRole("dialog", {
    name: "Factory navigation",
    exact: true,
  });
  await drawer.getByRole("link", { name: "Workers", exact: true }).click();
  await expect(drawer).toHaveCount(0);
  await expect(
    page.getByRole("heading", { name: "Workers", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Open navigation", exact: true })
    .click();
  await drawer
    .getByRole("button", {
      name: "Switch project: Atlas platform",
      exact: true,
    })
    .click();
  await page
    .getByRole("menuitem", { name: "Developer SDK", exact: true })
    .click();
  await expect(page).toHaveURL(/projects\/sdk\/workspaces$/);
  await expect(drawer).toHaveCount(0);
  await page
    .getByRole("button", { name: "Open navigation", exact: true })
    .click();
  await drawer.getByRole("link", { name: "Workspace", exact: true }).click();
  await page
    .getByRole("button", { name: "Switch thread", exact: true })
    .click();
  await expect(
    page.getByRole("dialog", { name: "Threads", exact: true }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});

test("project-first sidebar stays fixed on global pages and remembers the project", async ({
  page,
}) => {
  const errors = await fixture(page);
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/app/projects/sdk/workspaces");
  const sidebar = page.locator("#factory-sidebar");
  const switcher = sidebar.getByRole("button", {
    name: "Switch project: Developer SDK",
    exact: true,
  });
  await expect(switcher).toBeVisible();
  const measure = () =>
    sidebar
      .locator(
        ".project-switcher, .project-section-list, .project-global-links, .rail-bottom",
      )
      .evaluateAll((elements) =>
        elements.map((el) => {
          const { x, y, width, height } = el.getBoundingClientRect();
          return { x, y, width, height };
        }),
      );
  for (const collapsed of [false, true]) {
    if (collapsed) {
      await page
        .getByRole("button", { name: "Collapse sidebar", exact: true })
        .click();
      await expect(sidebar).toHaveCSS("width", "64px");
    }
    const before = await measure();
    for (const [label, heading] of [
      ["Infrastructure", "Infrastructure"],
      ["Projects", "Projects"],
      ["Platform services", "Platform services"],
      ["Network profiles", "Network profiles"],
    ]) {
      if (label === "Projects") {
        await sidebar.getByRole("link", { name: label, exact: true }).click();
      } else {
        await sidebar
          .getByRole("button", { name: "Platform", exact: true })
          .click();
        await page
          .getByRole("menuitem", {
            name: label === "Platform services" ? "Health" : label,
            exact: true,
          })
          .click();
      }
      await expect(
        page.getByRole("heading", { name: heading, exact: true }),
      ).toBeVisible();
      await expect(switcher).toBeVisible();
      expect(await measure()).toEqual(before);
      await expect(
        sidebar.locator('.project-section-list [aria-current="page"]'),
      ).toHaveCount(0);
      await expect(sidebar.locator("nav .is-active")).toHaveCount(1);
      if (label !== "Projects")
        await expect(
          sidebar.getByRole("button", { name: "Platform", exact: true }),
        ).toHaveClass(/is-active/);
      await expect(
        sidebar.getByRole("link", { name: "Workspace", exact: true }),
      ).toHaveAttribute("href", /projects\/sdk\/work$/);
      if (label === "Infrastructure")
        await page.screenshot({
          path: `artifacts/stable-sidebar-${collapsed ? "collapsed" : "expanded"}.png`,
        });
    }
    await page.reload();
    await expect(switcher).toBeVisible();
    expect(await measure()).toEqual(before);
    await sidebar.getByRole("link", { name: "Workspace", exact: true }).click();
    await expect(page).toHaveURL(/projects\/sdk\/work$/);
    await expect(
      page.getByRole("heading", { name: "Project work", exact: true }),
    ).toBeVisible();
    expect(await measure()).toEqual(before);
  }
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "Open navigation" }).click();
  const drawer = page.getByRole("dialog", { name: "Factory navigation" });
  await drawer.getByRole("button", { name: "Platform", exact: true }).click();
  await page
    .getByRole("menuitem", { name: "Infrastructure", exact: true })
    .click();
  await expect(drawer).toHaveCount(0);
  await page.getByRole("button", { name: "Open navigation" }).click();
  await expect(
    drawer.getByRole("button", { name: "Switch project: Developer SDK" }),
  ).toBeVisible();
  await drawer.getByRole("link", { name: "Workspace", exact: true }).click();
  await expect(page).toHaveURL(/projects\/sdk\/work$/);
  expect(errors).toEqual([]);
});

test("sidebar recovers from a removed project and keeps its layout when none exist", async ({
  page,
}) => {
  await fixture(page);
  await page.addInitScript(() =>
    localStorage.setItem("workbench.last-project", "removed-project"),
  );
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/app/infrastructure");
  const sidebar = page.locator("#factory-sidebar");
  await expect(
    sidebar.getByRole("button", { name: "Switch project: Atlas platform" }),
  ).toBeVisible();
  await expect(sidebar.locator('a[href*="removed-project"]')).toHaveCount(0);
  const globals = (await sidebar
    .locator(".project-global-links")
    .boundingBox())!;
  await page.route("**/ui-api/projects", (route) =>
    route.fulfill({ json: { items: [] } }),
  );
  await page.reload();
  await expect(
    sidebar.getByRole("button", { name: "New project", exact: true }),
  ).toBeVisible();
  await expect(
    sidebar.locator('.project-section-list [aria-disabled="true"]'),
  ).toHaveCount(8);
  await expect(sidebar.locator(".project-section-list a")).toHaveCount(0);
  expect(
    (await sidebar.locator(".project-global-links").boundingBox())!,
  ).toEqual(globals);
  await sidebar
    .getByRole("button", { name: "New project", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toBeVisible();
});

test("system tools share one menu and Docs sits beside the theme switcher", async ({
  page,
}) => {
  const errors = await fixture(page);
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/app/projects/atlas/work");
  const sidebar = page.locator("#factory-sidebar");
  const header = page.locator(".factory-header");
  const docs = header.getByRole("link", { name: "Docs", exact: true });
  const theme = header.getByRole("button", {
    name: "Switch to light theme",
    exact: true,
  });
  await expect(docs).toBeVisible();
  await expect(
    sidebar.getByRole("link", { name: /Docs|Getting started/ }),
  ).toHaveCount(0);
  await expect(sidebar.locator(".rail-health")).toHaveCount(0);
  const system = sidebar.getByRole("button", { name: "Platform", exact: true });
  await system.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("menuitem")).toHaveCount(4);
  await expect(
    page.getByRole("menuitem", { name: "Connections", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("menuitem", { name: "Infrastructure", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("menuitem", { name: "Network profiles", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("menuitem", { name: "Health", exact: true }),
  ).toBeVisible();
  await expect(
    page
      .getByRole("menuitem", { name: "Health", exact: true })
      .locator(".system-health-badge"),
  ).toHaveText("Degraded");
  await page.screenshot({
    path: "artifacts/system-menu-desktop.png",
    animations: "disabled",
  });
  await page.keyboard.press("Escape");
  await expect(system).toBeFocused();
  await docs.click();
  await expect(page).toHaveURL(/\/app\/help$/);
  await expect(
    page.getByRole("heading", { name: "Getting started", exact: true }),
  ).toBeVisible();
  for (const width of [1440, 390, 320]) {
    await page.setViewportSize({ width, height: 844 });
    await expect(docs).toBeInViewport();
    const docBounds = (await docs.boundingBox())!;
    const themeBounds = (await theme.boundingBox())!;
    expect(docBounds.x + docBounds.width).toBeLessThanOrEqual(themeBounds.x);
    expect(themeBounds.x - docBounds.x - docBounds.width).toBeLessThanOrEqual(
      10,
    );
    expect(
      Math.abs(
        docBounds.y +
          docBounds.height / 2 -
          themeBounds.y -
          themeBounds.height / 2,
      ),
    ).toBeLessThan(1);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
  }
  await page.screenshot({ path: "artifacts/docs-mobile.png" });
  await theme.click();
  await expect(
    header.getByRole("button", { name: "Switch to dark theme" }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});

test("health badge reports healthy and unknown observations without guessing", async ({
  page,
}) => {
  const errors = await fixture(page);
  await page.setViewportSize({ width: 1440, height: 900 });
  let response: { status: number; json: unknown } = {
    status: 200,
    json: {
      services: { items: [{ name: "Control plane", status: "healthy" }] },
    },
  };
  await page.route("**/dashboard-api/v1/snapshot", (route) =>
    route.fulfill(response),
  );
  for (const [status, json, expected] of [
    [
      200,
      { services: { items: [{ name: "Control plane", status: "healthy" }] } },
      "Healthy",
    ],
    [
      200,
      {
        services: {
          items: [
            { name: "Control plane", status: "healthy" },
            { name: "Gateway", status: "unknown" },
          ],
        },
      },
      "Unknown",
    ],
    [200, { services: { items: [] } }, "Unknown"],
    [503, { code: "UNAVAILABLE" }, "Unknown"],
  ] as const) {
    response = { status, json };
    await page.goto("/app/projects/atlas/work");
    await page.getByRole("button", { name: "Platform", exact: true }).click();
    const health = page.getByRole("menuitem", { name: "Health", exact: true });
    await expect(health.locator(".system-health-badge")).toHaveText(expected);
    await expect(health).toHaveAttribute(
      "aria-description",
      `Service health: ${expected.toLowerCase()}`,
    );
  }
  expect(errors).toEqual([]);
});

test("thread filter options stay readable in both themes and support keyboard selection", async ({
  page,
}) => {
  const errors = await fixture(page);
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/app/projects/atlas/work");
  const filter = page.getByRole("button", { name: "Filter threads by status" });
  for (const theme of ["dark", "light"]) {
    if (theme === "light")
      await page.getByRole("button", { name: "Switch to light theme" }).click();
    await filter.click();
    const menu = page.locator(".thread-filter-options");
    await expect(menu).toBeVisible();
    const contrast = await menu.evaluate((element) => {
      const canvas = document.createElement("canvas");
      canvas.width = canvas.height = 1;
      const context = canvas.getContext("2d")!;
      const luminance = (color: string) => {
        context.clearRect(0, 0, 1, 1);
        context.fillStyle = color;
        context.fillRect(0, 0, 1, 1);
        const rgb = [...context.getImageData(0, 0, 1, 1).data]
          .slice(0, 3)
          .map((value) => {
            const s = value / 255;
            return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
          });
        return rgb[0] * 0.2126 + rgb[1] * 0.7152 + rgb[2] * 0.0722;
      };
      const background = luminance(getComputedStyle(element).backgroundColor);
      return [...element.querySelectorAll('[role="menuitemradio"]')].map(
        (option) => {
          const foreground = luminance(getComputedStyle(option).color);
          return (
            (Math.max(background, foreground) + 0.05) /
            (Math.min(background, foreground) + 0.05)
          );
        },
      );
    });
    expect(Math.min(...contrast)).toBeGreaterThanOrEqual(4.5);
    await page.screenshot({
      path: `artifacts/thread-filter-${theme}.png`,
      animations: "disabled",
    });
    await page.keyboard.press("Escape");
    await expect(filter).toBeFocused();
  }
  await filter.focus();
  await page.keyboard.press("Enter");
  await expect(
    page.getByRole("menuitemradio", { name: "All threads (7)", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("ArrowDown");
  await expect(
    page.getByRole("menuitemradio", { name: "Planned (2)", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(filter).toContainText("Planned (2)");
  await expect(
    page.getByRole("button", { name: /^integration-tests\.md / }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: /^api-pagination\.md / }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: /Show threads|Hide threads/ }),
  ).toHaveCount(0);
  expect(errors).toEqual([]);
});
