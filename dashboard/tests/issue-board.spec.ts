import { test, expect } from "@playwright/test";

test("task status, archive, flags and explicit repository sync", async ({
  page,
}) => {
  const project = { id: "board_project", name: "Factory" };
  const work = {
    id: "work_one",
    project_id: project.id,
    workspace_id: "ws_one",
    state: "failed",
    kind: "software",
    specification_path: "work-requests/followup.md",
    updated_at: "2026-11-12T12:00:00Z",
  };
  let card = {
    id: work.id,
    project_id: project.id,
    revision: 1,
    stage: "planned",
    archived_from: "",
    title: "Repair checkout validation",
    attention: true,
    dirty: false,
    work,
    source: {
      connection_id: "forge_one",
      repository: "team/store",
      number: 42,
      url: "https://example.com/issues/42",
      title: "Repair checkout validation",
      state: "open",
      synced_at: "2026-11-12T12:00:00Z",
      error: "",
    },
  };
  const writes: { path: string; body: Record<string, unknown> }[] = [];
  await page.route("**/ui-api/**", async (route) => {
    const req = route.request(),
      path = new URL(req.url()).pathname;
    if (path.endsWith("/capabilities"))
      return route.fulfill({ json: { can_import: true, can_start: true } });
    if (path === "/ui-api/projects")
      return route.fulfill({ json: { items: [project] } });
    if (path === "/ui-api/board")
      return route.fulfill({ json: { items: [card] } });
    if (path.startsWith("/ui-api/board/") && req.method() === "POST") {
      const body = req.postDataJSON();
      writes.push({ path, body });
      if (path.endsWith("/sync"))
        card = {
          ...card,
          revision: card.revision + 1,
          dirty: false,
          source: {
            ...card.source,
            state: card.stage === "done" ? "closed" : "open",
            title: card.title,
          },
        };
      else
        card = {
          ...card,
          revision: card.revision + 1,
          archived_from:
            body.stage === "archived" ? card.stage : card.archived_from,
          stage: body.stage,
          title: body.title || card.title,
          dirty: true,
        };
      return route.fulfill({ json: card });
    }
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto(`/app/projects/${project.id}/board`);
  await expect(
    page.getByRole("heading", { name: "Overview", exact: true }),
  ).toBeVisible();
  const openTask = async () =>
    page.getByRole("button", { name: card.title, exact: true }).click();
  await openTask();
  const detail = page.getByRole("dialog");
  await detail.getByLabel("Task status").selectOption("done");
  await expect.poll(() => writes[0]?.body.stage).toBe("done");
  await detail
    .getByRole("button", { name: "Push changes", exact: true })
    .click();
  await expect
    .poll(() =>
      writes.some(
        (w) => w.path.endsWith("/sync") && w.body.direction === "push",
      ),
    )
    .toBe(true);
  await detail.getByRole("button", { name: "Archive", exact: true }).click();
  await expect.poll(() => card.stage).toBe("archived");
  await expect(
    detail.getByRole("button", { name: /^(Archive|Restore task)$/ }),
  ).toBeEnabled();
  await detail.getByRole("button", { name: "Close", exact: true }).click();
  await expect(detail).toHaveCount(0);
  await page.getByRole("button", { name: "Archive", exact: true }).click();
  await openTask();
  await detail
    .getByRole("button", { name: "Restore task", exact: true })
    .click();
  await expect.poll(() => card.stage).toBe("done");
  await expect(
    detail.getByRole("button", { name: /^(Archive|Restore task)$/ }),
  ).toBeEnabled();
  await detail.getByRole("button", { name: "Close", exact: true }).click();
  await expect(detail).toHaveCount(0);
  await page
    .getByRole("button", { name: "Back to overview", exact: true })
    .click();
  await page.getByRole("button", { name: /^Active/ }).click();
  await openTask();
  await detail.getByLabel("Task status").selectOption("running");
  await expect.poll(() => card.stage).toBe("running");
  await expect(
    detail.getByRole("button", { name: /^(Archive|Restore task)$/ }),
  ).toBeEnabled();
  await detail.getByRole("button", { name: "Close", exact: true }).click();
  await expect(detail).toHaveCount(0);
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
});

test("import only selected repository issues", async ({ page }) => {
  const imported: number[] = [];
  await page.route("**/ui-api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/capabilities"))
      return route.fulfill({ json: { can_import: true, can_start: true } });
    if (path === "/ui-api/projects")
      return route.fulfill({
        json: { items: [{ id: "board_project", name: "Factory" }] },
      });
    if (path === "/ui-api/forge-connections")
      return route.fulfill({
        json: {
          items: [{ id: "forge_one", repository: "team/repo", host: "office" }],
        },
      });
    if (path.endsWith("/issues"))
      return route.fulfill({
        json: {
          items: [
            { number: 1, title: "First issue", state: "open" },
            { number: 2, title: "Second issue", state: "open" },
          ],
        },
      });
    if (path.endsWith("/import")) {
      imported.push(Number(path.split("/").at(-2)));
      return route.fulfill({
        json: { id: "work_import", project_id: "board_project" },
      });
    }
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto("/app/projects/board_project/board");
  await page.getByRole("button", { name: "Import", exact: true }).click();
  await page.getByLabel("Repository to import from").selectOption("forge_one");
  await page.getByRole("checkbox", { name: /First issue/ }).check();
  await page
    .getByRole("button", { name: "Import selected (1)", exact: true })
    .click();
  await expect.poll(() => imported).toEqual([1]);
  await expect(page.getByRole("status")).toContainText("1 issues added");
});

test("create a standalone issue without a repository", async ({ page }) => {
  const project = { id: "local_project", name: "Local" };
  let cards: Record<string, unknown>[] = [];
  let payload: Record<string, unknown> | undefined;
  await page.route("**/ui-api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/capabilities"))
      return route.fulfill({ json: { can_import: true, can_start: true } });
    if (path === "/ui-api/projects")
      return route.fulfill({ json: { items: [project] } });
    if (path === "/ui-api/board") {
      if (route.request().method() === "POST") {
        payload = route.request().postDataJSON();
        const card = {
          id: "work_local",
          project_id: project.id,
          revision: 1,
          stage: "planned",
          title: payload!.title,
          description: payload!.description,
          work: {
            id: "work_local",
            project_id: project.id,
            workspace_id: "ws_local",
            kind: payload!.kind,
            state: "draft",
            specification_path: "request.md",
            updated_at: "2026-11-13T12:00:00Z",
          },
        };
        cards = [card];
        return route.fulfill({ status: 201, json: card });
      }
      return route.fulfill({ json: { items: cards } });
    }
    if (
      path === "/ui-api/board/work_local" &&
      route.request().method() === "POST"
    ) {
      const body = route.request().postDataJSON();
      cards[0] = {
        ...cards[0],
        ...body,
        revision: Number(cards[0].revision) + 1,
      };
      return route.fulfill({ json: cards[0] });
    }
    if (path.endsWith("/instruction"))
      return route.fulfill({
        json: {
          project_id: project.id,
          work_item_id: "work_local",
          instruction: "# Improve search\n\nSupport partial matches",
        },
      });
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto(`/app/projects/${project.id}/board`);
  await page
    .getByRole("button", { name: "Create task", exact: true })
    .first()
    .click();
  await page.getByLabel("Title", { exact: true }).fill("Improve search");
  await page
    .getByLabel("Description", { exact: true })
    .fill("Support partial matches");
  const profile = page.getByLabel("Work profile");
  await expect(profile).toHaveValue("software");
  await profile.selectOption({ label: "Desktop App Development" });
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Create task", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Improve search", exact: true }),
  ).toBeVisible();
  expect(payload).toMatchObject({
    project_id: project.id,
    title: "Improve search",
    description: "Support partial matches",
    kind: "desktop",
  });
  expect(typeof payload!.idempotency_key).toBe("string");
  expect(payload).not.toHaveProperty("connection_id");
  await expect(
    page.getByText("Support partial matches", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Edit", exact: true }).click();
  const markdown =
    "## Acceptance criteria\n- [ ] Works on mobile\n\n<script>window.ticketInjected = true</script>";
  await page
    .getByRole("textbox", { name: "Task description", exact: true })
    .fill(markdown);
  await page.getByRole("button", { name: "Preview", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Acceptance criteria", exact: true }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => (window as unknown as Record<string, unknown>).ticketInjected,
    ),
  ).toBeUndefined();
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Edit", exact: true }),
  ).toBeVisible();
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "Acceptance criteria", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Edit", exact: true }).click();
  await page
    .getByRole("textbox", { name: "Task description", exact: true })
    .fill("");
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect(
    page.getByText("No description yet. Select Edit to add one."),
  ).toBeVisible();
});

