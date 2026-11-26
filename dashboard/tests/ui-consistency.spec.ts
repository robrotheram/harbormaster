import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

async function fixture(page: Page) {
  const timestamp = "2026-11-11T12:00:00Z";
  const project = {
    id: "audit",
    name: "Orchestration development " + "x".repeat(80),
    created_at: timestamp,
  };
  const worker = {
    id: "ws_audit",
    project_id: project.id,
    name: "Development agent " + "x".repeat(80),
    agent_kind: "codex",
    state: "READY",
    ready: true,
    active_runs: 0,
    background_jobs: 0,
    runtime_type: "firecracker",
    isolation_status: "unverified",
    network_status: "unverified",
    cpu_limit: 1,
    memory_limit_bytes: 1073741824,
    created_at: timestamp,
    updated_at: timestamp,
  };
  const schedule = {
    id: "schedule_audit",
    project_id: project.id,
    workspace_id: worker.id,
    name: "Subscription connectivity check",
    instruction_path: "SCHEDULE.md",
    enabled: false,
    interval_seconds: 0,
    run_at: timestamp,
    created_at: timestamp,
  };
  await page.route("**/dashboard-api/**", (route) =>
    route.fulfill({
      json: {
        generated_at: timestamp,
        usage: {
          running_workspaces: 1,
          max_running_workspaces: 4,
          cpu_allocated: 1,
          memory_allocated_bytes: 1073741824,
        },
        services: {
          items: [
            {
              name: "Control plane",
              status: "healthy",
              reason_code: "available",
            },
          ],
        },
      },
    }),
  );
  await page.route("**/ui-api/**", async (route) => {
    expect(route.request().method()).toBe("GET");
    const path = new URL(route.request().url()).pathname;
    const responses: Record<string, unknown> = {
      "/ui-api/projects": { items: [project] },
      "/ui-api/workspaces": { items: [worker] },
      "/ui-api/work-items": { items: [] },
      "/ui-api/schedules": { items: [schedule] },
      "/ui-api/runs": {
        items: [
          {
            id: "run_audit",
            project_id: project.id,
            workspace_id: worker.id,
            schedule_id: schedule.id,
            state: "succeeded",
            task_id: "task_" + "b".repeat(32),
            created_at: timestamp,
            updated_at: timestamp,
          },
        ],
      },
      "/ui-api/connection-profiles": {
        items: [
          {
            id: "approved",
            name: "Development allowlist " + "x".repeat(80),
            description: "Approved destinations for development work.",
            revision: 1,
            policy: {
              mode: "allowlist",
              destinations: ["github.com:443", "registry.npmjs.org:443"],
            },
            updated_at: timestamp,
          },
        ],
      },
      "/ui-api/projects/audit/hermes": {
        config: {
          project_id: project.id,
          provider: "openai-codex",
          model: "approved-model",
          mode: "both",
          connection_profile_id: "approved",
          matrix: {
            homeserver: "https://matrix.example.com",
            bot_user_id: "@hermes:example.com",
            room_ids: ["!project:example.com"],
            sender_ids: ["@operator:example.com"],
          },
          revision: 1,
          updated_at: timestamp,
        },
        saved: true,
        runtime_available: true,
        runtime_status: "available",
        matrix_available: true,
        matrix_state: "disconnected",
      },
    };
    await route.fulfill({ json: responses[path] || { items: [] } });
  });
  const pages = [
    ["projects", "/app/", "Projects"],
    ["workers", "/app/workers", "Workers"],
    ["services", "/app/platform-services", "Platform services"],
    ["connections", "/app/connection-profiles", "Network profiles"],
    ["help", "/app/help", "Getting started"],
    ["repositories", "/app/repositories", "Connections"],
    [
      "assistant",
      "/app/projects/audit/assistant?tab=hermes",
      "Project assistant",
    ],
    ...[
      ["board", "Overview"],
      ["work", "Project work"],
      ["workspaces", "Workers"],
      ["settings", "Project settings"],
    ].map(([key, title]) => [key, `/app/projects/audit/${key}`, title]),
  ];
  return { pages, worker };
}

