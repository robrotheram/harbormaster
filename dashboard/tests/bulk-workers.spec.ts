import { expect, test, type Page } from "@playwright/test";

async function setup(page: Page) {
  const projects = [
    { id: "A", name: "Atlas", created_at: "2026-11-11T12:00:00Z" },
    { id: "B", name: "Beacon", created_at: "2026-11-11T12:00:00Z" },
  ];
  let workers = ["one", "two", "three", "four"].map((name, index) => ({
    id: `ws_${name}`,
    name: `Worker ${name}`,
    project_id: index < 2 ? "A" : "B",
    state: "STOPPED",
    ready: false,
    active_runs: 0,
    background_jobs: 0,
    cpu_limit: 1,
    memory_limit_bytes: 1073741824,
    runtime_type: "kata",
    isolation_status: "unverified",
    network_status: "unverified",
    created_at: projects[0].created_at,
    updated_at: projects[0].created_at,
  }));
  const posts: { id: string; action: string; body: Record<string, string> }[] =
    [];
  let failStop = true;
  let active = 0,
    maxActive = 0;
  await page.route("**/dashboard-api/**", (r) =>
    r.fulfill({ json: { generated_at: projects[0].created_at, usage: {} } }),
  );
  await page.route("**/ui-api/**", async (route) => {
    const request = route.request(),
      url = new URL(request.url());
    if (url.pathname === "/ui-api/projects")
      return route.fulfill({ json: { items: projects } });
    if (url.pathname === "/ui-api/workspaces")
      return route.fulfill({
        json: {
          items: workers.filter(
            (w) => w.project_id === url.searchParams.get("project_id"),
          ),
        },
      });
    const match = url.pathname.match(
      /^\/ui-api\/workspaces\/(ws_\w+)\/(start|stop|delete)$/,
    );
    if (match && request.method() === "POST") {
      const [, id, action] = match;
      posts.push({ id, action, body: request.postDataJSON() });
      active++;
      maxActive = Math.max(maxActive, active);
      await new Promise((resolve) => setTimeout(resolve, 100));
      active--;
      if (action === "stop" && id === "ws_two" && failStop) {
        failStop = false;
        return route.fulfill({ status: 409, json: { code: "WORKSPACE_BUSY" } });
      }
      const worker = workers.find((w) => w.id === id)!;
      if (action === "delete") workers = workers.filter((w) => w.id !== id);
      else {
        worker.state = action === "start" ? "READY" : "STOPPED";
        worker.ready = action === "start";
      }
      return route.fulfill({
        json: action === "delete" ? { deleted: true } : worker,
      });
    }
    return route.fulfill({
      status: 404,
      json: { code: "NOT_FOUND_OR_FORBIDDEN" },
    });
  });
  await page.goto("/app/infrastructure");
  await expect(
    page.getByRole("button", { name: "Worker four", exact: true }),
  ).toBeVisible();
  return { posts, maxActive: () => maxActive };
}