test("mobile task filters support keyboard and never change tasks", async ({
  page,
}) => {
  const writes: string[] = [];
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.route("**/ui-api/**", async (route) => {
    const request = route.request();
    if (request.method() !== "GET") writes.push(request.url());
    const path = new URL(request.url()).pathname;
    await route.fulfill({
      json: {
        items:
          path === "/ui-api/projects"
            ? [{ id: "mobile", name: "Mobile board" }]
            : [],
      },
    });
  });
  await page.setViewportSize({ width: 320, height: 844 });
  await page.goto("/app/projects/mobile/board");
  const navigation = page.getByRole("navigation", { name: "Work scope" });
  await expect(navigation).toBeVisible();
  for (const width of [320, 390]) {
    await page.setViewportSize({ width, height: 844 });
    for (const name of ["Active", "Completed", "Planned", "Open"]) {
      const button = navigation.getByRole("button", {
        name: `${name} 0`,
        exact: true,
      });
      await button.focus();
      await page.keyboard.press("Enter");
      await expect(button).toHaveAttribute("aria-pressed", "true");
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
    }
  }
  await page.getByRole("button", { name: "Archive", exact: true }).click();
  await expect(navigation).toHaveCount(0);
  expect(writes).toEqual([]);
});

test("board load failure offers retry without claiming there are no issues", async ({
  page,
}) => {
  let unavailable = true;
  await page.route("**/ui-api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/capabilities"))
      return route.fulfill({ json: { can_import: true, can_start: true } });
    if (path === "/ui-api/projects")
      return route.fulfill({
        json: { items: [{ id: "recover", name: "Recovery" }] },
      });
    if (path === "/ui-api/board" && unavailable)
      return route.fulfill({ status: 503, json: { error: "offline" } });
    await route.fulfill({ json: { items: [] } });
  });
  await page.goto("/app/projects/recover/board");
  await expect(page.getByRole("button", { name: "Try again" })).toBeVisible();
  await expect(
    page.getByText("Create a task to plan your next piece of work."),
  ).toHaveCount(0);
  unavailable = false;
  await page.getByRole("button", { name: "Try again" }).click();
  await expect(
    page.getByText("Create a task to plan your next piece of work."),
  ).toBeVisible();
  await expect(page.getByRole("alert")).toHaveCount(0);
});