test("all application pages preserve readable controls and fit narrow screens", async ({
  page,
}) => {
  test.setTimeout(180000);
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  const { pages, worker } = await fixture(page);
  for (const width of [1440, 320]) {
    await page.setViewportSize({ width, height: 900 });
    for (const [key, path, title] of pages) {
      await page.goto(path);
      await expect(
        page.getByRole("heading", { name: title, exact: true, level: 1 }),
      ).toBeVisible();
      await expect(page.locator("main [data-slot=skeleton]")).toHaveCount(0);
      await expect(
        page.locator("main").getByText(/^(Loading|Retrieving).*…$/),
      ).toHaveCount(0);
      await page.evaluate(() => document.fonts.ready);
      const overflow = await page.evaluate(() => ({
        width: innerWidth,
        document: document.documentElement.scrollWidth,
      }));
      expect(overflow.document, `${title} at ${width}px`).toBeLessThanOrEqual(
        overflow.width,
      );
      await expect(page.locator("main h1")).toHaveCount(1);
      for (const theme of ["dark", "light"]) {
        const toggle = page.getByRole("button", {
          name: `Switch to ${theme} theme`,
          exact: true,
        });
        if (await toggle.count()) await toggle.click();
        // Measure the settled palette, not interpolated colors during a theme transition.
        await page.waitForFunction(
          () =>
            !document
              .getAnimations()
              .some(
                (animation) =>
                  animation instanceof CSSTransition &&
                  animation.playState === "running",
              ),
          undefined,
          { timeout: 5000 },
        );
        const accessibility = await new AxeBuilder({ page })
          .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
          .analyze();
        expect
          .soft(
            accessibility.violations.map((violation) => ({
              rule: violation.id,
              help: violation.help,
              elements: violation.nodes.map((node) => ({
                target: node.target,
                summary: node.failureSummary,
              })),
            })),
            `${title} accessibility at ${width}px in ${theme} theme`,
          )
          .toEqual([]);
      }
      // The thread list suite checks its toolbar controls together.
      const searches = page.locator(
        'main input[aria-label^="Search"]:not([aria-label="Search threads"]):visible',
      );
      for (const input of await searches.all())
        expect((await input.boundingBox())!.height).toBe(32);
      if (key === "infrastructure") {
        const controls = page.locator(
          ".infra-filters input, .infra-filters select",
        );
        const boxes = await controls.evaluateAll((items) =>
          items.map((e) => ({
            height: e.getBoundingClientRect().height,
            y: e.getBoundingClientRect().y,
          })),
        );
        expect(boxes.every((box) => box.height === 32)).toBe(true);
        if (width === 1440)
          expect(new Set(boxes.map((box) => box.y)).size).toBe(1);
        const table = await page.locator(".infra-table").boundingBox();
        const name = await page
          .getByRole("button", { name: worker.name, exact: true })
          .boundingBox();
        expect(name!.x - table!.x).toBeGreaterThanOrEqual(20);
      }
      await page.screenshot({
        path: `artifacts/audit-${key}-${width}.png`,
        fullPage: true,
      });
    }
  }
  expect(errors).toEqual([]);
});

test("every main page identifies itself in the browser title", async ({
  page,
}) => {
  const { pages } = await fixture(page);
  for (const [, path, title] of pages) {
    await page.goto(path);
    await expect(
      page.getByRole("heading", { name: title, exact: true, level: 1 }),
    ).toBeVisible();
    await expect(page).toHaveTitle(`${title} · Workbench`);
  }
});