test("bulk lifecycle actions report partial failures and retry only failed workers", async ({
  page,
}) => {
  const { posts, maxActive } = await setup(page);
  await page
    .getByRole("button", { name: "Bulk operations", exact: true })
    .click();
  await page.getByLabel("Select all visible workers", { exact: true }).check();
  await page
    .getByRole("button", { name: "Start selected workers", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toContainText("Atlas");
  await expect(page.getByRole("dialog")).toContainText("Beacon");
  expect(posts).toHaveLength(0);
  await page
    .getByRole("button", { name: "Start 4 workers", exact: true })
    .click();
  await expect(page.getByRole("status")).toHaveText("4 succeeded");
  expect(posts.filter((p) => p.action === "start")).toHaveLength(4);
  expect(posts.every((p) => p.body.idempotency_key)).toBe(true);
  await page
    .getByRole("button", { name: "Close", exact: true })
    .first()
    .click();
  await expect(
    page.getByLabel("Select all visible workers", { exact: true }),
  ).not.toBeChecked();
  await page.getByLabel("Select all visible workers", { exact: true }).check();
  await page
    .getByRole("button", { name: "Stop selected workers", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Stop 4 workers", exact: true })
    .click();
  await expect(page.getByRole("status")).toHaveText("3 succeeded, 1 failed");
  await expect(page.getByRole("alert")).toContainText("active work");
  expect(posts.filter((p) => p.action === "stop")).toHaveLength(4);
  await page.screenshot({ path: "artifacts/bulk-worker-results.png" });
  await page
    .getByRole("button", { name: "Retry 1 failed", exact: true })
    .click();
  await expect(page.getByRole("status")).toHaveText("4 succeeded");
  const stops = posts.filter((p) => p.action === "stop");
  expect(stops).toHaveLength(5);
  const retried = stops.filter((p) => p.id === "ws_two");
  expect(retried).toHaveLength(2);
  expect(retried[0].body.idempotency_key).toBe(retried[1].body.idempotency_key);
  expect(maxActive()).toBeLessThanOrEqual(3);
  await page
    .getByRole("button", { name: "Close", exact: true })
    .first()
    .click();
  await expect(
    page.getByRole("button", { name: "Stop selected workers", exact: true }),
  ).toBeDisabled();
});

test("bulk deletion confirms exact targets, cancellation makes no requests", async ({
  page,
}) => {
  const { posts } = await setup(page);
  await page
    .getByRole("button", { name: "Bulk operations", exact: true })
    .click();
  await page.getByLabel("Select worker Worker one", { exact: true }).check();
  await page.getByLabel("Select worker Worker three", { exact: true }).check();
  await page
    .getByRole("button", { name: "Delete selected workers", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toContainText(
    "Private disks and saved evidence remain on the host",
  );
  await expect(page.getByRole("dialog")).not.toContainText("Worker two");
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  expect(posts).toHaveLength(0);
  await page
    .getByRole("button", { name: "Delete selected workers", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Delete 2 workers", exact: true })
    .click();
  await expect(page.getByRole("status")).toHaveText("2 succeeded");
  expect(posts).toEqual([
    { id: "ws_one", action: "delete", body: { confirm_id: "ws_one" } },
    { id: "ws_three", action: "delete", body: { confirm_id: "ws_three" } },
  ]);
  await page
    .getByRole("button", { name: "Close", exact: true })
    .first()
    .click();
  await expect(
    page.getByRole("button", { name: "Worker one", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Worker three", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Worker two", exact: true }),
  ).toBeVisible();
});

test("filter changes clear selection and select-all affects only visible workers", async ({
  page,
}) => {
  const { posts } = await setup(page);
  await page
    .getByRole("button", { name: "Bulk operations", exact: true })
    .click();
  await page.getByLabel("Select all visible workers", { exact: true }).check();
  await page.getByLabel("Filter workers by project").selectOption("A");
  await expect(
    page.getByRole("button", { name: "Start selected workers", exact: true }),
  ).toBeDisabled();
  await page.getByLabel("Select worker Worker one", { exact: true }).check();
  await expect(
    page.getByLabel("Select all visible workers", { exact: true }),
  ).toHaveJSProperty("indeterminate", true);
  await page.getByLabel("Select all visible workers", { exact: true }).check();
  await page
    .getByRole("button", { name: "Start selected workers", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toContainText("Start 2 workers");
  await expect(page.getByRole("dialog")).not.toContainText("Beacon");
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await page.getByLabel("Search workers", { exact: true }).fill("missing");
  await expect(
    page.getByLabel("Select all visible workers", { exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole("button", { name: "Delete selected workers", exact: true }),
  ).toBeDisabled();
  await page
    .getByRole("button", { name: "Clear filters", exact: true })
    .click();
  await expect(
    page.getByLabel("Select worker Worker one", { exact: true }),
  ).not.toBeChecked();
  expect(posts).toHaveLength(0);
});

test("bulk mode is optional, keeps selection separate, and clears it on exit", async ({
  page,
}) => {
  const { posts } = await setup(page);
  for (const width of [1440, 390, 320]) {
    await page.setViewportSize({ width, height: 900 });
    await expect(page.getByRole("checkbox")).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: "Start selected workers" }),
    ).toHaveCount(0);
    await page
      .getByRole("button", { name: "Bulk operations", exact: true })
      .click();
    await expect(page.getByRole("checkbox")).toHaveCount(5);
    const checkbox = page.getByLabel("Select worker Worker one", {
      exact: true,
    });
    await checkbox.focus();
    await page.keyboard.press("Space");
    await expect(checkbox).toBeChecked();
    await expect(
      page.getByRole("button", { name: "Start selected workers" }),
    ).toBeEnabled();
    const selection = await checkbox.boundingBox();
    const name = await page
      .getByRole("button", { name: "Worker one", exact: true })
      .boundingBox();
    expect(selection!.x + selection!.width).toBeLessThan(name!.x);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: `artifacts/bulk-mode-${width}.png`,
      fullPage: true,
    });
    await page
      .getByRole("button", { name: "Done with bulk operations", exact: true })
      .click();
    await expect(page.getByRole("checkbox")).toHaveCount(0);
    await page
      .getByRole("button", { name: "Bulk operations", exact: true })
      .click();
    await expect(checkbox).not.toBeChecked();
    await expect(
      page.getByRole("button", { name: "Start selected workers" }),
    ).toBeDisabled();
    await page
      .getByRole("button", { name: "Done with bulk operations", exact: true })
      .click();
  }
  expect(posts).toHaveLength(0);
});
