import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
function overviewCards() {
  return [
    {
      id: "work_search",
      number: 2,
      title: "Add task search and filters",
      priority: "normal",
      dependencies: ["work_welcome"],
    },
    {
      id: "work_keyboard",
      number: 3,
      title: "Check keyboard navigation",
      priority: "low",
      dependencies: ["work_search"],
    },
    {
      id: "work_welcome",
      number: 1,
      title: "Build a welcome page",
      priority: "high",
      dependencies: [],
    },
  ].map((c) => ({
    ...c,
    project_id: "A",
    revision: 1,
    stage: "planned",
    owner: "",
    owner_id: "",
    description:
      "Create a useful, accessible task dashboard.\n\n### Acceptance criteria\n- Clear empty and loading states\n- Works on a phone and laptop",
    work: {
      id: c.id,
      project_id: "A",
      workspace_id: "",
      specification_path: "",
      specification_digest: "",
      kind: "software",
      state: "planned",
      created_at: "2026-11-17T10:00:00Z",
      updated_at: "2026-11-17T10:00:00Z",
    },
  }));
}

async function overview(
  page: Page,
  canStart = true,
  userId = "alice",
  cards = overviewCards(),
  otherMembers: string[] = [],
) {
  const requests: string[] = [];
  const edits: Record<string, unknown>[] = [];
  let failEdit = true;
  let starts = 0;
  await page.route("**/auth/session", (r) =>
    r.fulfill({
      json: {
        status: "authenticated",
        user: { id: userId, email: `${userId}@example.com`, name: userId },
      },
    }),
  );
  await page.route("**/dashboard-api/**", (r) => r.fulfill({ json: {} }));
  await page.route("**/ui-api/**", (r) => {
    const path = new URL(r.request().url()).pathname;
    requests.push(path);
    if (path === "/ui-api/projects")
      return r.fulfill({
        json: { items: [{ id: "A", name: "Sample project" }] },
      });
    if (path.endsWith("/capabilities"))
      return r.fulfill({
        json: {
          can_import: false,
          can_start: canStart,
          start_reason:
            "Starting agents requires a project-isolated worker. Ask a platform operator to configure project isolation.",
        },
      });
    if (path.endsWith("/members"))
      return r.fulfill({
        json: {
          can_manage: true,
          user_id: userId,
          items: [
            {
              user_id: "alice",
              email: "alice@example.com",
              role: "admin",
              status: "active",
            },
            {
              user_id: "bob",
              email: "bob@example.com",
              role: "member",
              status: "active",
            },
            ...otherMembers.map((id) => ({
              user_id: id,
              email: `${id}@example.com`,
              role: "member",
              status: "active",
            })),
            {
              user_id: "pending",
              email: "pending@example.com",
              role: "member",
              status: "invited",
            },
          ],
        },
      });
    if (path === "/ui-api/board") return r.fulfill({ json: { items: cards } });
    if (path === "/ui-api/work-items")
      return r.fulfill({ json: { items: cards.map((c) => c.work) } });
    if (path.startsWith("/ui-api/board/") && r.request().method() === "POST") {
      const id = path.split("/")[3],
        card = cards.find((c) => c.id === id)!;
      if (path.endsWith("/start")) {
        starts++;
        return r.fulfill({
          status: 202,
          json: { id: "attempt_one", work_item_id: id },
        });
      }
      const body = r.request().postDataJSON();
      edits.push(body);
      if (body.title && failEdit) {
        failEdit = false;
        return r.fulfill({
          status: 503,
          json: { code: "STORAGE_UNAVAILABLE" },
        });
      }
      expect(body.revision).toBe(card.revision);
      Object.assign(card, body, { revision: card.revision + 1 });
      if (body.owner_id !== undefined)
        card.owner = body.owner_id ? `${body.owner_id}@example.com` : "";
      return r.fulfill({ json: card });
    }
    if (path.startsWith("/ui-api/work-items/")) {
      const card = cards.find((c) => c.id === path.split("/")[3]);
      if (card && !path.endsWith("/attempts") && !path.endsWith("/instruction"))
        return r.fulfill({ json: card.work });
    }
    return r.fulfill({ json: { items: [] } });
  });
  return { cards, requests, edits, starts: () => starts };
}

