const { test } = require("@playwright/test");
const assert = require("node:assert/strict");
const path = require("node:path");
const fs = require("node:fs");
const base = process.env.UI_TEST_URL || "http://127.0.0.1:4173/app/";
const root = path.resolve(__dirname, "..");
const workspace = (id, project, name) => ({
  id: "ws_" + id.repeat(32),
  project_id: project,
  name,

  agent_kind: "t3",
  home_id: "home_" + id,
  ready: true,
  state: "READY",
  active_runs: 0,
  background_jobs: 0,
  runtime_type: "docker",
  isolation_status: "unverified",
  network_status: "unverified",
  cpu_limit: 1,
  memory_limit_bytes: 1073741824,
  memory_bytes: 1234567,
  created_at: "2026-11-01T10:00:00Z",
  updated_at: "2026-11-01T10:00:00Z",
});
const projects = [
  { id: "p_a", name: "Research", created_at: "2026-11-01T10:00:00Z" },
  { id: "p_b", name: "Banking", created_at: "2026-11-01T10:00:00Z" },
  {
    id: "default",
    name: "Workspaces",
    created_at: "2026-11-01T10:00:00Z",
  },
];
const workspaces = {
  p_a: [
    workspace("1", "p_a", "Research worker A"),
    workspace("2", "p_a", "Research branch"),
  ],
  p_b: [workspace("3", "p_b", "Banking worker")],
  default: [],
};
const schedules = { p_a: [], p_b: [], default: [] };
test("project workflows preserve boundaries and UI behavior", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 1050 });
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  let requests = [];
  let taskState = "queued";
  await page.route("**/ui-api/**", async (route) => {
    const req = route.request(),
      url = new URL(req.url()),
      p = url.searchParams.get("project_id");
    requests.push({ path: url.pathname, project: p, method: req.method() });
    let status = 200,
      value = {};
    const body = req.method() === "POST" ? req.postDataJSON() : {};
    if (url.pathname === "/ui-api/projects") {
      if (req.method() === "POST") {
        value = {
          id: "p_new",
          name: body.name,
          created_at: new Date().toISOString(),
        };
        projects.push(value);
        status = 201;
        workspaces.p_new = [];
        schedules.p_new = [];
      } else value = { items: projects };
    } else if (url.pathname === "/ui-api/connection-profiles")
      value = {
        items: [
          {
            id: "github",
            name: "GitHub only",
            description: "Approved Git access",
            revision: 1,
            policy: { mode: "allowlist", destinations: ["github.com:443"] },
            updated_at: "2026-11-01T10:00:00Z",
          },
        ],
      };
    else if (url.pathname === "/ui-api/workspaces") {
      if (req.method() === "POST") {
        assert.equal(body.project_id, "p_a");
        assert.equal(
          Object.hasOwn(body, "profile_id"),
          false,
          "workspace creation must not send a tool profile",
        );
        value = {
          ...workspace(
            String(workspaces.p_a.length + 2),
            body.project_id,
            body.name,
          ),
          agent_kind: body.agent_kind,
          parent_id: body.parent_id,
          branch: body.branch,
        };
        workspaces.p_a.push(value);
        status = 201;
      } else {
        if (p === "p_b")
          await new Promise((resolve) => setTimeout(resolve, 150));
        value = { items: workspaces[p] || [] };
      }
    } else if (url.pathname === "/ui-api/schedules") {
      if (req.method() === "POST") {
        value = {
          ...body,
          id: "sched_1",
          enabled: true,
          next_run_at: body.run_at,
          created_at: new Date().toISOString(),
        };
        schedules[body.project_id].push(value);
        status = 201;
      } else value = { items: schedules[p] || [] };
    } else if (url.pathname === "/ui-api/runs") value = { items: [] };
    else if (url.pathname === "/ui-api/file-handoffs") {
      assert.equal(body.source_id, workspaces.p_a[0].id);
      assert.equal(body.destination_id, workspaces.p_a[1].id);
      value = { copied: true, bytes: 42 };
      status = 201;
    } else if (url.pathname.endsWith("/desktop-pair"))
      value = {
        pairing_url:
          "http://desktop-" +
          workspaces.p_a[0].id.replace("ws_", "ws-") +
          ".localhost:8080/pair#token=test-one-use-link",
        expires_at: "2026-11-01T12:00:00Z",
      };
    else if (url.pathname.endsWith("/settings")) {
      const id = url.pathname.split("/")[3];
      const w = Object.values(workspaces)
        .flat()
        .find((w) => w.id === id);
      assert(w);
      if (body.connection_profile_id) {
        w.connection_profile_id = body.connection_profile_id;
        w.connection_profile_revision = 1;
        w.connections = {
          mode: "allowlist",
          destinations: ["github.com:443"],
        };
      } else {
        w.connections = body.connections;
      }
      value = w;
    } else if (url.pathname.endsWith("/start"))
      value = { ...workspaces.p_a[0], console_ticket: "test-ticket" };
    else if (url.pathname.endsWith("/cancel")) {
      taskState = "cancelled";
      value = { id: "task_1", state: taskState };
    } else if (url.pathname.endsWith("/result"))
      value = {
        content: "<script>window.untrustedResultExecuted=true</script>",
        next_offset: -1,
      };
    else if (url.pathname.includes("/tasks"))
      value = { id: "task_1", state: taskState };
    else if (url.pathname.includes("/schedules/")) value = { accepted: true };
    else {
      status = 404;
      value = { code: "NOT_FOUND_OR_FORBIDDEN" };
    }
    await route.fulfill({
      status,
      contentType: "application/json",
      body: JSON.stringify(value),
    });
  });
  await page.goto(new URL("/app/projects/p_a/workspaces", base).toString());
  await page.getByText("Research worker A", { exact: true }).waitFor();
  assert.equal(
    await page
      .getByRole("link", { name: "Tool profiles", exact: true })
      .count(),
    0,
  );
  assert.equal(
    await page.getByRole("heading", { name: "Workers", exact: true }).count(),
    1,
  );
  await page
    .getByRole("button", { name: "Switch project: Research", exact: true })
    .click();
  await page.getByRole("menuitem", { name: "Banking", exact: true }).click();
  assert.equal(
    await page.getByText("Research worker A", { exact: true }).count(),
    0,
  );
  await page.getByText("Banking worker", { exact: true }).waitFor();
  assert.equal(
    await page.getByText("Research worker A", { exact: true }).count(),
    0,
  );
  await page
    .getByRole("button", { name: "Switch project: Banking", exact: true })
    .click();
  await page.getByRole("menuitem", { name: "Research", exact: true }).click();
  await page.getByText("Research worker A", { exact: true }).waitFor();
  await page
    .getByRole("button", { name: "Create worker", exact: true })
    .click();
  assert.equal(
    await page.getByLabel("Tool profile", { exact: true }).count(),
    0,
  );
  await page.keyboard.press("Escape");
  await page.getByRole("dialog").waitFor({ state: "detached" });
  await page.waitForFunction(() =>
    document.activeElement?.textContent?.includes("Create worker"),
  );
  await page
    .getByRole("button", { name: "Create worker", exact: true })
    .click();
  await page
    .getByLabel("Worker name", { exact: true })
    .fill("New project workspace");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Create worker", exact: true })
    .click();
  await page.getByText("New project workspace", { exact: true }).waitFor();
  await page
    .getByRole("link", { name: "Open worker Research worker A", exact: true })
    .click();
  await page
    .getByRole("navigation", { name: "Worker sections" })
    .getByRole("link", { name: "Settings", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Branch worker", exact: true })
    .click();
  assert.equal(
    await page.getByLabel("Tool profile", { exact: true }).count(),
    0,
  );
  assert.equal(
    await page.getByLabel("Agent", { exact: true }).isDisabled(),
    true,
  );
  await page.getByLabel(/New local Git branch/).fill("feature-cleanup");
  await page
    .getByRole("button", { name: "Create branch worker", exact: true })
    .click();
  await page.getByText("Worker branched.", { exact: true }).waitFor();

  await page
    .getByRole("button", { name: "Connect T3 desktop", exact: true })
    .click();
  await page.getByLabel("Private pairing URL").waitFor();
  assert(
    (await page.getByLabel("Private pairing URL").inputValue()).includes(
      "#token=",
    ),
  );
  await page.keyboard.press("Escape");
  await page
    .getByRole("navigation", { name: "Project navigation" })
    .getByRole("link", { name: "Workers", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Project actions", exact: true })
    .click();
  await page
    .getByRole("menuitem", { name: "Copy a file", exact: true })
    .click();
  await page
    .getByLabel("From worker", { exact: true })
    .selectOption(workspaces.p_a[0].id);
  await page
    .getByLabel("Source file", { exact: true })
    .fill("reports/finding.md");
  await page
    .getByLabel("To worker", { exact: true })
    .selectOption(workspaces.p_a[1].id);
  await page
    .getByLabel("New destination file", { exact: true })
    .fill("incoming/finding.md");
  await page.getByRole("button", { name: "Copy file", exact: true }).click();
  await page.getByText(/File copied \(42 B\)/).waitFor();
  assert.equal(
    await page.getByRole("link", { name: "Agent homes", exact: true }).count(),
    0,
  );
  assert.equal(
    await page.getByRole("link", { name: "Schedules", exact: true }).count(),
    0,
    "Removed scheduling pages must not remain in navigation",
  );
  await page
    .getByRole("navigation", { name: "Project navigation" })
    .getByRole("link", { name: "Workers", exact: true })
    .click();
  await page.getByText("Research worker A", { exact: true }).waitFor();

  assert.equal(
    await page.getByRole("menuitem", { name: "Run task", exact: true }).count(),
    0,
    "Workers must not offer the removed task action",
  );
  await page.getByLabel("Search workers").fill("no-matching-workspace");
  await page
    .getByText("No workers match this filter.", { exact: false })
    .waitFor();
  await page
    .getByRole("button", { name: "Clear filters", exact: true })
    .click();
  await page.getByText("Research worker A", { exact: true }).waitFor();
  await page
    .getByRole("link", { name: "Open worker Research worker A", exact: true })
    .click();
  await page
    .getByRole("navigation", { name: "Worker sections" })
    .getByRole("link", { name: "Settings", exact: true })
    .click();
  await page
    .getByRole("region", { name: "Worker settings" })
    .getByLabel("Global connection profile", { exact: true })
    .selectOption("github");
  await page
    .getByRole("button", { name: "Save settings", exact: true })
    .click();
  await page.getByText("Worker settings saved.", { exact: true }).waitFor();
  await page.getByRole("button", { name: "Platform", exact: true }).click();
  await page
    .getByRole("menuitem", { name: "Network profiles", exact: true })
    .click();
  await page
    .getByRole("heading", { name: "Network profiles", exact: true })
    .waitFor();
  await page.getByRole("button", { name: "Edit profile", exact: true }).click();
  await page
    .getByRole("dialog")
    .getByLabel("Connection mode", { exact: true })
    .selectOption("open");
  await page
    .getByText("Fully open allows Internet", { exact: false })
    .waitFor();
  await page.keyboard.press("Escape");
  await page.getByRole("dialog").waitFor({ state: "detached" });
  await page.getByRole("button", { name: /^Switch project:/ }).click();
  await page.getByRole("menuitem", { name: "Research", exact: true }).click();
  await page
    .getByRole("navigation", { name: "Project navigation" })
    .getByRole("link", { name: "Workers", exact: true })
    .click();
  await page.getByText("Research worker A", { exact: true }).waitFor();
  fs.mkdirSync(path.join(root, "artifacts"), { recursive: true });
  assert(
    await page.evaluate(() =>
      document.documentElement.classList.contains("dark"),
    ),
    "dark theme should be the default",
  );
  await page
    .getByRole("button", { name: "Switch to light theme", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Switch to dark theme", exact: true })
    .click();
  assert(
    await page.evaluate(() =>
      document.documentElement.classList.contains("dark"),
    ),
  );
  await page.reload();
  await page.getByText("Research worker A", { exact: true }).waitFor();
  assert(
    await page.evaluate(() =>
      document.documentElement.classList.contains("dark"),
    ),
    "theme did not persist",
  );
  await page.screenshot({
    path: path.join(root, "artifacts/workbench-dark.png"),
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "Switch to light theme", exact: true })
    .click();
  await page.evaluate(() => document.fonts.ready);
  fs.mkdirSync(path.join(root, "artifacts"), { recursive: true });
  await page.screenshot({
    path: path.join(root, "artifacts/workbench-desktop.png"),
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({
    path: path.join(root, "artifacts/workbench-mobile.png"),
    fullPage: true,
  });
  assert.equal(
    await page.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth,
    ),
    false,
    "mobile page overflows horizontally",
  );
  await page.getByRole("button", { name: "Open navigation" }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: /^Switch project:/ })
    .click();
  await page.getByRole("menuitem", { name: "Workspaces", exact: true }).click();
  await page
    .getByRole("heading", { name: "Your project's first worker" })
    .waitFor();
  await page.goto(new URL("/app/tool-profiles", base).toString());
  await page
    .getByRole("heading", { name: "Page not found", exact: true })
    .waitFor();
  assert.deepEqual(errors, []);
  assert(
    requests.some(
      (r) => r.path === "/ui-api/workspaces" && r.project === "p_a",
    ),
  );
  assert(
    requests.some(
      (r) => r.path === "/ui-api/workspaces" && r.project === "p_b",
    ),
  );
  assert.equal(
    requests.some(
      (r) =>
        r.path === "/ui-api/profiles" ||
        r.path.startsWith("/ui-api/tool-profiles"),
    ),
    false,
    "removed tool profile APIs must never be requested",
  );
  console.log(
    "Browser checks passed: project switching has no stale cross-project rows; workspace creation, native desktop pairing, file handoff, retired home navigation, removed scheduling navigation, global profile assignment/editor, search, dark theme persistence, global navigation and mobile layout. Uncaught errors: 0.",
  );
});
