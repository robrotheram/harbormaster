import { test, expect } from "@playwright/test";

test("board and review use real work and bind decisions to the inspected attempt", async ({
  page,
}) => {
  const project = {
    id: "project_delivery",
    name: "Delivery",
    created_at: "2026-11-01T00:00:00Z",
  };
  const item = {
    id: "work_delivery",
    project_id: project.id,
    workspace_id: "ws_one",
    kind: "software",
    specification_path: "spec.md",
    state: "awaiting_review",
    latest_attempt_id: "attempt_one",
    created_at: project.created_at,
    updated_at: project.created_at,
  };
  let decision: unknown;
  await page.route("**/ui-api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/review")) {
      decision = route.request().postDataJSON();
      item.state = "completed";
      return route.fulfill({ json: item });
    }
    if (path === "/ui-api/projects")
      return route.fulfill({ json: { items: [project] } });
    if (path === "/ui-api/board")
      return route.fulfill({
        json: {
          items: [
            {
              id: item.id,
              project_id: project.id,
              stage: "review",
              revision: 0,
              dirty: false,
              attention: false,
              work: item,
            },
          ],
        },
      });
    if (path === "/ui-api/work-items")
      return route.fulfill({ json: { items: [item] } });
    if (path === "/ui-api/work-items/" + item.id)
      return route.fulfill({ json: item });
    if (path.endsWith("/instruction"))
      return route.fulfill({
        json: {
          project_id: project.id,
          work_item_id: item.id,
          instruction: "Fix checkout validation",
        },
      });
    if (path.endsWith("/attempts"))
      return route.fulfill({
        json: {
          items: [
            {
              id: "attempt_one",
              work_item_id: item.id,
              state: "awaiting_review",
              artifacts: [],
              created_at: project.created_at,
              updated_at: project.created_at,
            },
          ],
        },
      });
    return route.fulfill({ json: { items: [] } });
  });
  await page.setViewportSize({ width: 938, height: 1152 });
  await page.goto(`/app/projects/${project.id}/board`);
  await expect(
    page.getByRole("heading", { name: "Overview", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Fix checkout validation", exact: true }),
  ).toBeVisible();
  const row = page.locator(".task-clickable-row");
  for (const width of [938, 768]) {
    await page.setViewportSize({ width, height: 1152 });
    await expect(row).toHaveCSS("display", "table-row");
    await expect
      .poll(async () => (await row.boundingBox())?.height)
      .toBeLessThan(100);
    await expect
      .poll(() =>
        page
          .locator(".team-table-scroll")
          .evaluate((table) => table.scrollWidth <= table.clientWidth),
      )
      .toBe(true);
  }
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(row).toHaveCSS("display", "grid");
  await expect
    .poll(async () => (await row.boundingBox())?.height)
    .toBeLessThan(175);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page
    .getByRole("button", { name: "Fix checkout validation", exact: true })
    .click();
  await expect(page.getByText("Review output", { exact: true })).toHaveCount(0);
  await page.getByRole("link", { name: "Open thread", exact: true }).click();
  await page.getByRole("button", { name: "Workspace", exact: true }).click();
  await expect(
    page.getByRole("region", { name: "Task workspace" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Task settings" }).click();
  const settings = page.getByRole("dialog", { name: "Task settings" });
  const accept = settings.getByRole("button", {
    name: "Approve this revision",
  });
  await expect(accept).toBeEnabled();
  await accept.click();
  await expect
    .poll(() => decision)
    .toEqual({
      project_id: project.id,
      accepted: true,
      expected_attempt_id: "attempt_one",
    });
  await page.keyboard.press("Escape");
  await expect(settings).toHaveCount(0);
  await expect(
    page
      .locator(".conversation-thread-heading")
      .getByText("Accepted", { exact: true }),
  ).toBeVisible();
});

test("a planned task can use the running worker's subscription model", async ({
  page,
}) => {
  const project = { id: "project_delivery", name: "Delivery" };
  const item = {
    id: "work_planned",
    project_id: project.id,
    workspace_id: "",
    kind: "software",
    state: "planned",
  };
  let payload: unknown;
  await page.route("**/ui-api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/capabilities"))
      return route.fulfill({
        json: { can_start: true, requires_agent_selection: true },
      });
    if (path.endsWith("/models"))
      return route.fulfill({
        json: {
          workspace_id: "ws_ready",
          items: [{ provider: "codex", model: "loaded-model" }],
        },
      });
    if (path === "/ui-api/projects")
      return route.fulfill({ json: { items: [project] } });
    if (path === "/ui-api/workspaces")
      return route.fulfill({
        json: {
          items: [
            {
              id: "ws_ready",
              project_id: project.id,
              name: "Running worker",
              ready: true,
            },
          ],
        },
      });
    if (path === "/ui-api/board")
      return route.fulfill({
        json: {
          items: [
            {
              id: item.id,
              project_id: project.id,
              title: "Planned task",
              stage: "planned",
              revision: 1,
              work: item,
            },
          ],
        },
      });
    if (path.endsWith("/start")) {
      payload = route.request().postDataJSON();
      return route.fulfill({ status: 202, json: {} });
    }
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto(`/app/projects/${project.id}/board`);
  await page.getByRole("button", { name: "Start work", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Start work", exact: true });
  await expect(dialog.getByLabel("Worker")).toHaveValue("ws_ready");
  const start = dialog.getByRole("button", { name: "Start work", exact: true });
  await expect(start).toBeDisabled();
  await dialog
    .getByLabel("Model", { exact: true })
    .selectOption('["codex","loaded-model"]');
  await start.click();
  await expect
    .poll(() => payload)
    .toMatchObject({
      project_id: project.id,
      revision: 1,
      workspace_id: "ws_ready",
      selection: { provider: "codex", model: "loaded-model" },
    });
});