test("Overview edits, assigns, links dependencies and launches the same task", async ({
  page,
}) => {
  const fixture = await overview(page);
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/app/projects/A/team");
  const rows = page.locator(".team-table tbody tr");
  await expect(rows.first()).toContainText("Build a welcome page");
  await expect(rows.first().locator(".priority-high")).toHaveText("↑High");
  await expect(page.locator(".team-table")).not.toContainText("work_");
  await expect(
    rows.first().getByRole("button", { name: "Start work", exact: true }),
  ).toBeEnabled();
  await page.screenshot({
    path: "artifacts/overview-polished.png",
    fullPage: true,
    animations: "disabled",
  });
  await page
    .getByRole("button", { name: "Add task search and filters", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(
    dialog.getByRole("heading", { name: "Add task search and filters" }),
  ).toBeVisible();
  await page.screenshot({
    path: "artifacts/task-pane-polished.png",
    fullPage: true,
    animations: "disabled",
  });
  await dialog
    .getByRole("button", { name: "Assign to me", exact: true })
    .click();
  await expect(
    dialog.getByRole("combobox", { name: "Assignee", exact: true }),
  ).toHaveText("alice@example.com");
  await dialog.getByRole("combobox", { name: "Assignee", exact: true }).click();
  await page.getByLabel("Search assignee", { exact: true }).fill("bob");
  await expect(page.getByRole("option", { name: /pending/ })).toHaveCount(0);
  await page.getByRole("option", { name: /bob@example.com/ }).click();
  await expect(
    dialog.getByRole("combobox", { name: "Assignee", exact: true }),
  ).toHaveText("bob@example.com");
  await dialog.getByRole("button", { name: "Edit", exact: true }).click();
  await expect(dialog.getByLabel("Task title", { exact: true })).toBeVisible();
  await expect(
    dialog.getByLabel("Task description", { exact: true }),
  ).toBeVisible();
  await dialog
    .getByLabel("Task title", { exact: true })
    .fill("Search tasks by owner");
  await dialog
    .getByLabel("Task description", { exact: true })
    .fill("Both fields save together.");
  await dialog
    .getByRole("button", { name: "Save changes", exact: true })
    .click();
  await expect(dialog.getByRole("alert")).toBeVisible();
  await expect(dialog.getByLabel("Task title", { exact: true })).toHaveValue(
    "Search tasks by owner",
  );
  await dialog
    .getByRole("button", { name: "Save changes", exact: true })
    .click();
  await expect(
    dialog.getByRole("heading", { name: "Search tasks by owner" }),
  ).toBeVisible();
  await expect(
    dialog.getByText("Both fields save together.", { exact: true }),
  ).toBeVisible();
  expect(fixture.edits.at(-1)).toMatchObject({
    title: "Search tasks by owner",
    description: "Both fields save together.",
  });
  await dialog.getByLabel("Priority", { exact: true }).selectOption("high");
  await expect(dialog.locator(".priority-high")).toBeVisible();
  await dialog
    .getByRole("link", { name: "#1 Build a welcome page", exact: true })
    .click();
  await expect(
    dialog.getByRole("heading", { name: "Build a welcome page" }),
  ).toBeVisible();
  await expect(page).toHaveURL(/work=work_welcome/);
  await dialog
    .getByRole("combobox", { name: "Add dependency", exact: true })
    .click();
  await page.getByLabel("Search add dependency", { exact: true }).fill("#3");
  await page
    .getByRole("option", { name: "#3 Check keyboard navigation", exact: true })
    .click();
  await expect(
    dialog.getByRole("link", {
      name: "#3 Check keyboard navigation",
      exact: true,
    }),
  ).toBeVisible();
  await dialog
    .getByRole("button", { name: "Remove dependency #3", exact: true })
    .click();
  await expect(
    dialog.getByRole("link", {
      name: "#3 Check keyboard navigation",
      exact: true,
    }),
  ).toHaveCount(0);
  const issues = await new AxeBuilder({ page })
    .include('[role="dialog"]')
    .analyze();
  expect(issues.violations).toEqual([]);
  await dialog.getByRole("button", { name: "Start work", exact: true }).click();
  await expect(page).toHaveURL(/\/work\?work=work_welcome/);
  expect(fixture.starts()).toBe(1);
});

test("Import explains access without forbidden requests, and the task pane reflows on mobile", async ({
  page,
}) => {
  const fixture = await overview(page, false);
  await page.goto("/app/projects/A/team");
  await page.getByRole("button", { name: "Import", exact: true }).click();
  await expect(
    page.getByRole("heading", {
      name: "Ask a platform operator to import issues",
    }),
  ).toBeVisible();
  expect(fixture.requests.some((p) => p.includes("forge-connections"))).toBe(
    false,
  );
  await expect(page.getByRole("alert")).toHaveCount(0);
  await page.getByRole("button", { name: "Back to tasks" }).click();
  await page
    .getByRole("button", { name: "Build a welcome page", exact: true })
    .click();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({
    path: "artifacts/overview-mobile.png",
    fullPage: true,
    animations: "disabled",
  });
  const dialog = page.getByRole("dialog");
  await expect(
    dialog.getByRole("button", { name: "Start work", exact: true }),
  ).toBeDisabled();
  await expect(
    dialog.getByText(/Starting agents requires a project-isolated worker/),
  ).toBeVisible();
  expect(await dialog.evaluate((e) => e.scrollWidth <= e.clientWidth)).toBe(
    true,
  );
  await dialog.getByRole("button", { name: "Edit", exact: true }).click();
  await expect(dialog.getByLabel("Task title", { exact: true })).toBeVisible();
  expect(await dialog.evaluate((e) => e.scrollWidth <= e.clientWidth)).toBe(
    true,
  );
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: "artifacts/task-pane-mobile.png",
    fullPage: true,
    animations: "disabled",
  });
});