test("keyboard navigation moves to new content and skip links focus main", async ({
  page,
}) => {
  await fixture(page);
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto("/app/");
    await expect(
      page.getByRole("heading", { name: "Projects", exact: true }),
    ).toBeVisible();
    await page.keyboard.press("Tab");
    await expect(
      page.getByRole("link", { name: "Skip to content" }),
    ).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(page.getByRole("main")).toBeFocused();
    await page.getByRole("link", { name: "Docs", exact: true }).focus();
    await page.keyboard.press("Enter");
    await expect(
      page.getByRole("heading", { name: "Getting started", exact: true }),
    ).toBeFocused();
    if (width < 1024) {
      await page.getByRole("button", { name: "Open navigation" }).focus();
      await page.keyboard.press("Enter");
      await expect(
        page.getByRole("dialog", { name: "Factory navigation" }),
      ).toBeVisible();
    }
    await page
      .getByRole("navigation", { name: "Project navigation" })
      .getByRole("link", { name: "Workers", exact: true })
      .focus();
    await page.keyboard.press("Enter");
    await expect(
      page.getByRole("heading", { name: "Workers", exact: true }),
    ).toBeFocused();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await page.goBack();
    await expect(
      page.getByRole("heading", { name: "Getting started", exact: true }),
    ).toBeFocused();
    await page.goBack();
    await expect(
      page.getByRole("heading", { name: "Projects", exact: true }),
    ).toBeFocused();
  }
});

test("all pages reflow with increased text spacing", async ({ page }) => {
  test.setTimeout(90000);
  const { pages } = await fixture(page);
  for (const width of [1440, 320]) {
    await page.setViewportSize({ width, height: 900 });
    for (const [key, path, title] of pages) {
      await page.goto(path);
      await expect(
        page.getByRole("heading", { name: title, exact: true, level: 1 }),
      ).toBeVisible();
      await expect(page.locator("main [data-slot=skeleton]")).toHaveCount(0);
      await expect(
        page.locator("main").getByText(/^(Loading|Retrieving).*…$/),
      ).toHaveCount(0);
      await page.addStyleTag({
        content:
          "* { line-height: 1.5 !important; letter-spacing: .12em !important; word-spacing: .16em !important; } p { margin-bottom: 2em !important; }",
      });
      const dimensions = await page.evaluate(() => ({
        viewport: innerWidth,
        document: document.documentElement.scrollWidth,
      }));
      expect
        .soft(dimensions.document, `${title} text spacing at ${width}px`)
        .toBeLessThanOrEqual(dimensions.viewport);
      await page.screenshot({
        path: `artifacts/spacing-${key}-${width}.png`,
        fullPage: true,
      });
    }
  }
});

test("all main pages reflow with a doubled base font size", async ({
  page,
}) => {
  test.setTimeout(90000);
  const { pages } = await fixture(page);
  for (const width of [1440, 320]) {
    await page.setViewportSize({ width, height: 900 });
    for (const [key, path, title] of pages) {
      await page.goto(path);
      await expect(
        page.getByRole("heading", { name: title, exact: true, level: 1 }),
      ).toBeVisible();
      await expect(page.locator("main [data-slot=skeleton]")).toHaveCount(0);
      await page.evaluate(() => {
        document.documentElement.style.fontSize = "200%";
      });
      await page.evaluate(() => document.fonts.ready);
      if (key === "hermes") {
        for (const tab of ["Agent", "Matrix"]) {
          await page.getByRole("tab", { name: tab, exact: true }).click();
          for (const button of await page
            .locator("main button:visible")
            .all()) {
            const box = await button.evaluate((el) => ({
              text: el.textContent,
              width: el.clientWidth,
              scroll: el.scrollWidth,
            }));
            expect
              .soft(box.scroll, `${tab}: ${box.text} at ${width}px`)
              .toBeLessThanOrEqual(box.width + 1);
          }
        }
      }
      expect
        .soft(
          await page.evaluate(() => document.documentElement.scrollWidth),
          `${title} enlarged base text at ${width}px`,
        )
        .toBeLessThanOrEqual(width);
      await page.screenshot({
        path: `artifacts/enlarged-${key}-${width}.png`,
        fullPage: true,
      });
    }
  }
});
