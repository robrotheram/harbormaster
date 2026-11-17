import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

async function setup(
  page: Page,
  kind = "software",
  fallback = false,
  savedPreview = false,
) {
  const project = {
    id: "prj_review",
    name: "Review test",
    created_at: "2026-11-12T12:00:00Z",
  };
  const work = {
    id: "work_review",
    project_id: project.id,
    workspace_id: "ws_review",
    specification_path: "request.md",
    kind,
    state: "awaiting_review",
    latest_attempt_id: "attempt_new",
    created_at: project.created_at,
    updated_at: project.created_at,
  };
  const attempts = [
    {
      id: "attempt_old",
      work_item_id: work.id,
      workspace_id: work.workspace_id,
      task_id: "task_old",
      review_state: "accepted",
      state: "awaiting_review",
      created_at: "2026-11-11T12:00:00Z",
      updated_at: project.created_at,
      artifacts: [],
    },
    {
      id: "attempt_new",
      work_item_id: work.id,
      workspace_id: work.workspace_id,
      task_id: "task_new",
      state: "awaiting_review",
      created_at: project.created_at,
      updated_at: project.created_at,
      artifacts: [
        {
          id: "code",
          source_path: "fix.patch",
          content_url: "/v1/artifacts/fix",
        },
        {
          id: "data",
          source_path: "measurements.csv",
          content_url: "/v1/artifacts/data",
        },
        {
          id: "report",
          source_path: "report.md",
          content_url: "/v1/artifacts/report",
        },
      ],
    },
  ];
  let boardRevision = 1;
  const writes: { path: string; body: Record<string, unknown> }[] = [];
  await page.route("**/ui-api/**", async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (route.request().method() === "POST") {
      writes.push({ path, body: route.request().postDataJSON() });
      if (
        path.endsWith("/review") &&
        route.request().postDataJSON().decision === "closed"
      ) {
        work.state = "closed";
        attempts[1].review_state = "closed";
        return route.fulfill({ json: work });
      }
      if (path === "/ui-api/board/" + work.id) {
        const body = route.request().postDataJSON();
        work.state = (
          {
            planned: "planned",
            running: "in_progress",
            review: "awaiting_review",
            done: "closed",
          } as Record<string, string>
        )[body.stage];
        attempts[1].review_state = work.state === "closed" ? "closed" : "";
        boardRevision++;
        return route.fulfill({
          json: {
            id: work.id,
            project_id: project.id,
            revision: boardRevision,
            stage: body.stage,
            work,
            title: "Make checkout clearer",
            dirty: false,
            attention: false,
          },
        });
      }
      if (path.endsWith("/review"))
        return route.fulfill({
          status: 409,
          json: { code: "WORK_STATE_CONFLICT" },
        });
      return route.fulfill({ json: attempts[1] });
    }
    if (path.endsWith("/applications"))
      return route.fulfill({
        json: {
          items: savedPreview
            ? [
                {
                  id: "app_old",
                  state: "expired",
                  ready: false,
                  application: {
                    kind: "web",
                    directory: "web",
                    command: "npm run dev -- --host 0.0.0.0 --port $PORT",
                    port: 3000,
                  },
                },
              ]
            : [],
        },
      });
    if (path === "/ui-api/projects")
      return route.fulfill({ json: { items: [project] } });
    if (path === "/ui-api/work-items")
      return route.fulfill({ json: { items: [work] } });
    if (path === "/ui-api/work-items/" + work.id)
      return route.fulfill({ json: work });
    if (path === "/ui-api/board")
      return route.fulfill({
        json: {
          items: [
            {
              id: work.id,
              project_id: project.id,
              revision: boardRevision,
              stage:
                work.state === "closed"
                  ? "done"
                  : work.state === "in_progress"
                    ? "running"
                    : work.state === "planned"
                      ? "planned"
                      : "review",
              title: "Make checkout clearer",
              work,
              attention: false,
              dirty: false,
            },
          ],
        },
      });
    if (path.endsWith("/attempts"))
      return route.fulfill({ json: { items: attempts } });
    if (path.endsWith("/instruction"))
      return route.fulfill({
        json: {
          project_id: project.id,
          work_item_id: work.id,
          instruction: "Make checkout clearer",
        },
      });
    if (path.endsWith("/history")) {
      const old = url.searchParams.get("episode") === "task_old";
      const content = old
        ? "# Previous delivery\nThe original checkout."
        : '# Checkout delivery\nValidation now explains missing fields.\n\n```workbench-chart\n{"title":"Validation results","data":[{"name":"Pass","value":8},{"name":"Fail","value":2}]}\n```';
      return route.fulfill({
        json: {
          workspace_id: work.workspace_id,
          project_id: project.id,
          page: {
            schema_version: 1,
            next_before: 0,
            capture_limited: false,
            records: fallback
              ? []
              : [
                  {
                    id: "message",
                    source: "codex",
                    at: project.created_at,
                    seq: 2,
                    episode_id: url.searchParams.get("episode"),
                    kind: "assistant.message",
                    content,
                  },
                  {
                    id: "edit",
                    source: "codex",
                    at: project.created_at,
                    seq: 1,
                    episode_id: url.searchParams.get("episode"),
                    kind: "tool.call",
                    tool: "apply_patch",
                    content: JSON.stringify({
                      patch:
                        "*** Update File: checkout.ts\n-oldLabel\n+clearLabel",
                    }),
                  },
                ],
          },
        },
      });
    }
    if (path.endsWith("/result"))
      return route.fulfill({
        json: {
          content: "# Retained task response\nThe task completed successfully.",
          next_offset: -1,
        },
      });
    if (path === "/ui-api/artifacts/fix")
      return route.fulfill({
        body: "--- checkout.ts\n+++ checkout.ts\n@@ -1 +1 @@\n-oldLabel\n+clearLabel",
      });
    if (path === "/ui-api/artifacts/data")
      return route.fulfill({ body: "name,value\nPass,8\nFail,2" });
    if (path === "/ui-api/artifacts/report")
      return route.fulfill({
        body: "# Verification report\nAll checks passed.\n\n<script>window.reviewInjected = true</script>",
      });
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto(`/app/projects/${project.id}/reviews`);
  await expect(
    page.getByRole("heading", { name: "Make checkout clearer" }),
  ).toBeVisible();
  return { writes, project, work, attempts };
}

test("review shows only the current task result and guards stale decisions", async ({
  page,
}) => {
  const { writes } = await setup(page);
  await expect(page.getByLabel("Review attempt")).toHaveCount(0);
  await expect(page.getByText(/Run \d/)).toHaveCount(0);
  await page.getByRole("tab", { name: "Code review" }).click();
  await expect(page.locator(".review-code .is-added")).toContainText([
    "+++ checkout.ts",
    "+clearLabel",
  ]);
  await page.getByRole("button", { name: /Edit 1/ }).click();
  await expect(
    page.getByText("Captured activity for this task", { exact: false }),
  ).toBeVisible();
  await page.getByRole("tab", { name: "Reports", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Checkout delivery" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "report.md Retained file" }).click();
  await expect(
    page.getByRole("heading", { name: "Verification report" }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => (window as unknown as Record<string, unknown>).reviewInjected,
    ),
  ).toBeUndefined();
  await page.getByRole("tab", { name: "Visualisations" }).click();
  await page
    .getByRole("button", { name: "measurements.csv Retained file" })
    .click();
  await expect(page.locator(".interactive-chart")).toBeVisible();
  await page.getByRole("tab", { name: "Reports", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Checkout delivery" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Previous delivery" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Approve this revision" }),
  ).toBeEnabled();
  await page.getByRole("button", { name: "Approve this revision" }).click();
  await expect
    .poll(() => writes[0]?.body)
    .toEqual({
      project_id: "prj_review",
      accepted: true,
      expected_attempt_id: "attempt_new",
    });
  await expect(page.getByRole("alert")).toContainText(
    "This work changed since you opened it",
  );
});

test("research reviews open reports and fall back to the task response", async ({
  page,
}) => {
  await setup(page, "research", true);
  await expect(
    page.getByRole("tab", { name: "Reports", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await expect(
    page.getByRole("heading", { name: "Retained task response" }),
  ).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
});

test("preview setup starts only the server and keeps the review unchanged", async ({
  page,
}) => {
  const { writes } = await setup(page);
  await page.getByRole("button", { name: "Set up preview" }).click();
  await page
    .getByLabel("Start command")
    .fill("npm run dev -- --host 0.0.0.0 --port $PORT");
  await page
    .getByRole("button", { name: "Start preview", exact: true })
    .click();
  await expect.poll(() => writes.length).toBe(1);
  expect(writes[0].path).toBe("/ui-api/workspaces/ws_review/applications");
  await expect(
    page.getByRole("heading", { name: "Make checkout clearer" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Approve this revision", exact: true }),
  ).toBeEnabled();
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "Make checkout clearer" }),
  ).toBeVisible();
});

test("expired previews reuse their launch command without creating a work attempt", async ({
  page,
}) => {
  const { writes } = await setup(page, "software", false, true);
  await page
    .getByRole("button", { name: "Start preview", exact: true })
    .click();
  await expect.poll(() => writes.length).toBe(1);
  expect(writes[0].path).toBe("/ui-api/workspaces/ws_review/applications");
  expect(writes[0].body.application).toEqual({
    kind: "web",
    directory: "web",
    command: "npm run dev -- --host 0.0.0.0 --port $PORT",
    port: 3000,
  });
  await expect(page.getByLabel("Review attempt")).toHaveCount(0);
  await expect(
    page.getByRole("heading", { name: "Make checkout clearer" }),
  ).toBeVisible();
});

test("request changes requires a comment and sends it with the reviewed attempt", async ({
  page,
}) => {
  const { writes } = await setup(page);
  await expect(page.getByRole("checkbox")).toHaveCount(0);
  await expect(
    page.getByRole("textbox", { name: "Changes needed" }),
  ).toHaveCount(0);
  await page
    .getByRole("button", { name: "Request changes", exact: true })
    .click();
  const changes = page.getByRole("button", {
    name: "Send changes",
    exact: true,
  });
  await expect(changes).toBeDisabled();
  await page
    .getByRole("textbox", { name: "Changes needed" })
    .fill("Make the button easier to read.");
  await changes.click();
  await expect
    .poll(() => writes[0]?.body.instruction)
    .toBe("Make the button easier to read.");
  expect(writes[0].path).toBe("/ui-api/work-items/work_review/continue");
  expect(writes[0].body.expected_attempt_id).toBe("attempt_new");
});

test("Close finishes work without approving or starting another attempt", async ({
  page,
}) => {
  const { writes } = await setup(page);
  await page.getByRole("button", { name: "Close", exact: true }).click();
  await expect
    .poll(() => writes[0]?.body)
    .toEqual({
      project_id: "prj_review",
      decision: "closed",
      expected_attempt_id: "attempt_new",
    });
  await expect(page.getByText("Nothing waiting for review.")).toBeVisible();
  await page.getByRole("button", { name: "History", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Make checkout clearer" }),
  ).toBeVisible();
  await expect(page.locator(".review-outcome")).toContainText("Closed");
  await expect(
    page.getByRole("button", { name: "Approve this revision", exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("link", { name: "Open conversation", exact: true })
    .click();
  await expect(
    page.locator(".conversation-work-item .thread-state"),
  ).toHaveText("Closed");
  await expect(page.locator(".conversation-thread-heading")).toContainText(
    "Closed",
  );
  await expect(page.locator(".conversation-thread-heading")).not.toContainText(
    "Accepted",
  );
  await page.getByRole("link", { name: "Overview", exact: true }).click();
  await page.getByRole("button", { name: /^Completed/ }).click();
  await page
    .getByRole("button", { name: "Make checkout clearer", exact: true })
    .click();
  await expect(page.getByLabel("Task status")).toBeDisabled();
  await expect(
    page
      .getByRole("dialog")
      .getByRole("button", { name: "Archive", exact: true }),
  ).toBeEnabled();
  expect(writes).toHaveLength(1);
});

test("preview gets a fresh single-use link after reopening or a readiness change", async ({
  page,
}) => {
  await setup(page);
  let ready = true;
  let connections = 0;
  await page.route("**/ui-api/workspaces/ws_review/applications", (route) =>
    route.fulfill({
      json: {
        items: [
          {
            id: "app_preview",
            state: "active",
            ready,
            application: {
              kind: "web",
              directory: "checkout",
              port: 3000,
              command: "npm run dev",
            },
          },
        ],
      },
    }),
  );
  await page.route(
    "**/ui-api/workspaces/ws_review/services/app_preview/connect",
    (route) => {
      connections++;
      return route.fulfill({
        json: {
          preview_url: `http://preview-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.localhost:4173/connect#ticket=${connections}`,
        },
      });
    },
  );
  await page.route(
    "http://preview-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.localhost:4173/**",
    (route) =>
      route.fulfill({
        contentType: "text/html",
        body: "<p>Checkout preview</p>",
      }),
  );
  const frame = page.getByTitle("Application preview", { exact: true });
  await expect(frame).toHaveAttribute("src", /ticket=\d+$/);
  const first = await frame.getAttribute("src");
  const opened = connections;
  // Polling alone must not reload a working preview.
  await page.waitForResponse((response) =>
    response.url().endsWith("/applications"),
  );
  expect(connections).toBe(opened);
  ready = false;
  await expect(frame).toHaveCount(0);
  ready = true;
  await expect(frame).toBeVisible({ timeout: 10000 });
  await expect(frame).not.toHaveAttribute("src", first!);
  const second = await frame.getAttribute("src");
  await page.getByRole("tab", { name: "Reports", exact: true }).click();
  await page.getByRole("tab", { name: "UI preview" }).click();
  await expect(frame).toBeVisible();
  await expect(frame).not.toHaveAttribute("src", second!);
  const third = await frame.getAttribute("src");
  await page.getByRole("button", { name: "Reload preview" }).click();
  await expect(frame).toBeVisible();
  await expect(frame).not.toHaveAttribute("src", third!);
  await expect(
    page.getByRole("button", { name: "Approve this revision", exact: true }),
  ).toBeEnabled();
});

test("finished work status stays bound to its run and cannot be changed from Overview", async ({
  page,
}) => {
  const { writes } = await setup(page);
  await page.getByRole("button", { name: "Close", exact: true }).click();
  await expect(page.getByText("Nothing waiting for review.")).toBeVisible();
  await page.getByRole("link", { name: "Overview", exact: true }).click();
  await page.getByRole("button", { name: /^Completed/ }).click();
  await page
    .getByRole("button", { name: "Make checkout clearer", exact: true })
    .click();
  await expect(page.getByLabel("Task status")).toBeDisabled();
  await expect(
    page.getByText("Status follows the run and review."),
  ).toBeVisible();
  await page.getByRole("link", { name: "Open thread", exact: true }).click();
  await expect(page.locator(".conversation-thread-heading")).toContainText(
    "Closed",
  );
  expect(writes.map((w) => w.path)).toEqual([
    "/ui-api/work-items/work_review/review",
  ]);
});

test("review comments retain drafts and delivery keys across task switches and explain byte limits", async ({
  page,
}) => {
  const f = await setup(page, "research");
  const second = {
    ...f.work,
    id: "work_other",
    latest_attempt_id: "attempt_other",
  };
  await page.route("**/ui-api/work-items?project_id=prj_review", (route) =>
    route.fulfill({ json: { items: [f.work, second] } }),
  );
  await page.route(
    "**/ui-api/work-items/work_other/attempts?project_id=prj_review",
    (route) =>
      route.fulfill({
        json: {
          items: [
            {
              ...f.attempts[1],
              id: "attempt_other",
              work_item_id: "work_other",
            },
          ],
        },
      }),
  );
  await page.route(
    "**/ui-api/work-items/work_other/instruction?project_id=prj_review",
    (route) =>
      route.fulfill({
        json: {
          project_id: f.project.id,
          work_item_id: second.id,
          instruction: "Review account settings",
        },
      }),
  );
  const sent: Record<string, unknown>[] = [];
  await page.route("**/ui-api/work-items/work_review/continue", (route) => {
    sent.push(route.request().postDataJSON());
    return sent.length < 3
      ? route.fulfill({ status: 503, json: { code: "WORKSPACE_UNAVAILABLE" } })
      : route.fulfill({ json: f.attempts[1] });
  });
  await page.reload();
  await page
    .getByRole("button", { name: "Request changes", exact: true })
    .click();
  const input = page.getByRole("textbox", {
    name: "Changes needed",
    exact: true,
  });
  const send = page.getByRole("button", { name: "Send changes", exact: true });
  await input.fill("😀".repeat(4001));
  await expect(input).toHaveAttribute("aria-invalid", "true");
  await expect(page.getByRole("alert")).toContainText("exceeds 16 kB");
  await expect(send).toBeDisabled();
  expect(sent).toEqual([]);
  for (const width of [1440, 320]) {
    await page.setViewportSize({ width, height: 800 });
    for (const theme of ["dark", "light"]) {
      const toggle = page.getByRole("button", {
        name: `Switch to ${theme} theme`,
        exact: true,
      });
      if (await toggle.count()) await toggle.click();
      await page.waitForFunction(
        () =>
          !document
            .getAnimations()
            .some(
              (animation) =>
                animation instanceof CSSTransition &&
                animation.playState === "running",
            ),
      );
      const audit = await new AxeBuilder({ page })
        .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
        .analyze();
      expect(audit.violations, `${theme} review comment at ${width}px`).toEqual(
        [],
      );
      if (width === 320)
        expect((await send.boundingBox())!.height).toBeGreaterThanOrEqual(44);
      await page.evaluate(() => {
        document.documentElement.style.fontSize = "200%";
      });
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
      expect(
        await send.evaluate(
          (element) => element.scrollWidth <= element.clientWidth,
        ),
      ).toBe(true);
      await page.evaluate(() => {
        document.documentElement.style.fontSize = "";
      });
    }
  }
  await input.fill("Clarify missing fields.");
  await send.click();
  await expect(page.locator(".review-decision [role=alert]")).toBeVisible();
  await expect(input).toHaveValue("Clarify missing fields.");
  const queue = page.getByRole("complementary", { name: "Review queue" });
  await queue.getByRole("button", { name: /Review account settings/ }).click();
  await page
    .getByRole("button", { name: "Request changes", exact: true })
    .click();
  await input.fill("Check profile saves.");
  await queue.getByRole("button", { name: /Make checkout clearer/ }).click();
  await expect(input).toHaveValue("Clarify missing fields.");
  await send.click();
  await expect.poll(() => sent.length).toBe(2);
  expect(sent[0].idempotency_key).toBeTruthy();
  expect(sent[1].idempotency_key).toBe(sent[0].idempotency_key);
  expect(sent[1].expected_attempt_id).toBe("attempt_new");
  await expect(input).toBeEnabled();
  await input.fill("Clarify missing fields and add an example.");
  await send.click();
  await expect.poll(() => sent.length).toBe(3);
  expect(sent[2].idempotency_key).not.toBe(sent[0].idempotency_key);
  await expect(input).toHaveValue("");
  await queue.getByRole("button", { name: /Review account settings/ }).click();
  await expect(input).toHaveValue("Check profile saves.");
  expect(f.writes).toEqual([]);
});

test("review status outages keep the comment while disabling decisions until a read retry succeeds", async ({
  page,
}) => {
  const f = await setup(page, "research");
  await page
    .getByRole("button", { name: "Request changes", exact: true })
    .click();
  const input = page.getByRole("textbox", { name: "Changes needed" });
  await input.fill("Keep this review draft through the outage.");
  for (const endpoint of [
    "**/ui-api/work-items/work_review/attempts?project_id=prj_review",
    "**/ui-api/work-items?project_id=prj_review",
  ]) {
    let failing = true;
    await page.route(endpoint, (route) =>
      failing
        ? route.fulfill({ status: 503, json: { code: "SERVICE_UNAVAILABLE" } })
        : route.fallback(),
    );
    await expect(page.getByText(/Review status is unavailable/)).toBeVisible({
      timeout: 12000,
    });
    await expect(input).toHaveValue(
      "Keep this review draft through the outage.",
    );
    await expect(input).toBeEnabled();
    await expect(
      page.getByRole("button", { name: "Send changes", exact: true }),
    ).toBeDisabled();
    await page.getByRole("button", { name: "Cancel", exact: true }).click();
    for (const name of ["Approve this revision", "Request changes", "Close"])
      await expect(
        page.getByRole("button", { name, exact: true }),
      ).toBeDisabled();
    failing = false;
    await page
      .getByRole("alert")
      .filter({
        has: page.getByRole("button", { name: "Try again", exact: true }),
      })
      .getByRole("button", { name: "Try again", exact: true })
      .click();
    await expect(
      page.getByRole("button", { name: "Request changes", exact: true }),
    ).toBeEnabled();
    await page
      .getByRole("button", { name: "Request changes", exact: true })
      .click();
    await expect(input).toHaveValue(
      "Keep this review draft through the outage.",
    );
    await page.unroute(endpoint);
  }
  expect(f.writes).toEqual([]);
});

test("failed evidence reads never masquerade as empty reports and recover in place", async ({
  page,
}) => {
  const f = await setup(page, "research");
  f.attempts[1].artifacts = [];
  let historyFailed = true;
  await page.route("**/ui-api/workspaces/ws_review/history?*", (route) =>
    historyFailed
      ? route.fulfill({ status: 503, json: { code: "HISTORY_UNAVAILABLE" } })
      : route.fallback(),
  );
  await page.route(
    "**/ui-api/workspaces/ws_review/tasks/task_new/result?*",
    (route) =>
      route.fulfill({ status: 503, json: { code: "RESULT_UNAVAILABLE" } }),
  );
  await page.reload();
  for (const [tab, heading] of [
    ["Reports", "Report unavailable"],
    ["Code review", "Code evidence unavailable"],
    ["Visualisations", "Visualisations unavailable"],
    ["Reports", "Report unavailable"],
  ]) {
    await page.getByRole("tab", { name: tab, exact: true }).click();
    await expect(
      page.getByRole("heading", { name: heading, exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", {
        name: /^No (report|code evidence|visualisations)/,
      }),
    ).toHaveCount(0);
    await expect(
      page
        .locator(".review-evidence")
        .getByRole("button", { name: "Try again", exact: true })
        .first(),
    ).toBeVisible();
  }
  historyFailed = false;
  await page
    .locator(".review-evidence > [role=alert]")
    .first()
    .getByRole("button", { name: "Try again", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Checkout delivery", exact: true }),
  ).toBeVisible();
  await expect(page.locator(".review-evidence > [role=alert]")).toHaveCount(0);
  expect(f.writes).toEqual([]);
});

test("retained review files retry without a decision or losing the comment", async ({
  page,
}) => {
  const f = await setup(page, "research");
  let failing = true;
  await page.route("**/ui-api/artifacts/report", (route) =>
    failing
      ? route.fulfill({ status: 503, json: { code: "OUTPUT_UNAVAILABLE" } })
      : route.fallback(),
  );
  await page
    .getByRole("button", { name: "Request changes", exact: true })
    .click();
  await page
    .getByRole("textbox", { name: "Changes needed" })
    .fill("Check the report totals.");
  await page.getByRole("button", { name: "report.md Retained file" }).click();
  await expect(page.locator(".review-document [role=alert]")).toBeVisible();
  failing = false;
  await page
    .locator(".review-document")
    .getByRole("button", { name: "Try again", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Verification report" }),
  ).toBeVisible();
  await expect(
    page.getByRole("textbox", { name: "Changes needed" }),
  ).toHaveValue("Check the report totals.");
  expect(f.writes).toEqual([]);
});

test("large review queues load titles in batches and keep the open review stable during search", async ({
  page,
}) => {
  const fixture = await setup(page, "research");
  const catalog = Array.from({ length: 120 }, (_, index) => ({
    ...fixture.work,
    id: index === 0 ? fixture.work.id : `work_queue_${index}`,
    specification_digest: `digest-${index}`,
    specification_path: `work-requests/${index}.md`,
  }));
  const readIds = new Set<string>();
  let reading = 0;
  let peak = 0;
  let failLast = true;
  await page.route("**/ui-api/work-items**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (route.request().method() !== "GET") return route.fallback();
    if (path === "/ui-api/work-items")
      return route.fulfill({ json: { items: catalog } });
    if (path.endsWith("/instruction")) {
      const id = path.split("/").at(-2)!;
      const index = catalog.findIndex((item) => item.id === id);
      if (index < 0) return route.fallback();
      readIds.add(id);
      reading++;
      peak = Math.max(peak, reading);
      await new Promise((resolve) => setTimeout(resolve, 25));
      reading--;
      if (index === 119 && failLast)
        return route.fulfill({
          status: 503,
          json: { code: "SERVICE_UNAVAILABLE" },
        });
      return route.fulfill({
        json: {
          project_id: fixture.project.id,
          work_item_id: id,
          instruction:
            index === 0
              ? "Make checkout clearer"
              : index >= 118
                ? `Quarterly reconciliation ${index}`
                : `Review task ${index}`,
        },
      });
    }
    if (path.endsWith("/attempts")) {
      const id = path.split("/").at(-2)!;
      return route.fulfill({
        json: {
          items: fixture.attempts.map((attempt) => ({
            ...attempt,
            work_item_id: id,
          })),
        },
      });
    }
    return route.fallback();
  });
  await page.reload();
  const queue = page.getByRole("complementary", { name: "Review queue" });
  const heading = page.locator(".review-detail-heading h2");
  await expect(heading).toHaveText("Make checkout clearer");
  await expect(queue.getByRole("status")).toHaveCount(0);
  expect(readIds.size).toBe(20);
  // The open review has its own independent request-text read.
  expect(peak).toBeLessThanOrEqual(5);
  await expect(queue.locator(".review-queue-item")).toHaveCount(20);
  await expect(queue.getByText("Showing 20 of 120 tasks")).toBeVisible();
  await queue.getByRole("button", { name: "Show more tasks" }).click();
  await expect(queue.getByRole("status")).toHaveCount(0);
  expect(readIds.size).toBe(40);
  await expect(queue.locator(".review-queue-item")).toHaveCount(40);
  await page
    .getByRole("button", { name: "Request changes", exact: true })
    .click();
  const comment = page.getByRole("textbox", { name: "Changes needed" });
  await comment.fill("Keep the original review comment while I search.");
  peak = 0;
  await page
    .getByRole("textbox", { name: "Filter reviews" })
    .fill("  Quarterly reconciliation  ");
  await expect(queue.getByText("Searching task titles…")).toBeVisible();
  await expect(
    queue.getByText("No matching tasks.", { exact: true }),
  ).toHaveCount(0);
  await expect(heading).toHaveText("Make checkout clearer");
  await expect(comment).toHaveValue(
    "Keep the original review comment while I search.",
  );
  await expect(queue.getByText("Searching task titles…")).toHaveCount(0);
  expect(readIds.size).toBe(120);
  expect(peak).toBeLessThanOrEqual(4);
  await expect(queue.getByText("Showing 1 of 1 available match")).toBeVisible();
  await expect(
    queue.getByText(/Search results may be incomplete/),
  ).toBeVisible();
  failLast = false;
  await queue.getByRole("button", { name: "Retry titles" }).click();
  await expect(queue.getByText("Showing 2 of 2 matches")).toBeVisible();
  await expect(heading).toHaveText("Make checkout clearer");
  for (const width of [1440, 320]) {
    await page.setViewportSize({ width, height: 900 });
    for (const theme of ["light", "dark"]) {
      const toggle = page.getByRole("button", {
        name: `Switch to ${theme} theme`,
        exact: true,
      });
      if (await toggle.count()) await toggle.click();
      await page.waitForFunction(
        () =>
          !document
            .getAnimations()
            .some(
              (animation) =>
                animation instanceof CSSTransition &&
                animation.playState === "running",
            ),
      );
      expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
    }
  }
  await queue
    .getByRole("button", { name: /Quarterly reconciliation 119/ })
    .click();
  await expect(heading).toHaveText("Quarterly reconciliation 119");
  await page.getByRole("textbox", { name: "Filter reviews" }).fill("");
  await expect(
    queue.getByRole("button", { name: "Show open review" }),
  ).toBeVisible();
  await queue.getByRole("button", { name: "Show open review" }).click();
  await expect(
    queue.getByRole("button", { name: /Quarterly reconciliation 119/ }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(
    queue.getByRole("button", { name: /Quarterly reconciliation 119/ }),
  ).toBeInViewport();
  await expect(
    queue.getByRole("button", { name: /Quarterly reconciliation 119/ }),
  ).toBeFocused();
  await page
    .getByRole("textbox", { name: "Filter reviews" })
    .fill("Make checkout clearer");
  await expect(heading).toHaveText("Quarterly reconciliation 119");
  await queue.getByRole("button", { name: /Make checkout clearer/ }).click();
  await expect(comment).toHaveValue(
    "Keep the original review comment while I search.",
  );
  await page
    .getByRole("textbox", { name: "Filter reviews" })
    .fill("nothing matches this title");
  await expect(
    queue.getByText("No matching tasks.", { exact: true }),
  ).toBeVisible();
  await expect(heading).toHaveText("Make checkout clearer");
  await queue.getByRole("button", { name: "Show open review" }).click();
  await expect(
    page.getByRole("textbox", { name: "Filter reviews" }),
  ).toHaveValue("");
  await expect(
    queue.getByRole("button", { name: /Make checkout clearer/ }),
  ).toHaveAttribute("aria-pressed", "true");
  expect(fixture.writes).toEqual([]);
});

test("screenshot notes are sent with feedback and a failed send keeps the image", async ({
  page,
}) => {
  const { writes } = await setup(page);
  await page.route("**/ui-api/work-items/work_review/continue", (route) => {
    writes.push({
      path: "/ui-api/work-items/work_review/continue",
      body: route.request().postDataJSON(),
    });
    return route.fulfill({
      status: 503,
      json: { code: "WORKSPACE_UNAVAILABLE" },
    });
  });
  await page
    .getByRole("button", { name: "Request changes", exact: true })
    .click();
  await page
    .getByLabel("Changes needed", { exact: true })
    .fill("Fix the highlighted layout.");
  await page.getByLabel("Attach screenshot").setInputFiles({
    name: "capture.png",
    mimeType: "image/png",
    buffer: Buffer.from(
      await page.evaluate(() => {
        const canvas = document.createElement("canvas");
        canvas.width = 20;
        canvas.height = 20;
        return canvas.toDataURL("image/png").split(",")[1];
      }),
      "base64",
    ),
  });
  await expect(page.getByAltText("Review screenshot 1")).toBeVisible();
  await page
    .getByLabel("Screenshot 1 notes")
    .fill("The button overlaps the title.");
  await page.getByRole("button", { name: "Send changes", exact: true }).click();
  await expect.poll(() => writes.length).toBe(1);
  expect(writes[0].body.screenshots).toEqual([
    { content: expect.any(String), note: "The button overlaps the title." },
  ]);
  await expect(page.getByAltText("Review screenshot 1")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Send changes", exact: true }),
  ).toBeEnabled();
  await page.getByRole("button", { name: "Send changes", exact: true }).click();
  await expect.poll(() => writes.length).toBe(2);
  expect(writes[1].body).toEqual(writes[0].body);
});

test("agent review starts separate work without deciding or revising the source", async ({
  page,
}) => {
  await setup(page);
  const requests: { path: string; body: any }[] = [];
  await page.route("**/ui-api/work-items", async (route) => {
    if (route.request().method() !== "POST") return route.fallback();
    requests.push({
      path: "/work-items",
      body: route.request().postDataJSON(),
    });
    return route.fulfill({
      status: 201,
      json: {
        id: "work_agent_review",
        project_id: "prj_review",
        workspace_id: "ws_review",
        kind: "research",
        state: "draft",
      },
    });
  });
  await page.route("**/ui-api/work-items/work_agent_review/run", (route) => {
    requests.push({ path: "/run", body: route.request().postDataJSON() });
    return route.fulfill({ status: 202, json: {} });
  });
  await page.getByText("Agent review", { exact: true }).click();
  await page.getByLabel("Agent review focus").fill("Check accessibility");
  await page.getByRole("button", { name: "Start agent review" }).click();
  await expect(
    page.getByRole("link", { name: "Open agent review conversation" }),
  ).toBeVisible();
  expect(requests.map((r) => r.path)).toEqual(["/work-items", "/run"]);
  expect(requests[0].body.instruction).toContain("attempt_new");
  expect(requests[0].body.instruction).toContain("Check accessibility");
  expect(requests[0].body.kind).toBe("research");
  await expect(
    page.getByRole("button", { name: "Approve this revision", exact: true }),
  ).toBeVisible();
});