test("Overview and task details remain readable in light mode", async ({
  page,
}) => {
  await overview(page);
  await page.goto("/app/projects/A/team");
  await page
    .getByRole("button", { name: "Switch to light theme", exact: true })
    .click();
  await expect(page.locator(".team-table tbody tr")).toHaveCount(3);
  const overviewAudit = await new AxeBuilder({ page })
    .include(".overview-page")
    .analyze();
  expect(overviewAudit.violations).toEqual([]);
  await page
    .getByRole("button", { name: "Build a welcome page", exact: true })
    .click();
  const paneAudit = await new AxeBuilder({ page })
    .include('[role="dialog"]')
    .analyze();
  expect(paneAudit.violations).toEqual([]);
  await page.screenshot({
    path: "artifacts/task-pane-light.png",
    fullPage: true,
    animations: "disabled",
  });
});

test("all open tasks stay discoverable and actions fit at desktop and phone sizes", async ({
  page,
}) => {
  const fixture = await overview(page);
  Object.assign(fixture.cards[2], {
    owner: "alice@example.com",
    owner_id: "alice",
  });
  Object.assign(fixture.cards[0], { owner: "alice@example.com", owner_id: "" });
  fixture.cards[0].work.state = "in_progress";
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/app/projects/A/team");
  const rows = page.locator(".team-table tbody tr");
  await expect(rows).toHaveCount(3);
  await expect(rows.first()).toContainText("Build a welcome page");
  await expect(rows.first().locator(".task-assignee")).toContainText("You");
  await expect(page.locator(".team-table")).toContainText("Ready to start");
  await expect(
    page.getByLabel("Filter by owner").locator("option"),
  ).toHaveCount(3);
  for (const width of [1440, 1280, 1024, 390]) {
    await page.setViewportSize({ width, height: 1000 });
    for (const row of await rows.all()) {
      const button = row.getByRole("button", {
        name: "Start work",
        exact: true,
      });
      await expect(button).toHaveCSS("height", width >= 1280 ? "32px" : "36px");
      const start = await button.boundingBox();
      expect(start).not.toBeNull();
      expect(start!.x + start!.width).toBeLessThanOrEqual(width);
    }
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    expect(
      await page
        .locator(".team-table-scroll")
        .evaluate((e) => e.scrollWidth <= e.clientWidth),
    ).toBe(true);
  }
  const audit = await new AxeBuilder({ page })
    .include(".overview-page")
    .analyze();
  expect(audit.violations).toEqual([]);
  await page.getByLabel("Filter by owner").selectOption("alice@example.com");
  await expect(rows).toHaveCount(2);
});

