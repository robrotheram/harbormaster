import { test, expect } from "@playwright/test";

test("empty projects expose only Projects navigation and retry sample creation safely", async ({
  page,
}) => {
  const project = { id: "sample", name: "Sample project" };
  let created = false;
  const titles = [
    "Build a welcome page",
    "Add task search and filters",
    "Check keyboard navigation",
  ];
  const cards = titles.map((title, i) => ({
    id: `sample_task_${i}`,
    project_id: "sample",
    title,
    description: "Sample task acceptance criteria",
    stage: "planned",
    revision: 1,
    priority: i === 0 ? "high" : "normal",
    dependencies: i ? [`sample_task_${i - 1}`] : [],
    work: {
      id: `sample_task_${i}`,
      project_id: "sample",
      state: "planned",
      workspace_id: "",
      specification_path: "",
      specification_digest: "",
      kind: "software",
      updated_at: "2026-11-17T10:00:00Z",
    },
  }));
  const writes: { path: string; body: Record<string, unknown> }[] = [];
  await page.route("**/dashboard-api/**", (r) => r.fulfill({ json: {} }));
  await page.route("**/ui-api/**", (r) => {
    const path = new URL(r.request().url()).pathname;
    if (r.request().method() === "POST") {
      writes.push({ path, body: r.request().postDataJSON() });
      if (path === "/ui-api/projects") {
        if (writes.length === 1)
          return r.fulfill({
            status: 503,
            json: { code: "STORAGE_UNAVAILABLE" },
          });
        created = true;
        return r.fulfill({ status: 201, json: project });
      }
    }
    if (path === "/ui-api/projects")
      return r.fulfill({ json: { items: created ? [project] : [] } });
    if (path === "/ui-api/board") return r.fulfill({ json: { items: cards } });
    if (path === "/ui-api/work-items")
      return r.fulfill({ json: { items: cards.map((c) => c.work) } });
    return r.fulfill({ json: { items: [] } });
  });
  await page.goto("/app/");
  const sidebar = page.locator("#factory-sidebar");
  await expect(
    page.getByRole("heading", { name: "Getting started" }),
  ).toBeVisible();
  await expect(
    sidebar.getByRole("link", { name: "Projects", exact: true }),
  ).toBeVisible();
  for (const name of [
    "Workspace",
    "Overview",
    "Reviews",
    "Workers",
    "Settings",
  ])
    await expect(sidebar.getByRole("link", { name, exact: true })).toHaveCount(
      0,
    );
  await expect(
    sidebar.getByRole("button", { name: "Platform", exact: true }),
  ).toHaveCount(0);
  const add = page.getByRole("button", {
    name: "Add sample project",
    exact: true,
  });
  await add.click();
  await expect(page.getByRole("alert")).toBeVisible();
  await add.click();
  await expect(page).toHaveURL(/projects\/sample\/team/);
  for (const title of titles)
    await expect(
      page.getByRole("button", { name: title, exact: true }),
    ).toBeVisible();
  await expect(page.getByText("Not started", { exact: true })).toHaveCount(3);
  expect(writes).toHaveLength(2);
  expect(writes[0]).toEqual(writes[1]);
  expect(writes[1].body.name).toBe("Sample project");
  expect(writes[1].body.sample).toBe(true);
  expect(writes[1].body.idempotency_key).toBeTruthy();
  await page.getByRole("button", { name: titles[0], exact: true }).click();
  await expect(
    page.getByText("Sample task acceptance criteria", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Close", exact: true }).click();
  await sidebar.getByRole("link", { name: "Workspace", exact: true }).click();
  await expect(page).toHaveURL(/projects\/sample\/work/);
  for (const title of titles)
    await expect(
      page
        .locator(".conversation-work-items")
        .getByText(title, { exact: true }),
    ).toHaveCount(0);

  await sidebar.getByRole("link", { name: "Projects", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Sample project", exact: true }),
  ).toBeVisible();
  await expect(add).toHaveCount(0);
});

test("a project administrator deletes and returns to clean getting started navigation", async ({
  page,
}) => {
  let deleted = false;
  const afterDeletion: string[] = [];
  await page.route("**/auth/session", (r) =>
    r.fulfill({
      json: {
        status: "authenticated",
        user: { id: "alice", email: "alice@example.com", name: "Alice" },
      },
    }),
  );
  await page.route("**/dashboard-api/**", (r) => r.fulfill({ json: {} }));
  await page.route("**/ui-api/**", (r) => {
    const path = new URL(r.request().url()).pathname;
    if (deleted && path.startsWith("/ui-api/projects/A/"))
      afterDeletion.push(path);
    if (path === "/ui-api/projects")
      return r.fulfill({
        json: { items: deleted ? [] : [{ id: "A", name: "Disposable" }] },
      });
    if (path.endsWith("/members"))
      return r.fulfill({
        json: { items: [], can_manage: true, user_id: "alice" },
      });
    if (path.endsWith("/delete")) {
      expect(r.request().postDataJSON()).toEqual({ confirm_id: "A" });
      deleted = true;
      return r.fulfill({ json: { deleted: true } });
    }
    return r.fulfill({ json: { items: [] } });
  });
  await page.goto("/app/projects/A/settings");
  await page
    .getByRole("button", { name: "Delete project", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Delete project?",
    exact: true,
  });
  await dialog
    .getByLabel("Type the project name to confirm")
    .fill("Disposable");
  await dialog
    .getByRole("button", { name: "Delete project", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Add sample project", exact: true }),
  ).toBeVisible();
  await expect(
    page
      .locator("#factory-sidebar")
      .getByRole("navigation", { name: "Project navigation" }),
  ).toHaveCount(0);
  await expect(page.getByRole("alert")).toHaveCount(0);
  expect(afterDeletion).toEqual([]);
  expect(
    await page.evaluate(() => localStorage.getItem("workbench.last-project")),
  ).toBeNull();
});