test("a failed access check keeps tasks usable and can recover without reloading", async ({
  page,
}) => {
  await overview(page);
  let fail = true;
  await page.route("**/ui-api/projects/A/capabilities", (r) =>
    r.fulfill(
      fail
        ? { status: 404, json: { code: "NOT_FOUND" } }
        : { json: { can_start: true, can_import: false } },
    ),
  );
  await page.goto("/app/projects/A/team");
  await expect(
    page.getByText("Task actions are temporarily unavailable"),
  ).toBeVisible();
  await expect(page.locator(".team-table tbody tr")).toHaveCount(3);
  await expect(page.getByRole("alert")).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Start work", exact: true }).first(),
  ).toBeDisabled();
  fail = false;
  await page.getByRole("button", { name: "Retry access check" }).click();
  await expect(
    page.getByText("Task actions are temporarily unavailable"),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Start work", exact: true }).first(),
  ).toBeEnabled();
});

test("keyboard users return to the task after closing its details", async ({
  page,
}) => {
  await overview(page);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/app/projects/A/team");
  const opener = page.getByRole("button", {
    name: "Build a welcome page",
    exact: true,
  });
  await opener.focus();
  await page.keyboard.press("Enter");
  const pane = page.getByRole("dialog");
  await expect(pane).toBeVisible();
  await expect.poll(async () => (await pane.boundingBox())?.x).toBeLessThan(1);
  await expect.poll(async () => (await pane.boundingBox())?.width).toBe(390);
  await page.keyboard.press("Escape");
  await expect(pane).toHaveCount(0);
  await expect(opener).toBeFocused();
});

test("task tiles open details while execution actions remain independent", async ({
  page,
}) => {
  const fixture = await overview(page);
  await page.goto("/app/projects/A/team");
  const row = page.locator(".team-table tbody tr").first();
  await expect(page.locator(".team-table th")).toHaveText([
    "Task ID",
    "Task",
    "Priority",
    "Assignee",
    "Activity",
    "Actions",
  ]);
  await expect(row.locator("td").first()).toHaveText("#1");
  await expect(
    row.getByRole("button", { name: "Details", exact: true }),
  ).toHaveCount(0);
  await row.locator("td[data-label='Assignee']").click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await expect(
    page
      .getByRole("dialog")
      .getByRole("heading", { name: "Build a welcome page", exact: true }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await row.getByRole("button", { name: "Start work", exact: true }).click();
  await expect(page).toHaveURL(/work\?work=work_welcome/);
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(fixture.starts()).toBe(1);
});

test("activity pills distinguish execution states in both themes", async ({
  page,
}) => {
  const fixture = await overview(page);
  fixture.cards[0].work.state = "failed";
  fixture.cards[1].work.state = "awaiting_review";
  fixture.cards[2].work.state = "running";
  await page.goto("/app/projects/A/team");
  await expect(page.locator(".activity-failed")).toHaveText("Failed");
  await expect(page.locator(".activity-attention")).toHaveText(
    "Awaiting review",
  );
  await expect(page.locator(".activity-active")).toHaveText("Running");
  for (let i = 0; i < 2; i++) {
    const audit = await new AxeBuilder({ page })
      .include(".overview-page")
      .analyze();
    expect(audit.violations).toEqual([]);
    if (i === 0) {
      await page
        .getByRole("button", { name: "Switch to light theme", exact: true })
        .click();
      await expect(page.locator("html")).not.toHaveClass(/\bdark\b/);
      await page.locator(".team-scopes").evaluate(async (scopes) => {
        await Promise.allSettled(
          scopes
            .getAnimations({ subtree: true })
            .map((animation) => animation.finished),
        );
      });
    }
  }
});

test("four project members see their own handoffs while the board refreshes", async ({
  browser,
  baseURL,
}) => {
  const cards = overviewCards();
  const [search, keyboard, welcome] = cards;
  Object.assign(welcome, { owner: "alice@example.com", owner_id: "alice" });
  Object.assign(welcome.work, { controller_id: "architect" });
  Object.assign(search, { owner: "alice@example.com", owner_id: "alice" });
  Object.assign(search.work, {
    controller_id: "alice",
    reviewer_id: "bob",
    state: "awaiting_review",
    latest_attempt_id: "attempt_review",
  });
  Object.assign(keyboard, {
    title: "Accept the integrated result",
    require_dependencies: true,
    owner: "tester@example.com",
    owner_id: "tester",
  });
  Object.assign(keyboard.work, { controller_id: "tester" });

  const actors = ["architect", "alice", "bob", "tester"];
  const pages = await Promise.all(
    actors.map(() => browser.newPage({ baseURL })),
  );
  try {
    await Promise.all(
      pages.map(async (page, index) => {
        await overview(page, true, actors[index], cards, [
          "architect",
          "tester",
        ]);
        await page.goto("/app/projects/A/team");
        await page.getByRole("button", { name: /^For me/ }).click();
      }),
    );
    const [architect, author, reviewer, tester] = pages;
    await expect(architect.locator(".team-table tbody tr")).toHaveCount(1);
    await expect(architect.locator(".team-table")).toContainText(welcome.title);
    await expect(author.locator(".team-table tbody tr")).toHaveCount(2);
    await expect(author.locator(".team-table")).toContainText(
      "Controller: architect@example.com",
    );
    await expect(
      author.getByRole("link", { name: "View review", exact: true }),
    ).toBeVisible();
    await expect(reviewer.locator(".team-table tbody tr")).toHaveCount(1);
    await expect(reviewer.locator(".team-table")).toContainText(
      "Reviewer: You",
    );
    await expect(
      reviewer.getByRole("link", { name: "Review", exact: true }),
    ).toBeVisible();
    await expect(reviewer.locator(".team-table")).toContainText(
      "1 unfinished dependency",
    );
    await expect(tester.locator(".team-table tbody tr")).toHaveCount(1);
    await expect(tester.locator(".team-table")).toContainText(keyboard.title);
    await expect(tester.locator(".team-table")).toContainText(
      "1 required, unfinished dependency",
    );
    await expect(
      tester.getByRole("button", { name: "Start work" }),
    ).toBeDisabled();

    Object.assign(welcome, { stage: "done" });
    Object.assign(welcome.work, { state: "completed" });
    await reviewer.bringToFront();
    await expect(reviewer.locator(".team-table")).not.toContainText(
      "unfinished dependency",
      { timeout: 10_000 },
    );
    Object.assign(search.work, { state: "changes_requested" });
    await author.bringToFront();
    await expect(
      author.getByRole("link", { name: "Revise", exact: true }),
    ).toBeVisible({ timeout: 10_000 });
    await reviewer.bringToFront();
    await expect(reviewer.locator(".team-table tbody tr")).toHaveCount(0, {
      timeout: 10_000,
    });

    Object.assign(search, { stage: "done" });
    Object.assign(search.work, { state: "completed" });
    await tester.bringToFront();
    await expect(tester.locator(".team-table")).not.toContainText(
      "unfinished dependency",
      { timeout: 10_000 },
    );
    await expect(
      tester.getByRole("button", { name: "Start work" }),
    ).toBeEnabled();
  } finally {
    await Promise.all(pages.map((page) => page.close()));
  }
});

test("retrying a stale task edit preserves another member's saved description", async ({
  page,
}) => {
  const fixture = await overview(page);
  const card = fixture.cards[0];
  await page.goto("/app/projects/A/team?work=" + card.id);
  const details = page.getByRole("dialog");
  await details.getByRole("button", { name: "Edit", exact: true }).click();
  await details
    .getByRole("textbox", { name: "Task title" })
    .fill("Updated title");

  await page.route(
    "**/ui-api/board/" + card.id,
    async (route) => {
      if (route.request().method() === "POST") {
        expect(route.request().postDataJSON()).toMatchObject({
          revision: 1,
          title: "Updated title",
        });
        expect(route.request().postDataJSON()).not.toHaveProperty(
          "description",
        );
        card.description = "Description saved by another member.";
        card.revision++;
        return route.fulfill({
          status: 409,
          json: { code: "BOARD_REVISION_CONFLICT" },
        });
      }
      return route.fallback();
    },
    { times: 1 },
  );

  const save = details.getByRole("button", { name: "Save changes" });
  await save.click();
  await expect(save).toBeDisabled();
  await expect(
    details.getByRole("textbox", { name: "Task title" }),
  ).toHaveValue("Updated title");
  await details
    .getByRole("button", { name: "Load latest and discard draft" })
    .click();
  await expect(
    details.getByRole("textbox", { name: "Task description" }),
  ).toHaveValue("Description saved by another member.");
  await details
    .getByRole("textbox", { name: "Task title" })
    .fill("Updated title");
  await save.click();
  // The shared fixture fails its first title save so its recovery path is also exercised.
  await expect(details.getByRole("alert")).toBeVisible();
  await save.click();
  await expect(
    details.getByRole("heading", { name: "Updated title" }),
  ).toBeVisible();
  expect(card.description).toBe("Description saved by another member.");
});
