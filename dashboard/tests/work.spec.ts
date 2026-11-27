import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { readFile } from "node:fs/promises";

async function previewTool(page: Page, name: string) {
  await page
    .getByRole("button", { name: "Preview options", exact: true })
    .click();
  await page.getByRole("menuitem", { name, exact: true }).click();
}
async function conversationTool(page: Page, name: string) {
  await page.getByRole("button", { name: "Workspace", exact: true }).click();
  await page
    .getByRole("group", { name: "Workspace tools" })
    .getByRole("button", { name, exact: true })
    .click();
}

async function conversationFixture(page: Page) {
  const now = "2026-11-12T12:00:00Z";
  const item = {
    id: "work_one",
    project_id: "A",
    workspace_id: "ws_one",
    specification_path: "spec.md",
    specification_digest: "a".repeat(64),
    kind: "research",
    state: "draft",
    created_at: now,
    updated_at: now,
    latest_attempt_id: "",
  };
  const attempts: Record<string, any>[] = [];
  const posts: { path: string; body: Record<string, any> }[] = [];
  const state = {
    exists: false,
    failContinue: false,
    applicationsError: 0,
    foreignHistory: false,
    alternateWorker: false,
    request: "Investigate the experiment.",
    reply: "All four tests passed. The report is ready.",
    connects: 0,
    report:
      "# Results\nAll four tests passed.\n<script>window.untrusted=true</script>",
  };
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  const addRun = (parent = "") => {
    const n = attempts.length + 1;
    const attempt = {
      id: `attempt_${n}`,
      work_item_id: item.id,
      workspace_id: item.workspace_id,
      task_id: `task_${n}`,
      state: "running",
      created_at: `2026-11-12T12:0${n}:00Z`,
      updated_at: now,
      parent_attempt_id: parent,
      artifacts: [],
    };
    attempts.push(attempt);
    item.state = "running";
    item.latest_attempt_id = attempt.id;
    return attempt;
  };
  const finish = () => {
    const a = attempts.at(-1)!;
    a.state = "awaiting_review";
    item.state = "awaiting_review";
    a.artifacts = [
      {
        id: `artifact_${attempts.length}`,
        source_path: "outputs/report.md",
        size_bytes: 80,
        content_url: `/v1/work-items/work_one/artifacts/artifact_${attempts.length}/content?project_id=A`,
      },
    ];
  };
  const apps: Record<string, any>[] = [];
  const ports: { port: number; listener: string }[] = [];
  await page.route("**/ui-api/**", async (route) => {
    const req = route.request(),
      url = new URL(req.url()),
      path = url.pathname;
    if (path.endsWith("/models"))
      return route.fulfill({
        json: {
          workspace_id: "ws_one",
          default_provider: "codex",
          default_model: "worker-model",
          items: [
            { provider: "codex", model: "worker-model" },
            { provider: "claude", model: "chosen-model" },
          ],
        },
      });
    if (path.endsWith("/ports"))
      return route.fulfill({ json: { items: ports } });
    if (path.endsWith("/applications")) {
      if (req.method() === "POST") {
        const body = req.postDataJSON();
        posts.push({ path, body });
        const app = {
          id: "service_live",
          state: "active",
          ready: true,
          logs: "Listening on port 4173",
          application: body.application,
        };
        apps.push(app);
        return route.fulfill({ status: 201, json: app });
      }
      if (state.applicationsError)
        return route.fulfill({
          status: state.applicationsError,
          json: {
            code:
              state.applicationsError === 403
                ? "FORBIDDEN"
                : "WORKSPACE_UNAVAILABLE",
          },
        });
      return route.fulfill({ json: { items: apps } });
    }
    if (path.endsWith("/services/service_live/revoke")) {
      const app = apps.find(({ id }) => id === "service_live")!;
      app.state = "revoked";
      return route.fulfill({ json: app });
    }
    if (path.endsWith("/services/service_live/connect")) {
      state.connects++;
      return route.fulfill({
        json: {
          preview_url: `http://preview-0123456789abcdef0123456789abcdef.localhost:4173/connect#ticket=test${state.connects}`,
        },
      });
    }
    if (req.method() === "POST") {
      const body = req.postDataJSON();
      posts.push({ path, body });
      if (path === "/ui-api/work-items") {
        state.exists = true;
        if (body.empty_thread) {
          item.specification_path = "";
          item.specification_digest = "";
        }
        return route.fulfill({ status: 201, json: item });
      }
      if (path.endsWith("/run"))
        return route.fulfill({ status: 202, json: addRun() });
      if (path.endsWith("/review")) {
        item.state = "completed";
        attempts.at(-1)!.review_state = "accepted";
        return route.fulfill({ json: item });
      }
      if (path.endsWith("/continue")) {
        if (state.failContinue) {
          state.failContinue = false;
          return route.fulfill({
            status: 503,
            json: { code: "WORKSPACE_UNAVAILABLE" },
          });
        }
        if (body.workspace_id) item.workspace_id = body.workspace_id;
        if (!body.expected_attempt_id) {
          item.specification_path = "work-requests/first.md";
          item.specification_digest = "b".repeat(64);
          state.request = body.instruction;
        }
        const a = addRun(body.expected_attempt_id);
        a.message = body.instruction;
        return route.fulfill({ status: 202, json: a });
      }
      if (path.endsWith("/cancel")) {
        item.state = "cancelled";
        attempts.at(-1)!.state = "cancelled";
        return route.fulfill({ json: item });
      }
    }
    if (path === "/ui-api/projects")
      return route.fulfill({
        json: {
          items: [
            { id: "A", name: "Research", created_at: now },
            { id: "B", name: "Other project", created_at: now },
          ],
        },
      });
    if (path === "/ui-api/workspaces")
      return route.fulfill({
        json: {
          items:
            url.searchParams.get("project_id") === "B"
              ? []
              : [
                  {
                    id: "ws_one",
                    project_id: "A",
                    name: "Research worker",
                    agent_kind: "codex",
                    state: state.alternateWorker ? "STOPPED" : "READY",
                    ready: !state.alternateWorker,
                  },
                  ...(state.alternateWorker
                    ? [
                        {
                          id: "ws_two",
                          project_id: "A",
                          name: "Backup worker",
                          state: "STOPPED",
                          ready: false,
                        },
                      ]
                    : []),
                ],
        },
      });
    if (path === "/ui-api/work-items")
      return route.fulfill({
        json: {
          items:
            state.exists && url.searchParams.get("project_id") !== "B"
              ? [item]
              : [],
        },
      });
    if (path === "/ui-api/work-items/work_one")
      return route.fulfill({ json: item });
    if (path.endsWith("/attempts"))
      return route.fulfill({ json: { items: attempts } });
    if (path.endsWith("/instruction"))
      return route.fulfill({
        json: {
          project_id: "A",
          work_item_id: item.id,
          instruction: state.request,
        },
      });
    if (path.includes("/artifacts/"))
      return route.fulfill({
        body: state.report,
        contentType: "application/octet-stream",
      });
    if (path.endsWith("/history")) {
      const a = attempts.find((a) => a.id === path.split("/").at(-2));
      const task = a?.task_id;
      const episode = state.foreignHistory ? "task_foreign" : task;
      const record = (seq: number, kind: string, content: string) => ({
        id: `hist_${task}_${seq}`,
        seq,
        episode_id: episode,
        at: now,
        captured_at: now,
        source: "codex",
        kind,
        content,
        capture_scope: "live",
      });
      const records = a
        ? [
            record(
              1,
              "user.message",
              a.message && a.parent_attempt_id
                ? `Continue the same work in this workspace. Read the old request.\n\nCurrent request:\n${a.message}`
                : state.request,
            ),
            {
              ...record(2, "tool.call", "python3 -m unittest -v"),
              tool: "terminal",
            },
          ]
        : [];
      if (a?.state === "awaiting_review")
        records.push(record(3, "assistant.message", state.reply));
      return route.fulfill({
        json: {
          project_id: "A",
          workspace_id: a?.workspace_id || "ws_one",
          cache:
            state.alternateWorker && a?.workspace_id === "ws_one"
              ? { synced_at: now, cached_through: 3, source_high_watermark: 3 }
              : null,
          page: {
            records,
            next_before: 0,
            high_watermark: 3,
            schema_version: 1,
            bytes_used: 100,
            capture_limited: false,
          },
        },
      });
    }
    if (path.endsWith("/result"))
      return route.fulfill({ json: { content: "", next_offset: -1 } });
    if (path.endsWith("/changes"))
      return route.fulfill({ body: "null", contentType: "application/json" });
    if (path.endsWith("/files")) return route.fulfill({ json: { paths: [] } });
    return route.fulfill({ json: { items: [] } });
  });
  return { item, attempts, posts, state, addRun, finish, errors, apps, ports };
}

test("starts work, shows live tools, inspects files, accepts, and sends changes in one conversation", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  await page.goto("/app/projects/A/work");
  await page.getByLabel("Worker", { exact: true }).selectOption("ws_one");
  await page
    .getByLabel("Model", { exact: true })
    .selectOption(JSON.stringify(["claude", "chosen-model"]));
  await page.getByText("Keep specific output files", { exact: true }).click();
  await page.getByLabel("Retained output paths").fill("outputs/report.md");
  await page.getByLabel("Work profile").selectOption("software");
  await page
    .getByRole("button", { name: "Create thread", exact: true })
    .click();
  await expect(page).toHaveURL(/work=work_one/);
  expect(
    f.posts.find((p) => p.path === "/ui-api/work-items")?.body,
  ).toMatchObject({ provider: "claude", model: "chosen-model" });
  await page
    .getByLabel("Message to your agent")
    .fill("Investigate the experiment.");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(
    page
      .locator(".conversation-message")
      .getByText("Investigate the experiment.", { exact: true }),
  ).toBeVisible();
  await page.getByText("1 tool action", { exact: true }).click();
  await page.getByText("Terminal call", { exact: true }).click();
  await expect(
    page.getByText("python3 -m unittest -v", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Stop task", exact: true }),
  ).toBeVisible();
  f.finish();
  await expect(
    page.getByText("All four tests passed. The report is ready.", {
      exact: true,
    }),
  ).toBeVisible({ timeout: 10000 });
  await conversationTool(page, "Files");
  const files = page.getByRole("region", { name: "Task files" });
  await files
    .getByRole("button", { name: "outputs/report.md", exact: true })
    .click();
  await expect(
    files.getByText("All four tests passed.", { exact: false }),
  ).toBeVisible();
  expect(await page.evaluate(() => "untrusted" in window)).toBe(false);
  await page.getByRole("button", { name: "Conversation", exact: true }).click();
  await expect(page.getByText("Review output", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "Workspace", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Approve this revision" }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Task settings" }).click();
  const settings = page.getByRole("dialog", { name: "Task settings" });
  const approve = settings.getByRole("button", {
    name: "Approve this revision",
  });
  await approve.click();
  await expect(approve).toHaveCount(0);
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: "Conversation", exact: true }).click();
  await page
    .getByRole("button", { name: "Switch thread", exact: true })
    .click();
  await page.getByRole("button", { name: "Filter threads by status" }).click();
  await expect(
    page.getByRole("menuitemradio", { name: "Done (1)", exact: true }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await page.keyboard.press("Escape");
  await page.reload();
  await expect(
    page
      .locator(".conversation-thread-heading")
      .getByText("Accepted", { exact: true }),
  ).toHaveCount(0);
  await page
    .getByLabel("Message to your agent")
    .fill("Add a comparison experiment.");
  f.state.failContinue = true;
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(page.getByRole("alert").first()).toBeVisible();
  await expect(page.getByLabel("Message to your agent")).toHaveValue(
    "Add a comparison experiment.",
  );
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(page).not.toHaveURL(/attempt=/);
  await expect(
    page.getByText("Add a comparison experiment.", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Continue the same work in this workspace.", {
      exact: false,
    }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Stop task", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Retry task", exact: true }),
  ).toBeVisible();
  const retry = page
    .locator(".conversation-message")
    .getByRole("button", { name: "Retry task", exact: true });
  await expect(retry).toHaveCount(1);
  await expect(
    page
      .locator(".conversation-composer")
      .getByRole("button", { name: "Retry task" }),
  ).toHaveCount(0);
  await expect(
    page.getByText("Enter to send · Shift+Enter for a new line", {
      exact: true,
    }),
  ).toHaveCount(0);
  await retry.click();
  await expect(
    page.getByRole("button", { name: "Stop task", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Retry task", exact: true }),
  ).toHaveCount(0);
  const revisions = f.posts.filter(
    (p) => p.path.endsWith("/continue") && p.body.expected_attempt_id,
  );
  expect(revisions).toHaveLength(2);
  expect(revisions[0].body.idempotency_key).toBe(
    revisions[1].body.idempotency_key,
  );
  expect(revisions[1].body).toMatchObject({
    project_id: "A",
    instruction: "Add a comparison experiment.",
    expected_attempt_id: "attempt_1",
  });
  expect(f.errors).toEqual([]);
});

test("request changes focuses a message instead of silently rejecting the result", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  await page.goto("/app/projects/A/work?work=work_one");
  await page.getByRole("button", { name: "Start task", exact: true }).click();
  f.finish();
  await page.getByLabel("Message to your agent").click();
  await expect(page.getByLabel("Message to your agent")).toBeFocused();
  expect(f.posts.filter((p) => p.path.endsWith("/review"))).toHaveLength(0);
  await page
    .getByLabel("Message to your agent")
    .fill("Please include uncertainty estimates.");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(
    page.getByText("Please include uncertainty estimates.", { exact: true }),
  ).toBeVisible();
});

test("conversation is scoped to the selected task and project and fits narrow screens", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  await page.goto("/app/projects/A/work?work=work_one");
  await page.getByRole("button", { name: "Start task", exact: true }).click();
  f.finish();
  await expect(
    page.getByText("All four tests passed. The report is ready.", {
      exact: true,
    }),
  ).toBeVisible({ timeout: 10000 });
  for (const width of [1440, 1024, 390, 320]) {
    await page.setViewportSize({ width, height: 900 });
    await expect(
      page.getByRole("button", { name: "Workspace", exact: true }),
    ).toBeVisible();
    if (width >= 1024) {
      await expect
        .poll(async () => {
          const rail = await page.locator(".factory-sidebar").boundingBox();
          const header = await page.locator(".factory-header").boundingBox();
          return Math.abs(header!.x - rail!.x - rail!.width);
        })
        .toBeLessThanOrEqual(1);
    }
    await page.evaluate(() => scrollTo(0, 0));
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: `artifacts/work-conversation-${width}.png`,
      animations: "disabled",
      fullPage: true,
    });
  }
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.getByLabel("Message to your agent").fill("Private project draft");
  await page
    .getByRole("button", { name: "Switch project: Research", exact: true })
    .click();
  await page
    .getByRole("menuitem", { name: "Other project", exact: true })
    .click();
  await expect(
    page.getByText("All four tests passed. The report is ready.", {
      exact: true,
    }),
  ).toHaveCount(0);
  await expect(page.getByLabel("Message to your agent")).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Create thread", exact: true }),
  ).toBeVisible();
  f.state.foreignHistory = true;
  await page.goto("/app/projects/A/work?work=work_one");
  await expect(page.getByRole("alert").first()).toBeVisible();
  await expect(
    page
      .locator(".conversation-message")
      .getByText("Investigate the experiment.", { exact: true }),
  ).toHaveCount(0);
  expect(f.errors).toEqual([]);
});

test("starts and stops a workspace application from the conversation", async ({
  page,
  context,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  f.item.kind = "software";
  f.apps.push({
    id: "service_desktop",
    state: "active",
    ready: true,
    application: { kind: "desktop", port: 6080 },
  });
  f.ports.push({ port: 6080, listener: "listener_desktop" });
  await context.route(
    "http://preview-0123456789abcdef0123456789abcdef.localhost:4173/**",
    (route) =>
      route.fulfill({
        contentType: "text/html",
        body: "<h1>Preview application</h1>",
      }),
  );
  await page.goto("/app/projects/A/work?work=work_one");
  const preview = page.getByRole("complementary", {
    name: "Preview workspace",
  });
  if (
    !(await page
      .getByRole("button", { name: "Preview options", exact: true })
      .isVisible())
  )
    await conversationTool(page, "Preview");
  await expect(
    page.getByRole("tab", { name: "Desktop", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Ask agent to run app" }),
  ).toBeVisible();
  await previewTool(page, "Preview settings and logs");
  await page.getByLabel("Application directory").fill("my-app");
  await page.getByRole("button", { name: "Start preview" }).click();
  await expect(
    page
      .frameLocator('iframe[title="Application preview"]')
      .getByRole("heading"),
  ).toHaveText("Preview application");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  const popupPromise = page.waitForEvent("popup");
  await previewTool(page, "Open preview in new tab");
  const popup = await popupPromise;
  await expect(
    popup.getByRole("heading", { name: "Preview application" }),
  ).toBeVisible();
  await previewTool(page, "Application logs");
  await expect(
    page.getByText("Listening on port 4173", { exact: true }),
  ).toBeVisible();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Close", exact: true })
    .click();
  if (
    !(await page
      .getByRole("button", { name: "Preview options", exact: true })
      .isVisible())
  )
    await conversationTool(page, "Preview");
  await previewTool(page, "Preview settings and logs");
  await page.getByRole("button", { name: "Stop app" }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Close", exact: true })
    .click();
  await conversationTool(page, "Preview");
  await expect(
    preview.getByRole("button", { name: "Restart preview" }),
  ).toBeVisible();
  expect(
    f.posts.find((p) => p.path.endsWith("/applications"))?.body.application,
  ).toEqual({
    directory: "my-app",
    command: 'python3 -m http.server "$PORT" --bind 127.0.0.1',
    port: 4173,
  });
  await popup.close();
  expect(f.errors).toEqual([]);
});

test("web development shows interactive output alongside its web preview", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  f.item.kind = "software";
  f.state.reply =
    "```workbench-chart\nexperiment,score\nbaseline,72\nrevision,86\n```";
  await page.goto("/app/projects/A/work?work=work_one");
  await page.getByRole("button", { name: "Start task", exact: true }).click();
  f.finish();
  await page.reload();
  await page.getByRole("button", { name: "Workspace", exact: true }).click();
  await expect(page.getByRole("tab", { name: "Output" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(
    page.getByRole("img", { name: "score by experiment" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Explore", exact: true }).click();
  await page.getByLabel("Chart type").selectOption("line");
  await expect(
    page
      .getByRole("region", { name: "Agent output" })
      .locator(".browser-chart svg"),
  ).toHaveCount(1);
  await expect(
    page.getByRole("tab", { name: "Web", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("tab", { name: "Desktop", exact: true }),
  ).toHaveCount(0);
  expect(f.errors).toEqual([]);
});

test("desktop development opens directly to the worker desktop and keeps it on mobile", async ({
  page,
  context,
}) => {
  const f = await conversationFixture(page);
  f.item.kind = "desktop";
  f.apps.push({
    id: "service_web",
    state: "active",
    ready: true,
    application: { kind: "web", port: 4173 },
  });
  const portRequests: string[] = [];
  page.on("request", (request) => {
    if (new URL(request.url()).pathname.endsWith("/ports"))
      portRequests.push(request.url());
  });
  await context.route(
    "http://preview-0123456789abcdef0123456789abcdef.localhost:4173/**",
    (route) =>
      route.fulfill({
        contentType: "text/html",
        body: "<h1>Desktop connected</h1>",
      }),
  );
  await page.goto("/app/projects/A/work");
  await page.getByLabel("Work profile").selectOption({
    label: "Desktop App Development",
  });
  await page
    .getByLabel("Model", { exact: true })
    .selectOption({ label: "worker-model · Codex" });
  await page
    .getByRole("button", { name: "Create thread", exact: true })
    .click();
  await expect(
    page
      .getByRole("complementary", { name: "Preview workspace" })
      .getByText("Workspace desktop", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("tab", { name: "Desktop", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("combobox", { name: "Preview application" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("heading", { name: "Desktop is stopped" }),
  ).toBeVisible();
  await expect(page.getByRole("tab", { name: "Web", exact: true })).toHaveCount(
    0,
  );
  expect(f.posts.find((p) => p.path === "/ui-api/work-items")?.body.kind).toBe(
    "desktop",
  );
  await page.getByRole("button", { name: "Start worker desktop" }).click();
  await expect(
    page.frameLocator('iframe[title="Workspace desktop"]').getByRole("heading"),
  ).toHaveText("Desktop connected");
  const desktopPane = page.getByRole("complementary", {
    name: "Preview workspace",
  });
  const desktopFrame = page.locator('iframe[title="Workspace desktop"]');
  const desktopURL = await desktopFrame.getAttribute("src");
  await desktopPane.getByRole("button", { name: "Enter fullscreen" }).click();
  await expect(desktopPane.and(page.locator(":fullscreen"))).toBeVisible();
  await desktopPane.getByRole("button", { name: "Exit fullscreen" }).click();
  await expect(desktopPane.and(page.locator(":fullscreen"))).toHaveCount(0);
  await expect(desktopFrame).toHaveAttribute("src", desktopURL!);
  expect(
    f.posts.find((p) => p.path.endsWith("/applications"))?.body.application,
  ).toEqual({
    kind: "desktop",
    directory: ".",
    command: "workbench-desktop",
    port: 6080,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "Conversation", exact: true }).click();
  await expect(
    page.getByRole("complementary", { name: "Preview workspace" }),
  ).toBeHidden();
  await page.getByRole("button", { name: "Workspace", exact: true }).click();
  await expect(
    page.frameLocator('iframe[title="Workspace desktop"]').getByRole("heading"),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  expect(portRequests).toEqual([]);
  expect(f.errors).toEqual([]);

  const iframe = page.locator('iframe[title="Workspace desktop"]');
  const connectedURL = await iframe.getAttribute("src");
  f.state.applicationsError = 503;
  await page.waitForResponse(
    (res) => res.url().endsWith("/applications") && res.status() === 503,
  );
  await expect(iframe).toHaveAttribute("src", connectedURL!);
  f.state.applicationsError = 0;
  await page.waitForResponse(
    (res) => res.url().endsWith("/applications") && res.ok(),
  );
  await expect(iframe).toHaveAttribute("src", connectedURL!);

  const opened = page.waitForEvent("popup");
  await previewTool(page, "Open preview in new tab");
  const separated = await opened;
  await expect(separated.getByRole("heading")).toHaveText("Desktop connected");
  await expect(iframe).toHaveCount(0);
  await page
    .getByRole("button", { name: "Return desktop to this tab" })
    .click();
  await expect(iframe.contentFrame().getByRole("heading")).toHaveText(
    "Desktop connected",
  );
  await separated.close();
  f.state.applicationsError = 403;
  await expect(iframe).toHaveCount(0, { timeout: 10000 });
});

test("preview selection adds a draft only from the selected frame and reload renews the connection", async ({
  page,
  context,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  await context.route(
    "http://preview-0123456789abcdef0123456789abcdef.localhost:4173/**",
    (route) =>
      route.fulfill({
        contentType: "text/html",
        body: `<button id="hero">Build something</button><script>let nonce='';addEventListener('message',e=>{if(e.data.type==='workbench-select')nonce=e.data.nonce});document.querySelector('button').onclick=()=>parent.postMessage({type:'workbench-selection',nonce,selector:'#hero',text:'Build something',path:'/'},'http://127.0.0.1:4173')</script>`,
      }),
  );
  await page.goto("/app/projects/A/work?work=work_one");
  await page.getByRole("button", { name: "Start task", exact: true }).click();
  f.finish();
  await expect(page.getByText(f.state.reply, { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Workspace", exact: true }).click();
  if (
    !(await page
      .getByRole("button", { name: "Preview options", exact: true })
      .isVisible())
  )
    await conversationTool(page, "Preview");
  await previewTool(page, "Preview settings and logs");
  await page
    .getByRole("button", { name: "Start preview", exact: true })
    .click();
  const frame = page.frameLocator('iframe[title="Application preview"]');
  await expect(frame.getByRole("button")).toBeVisible();
  const previewPane = page.getByRole("complementary", {
    name: "Preview workspace",
  });
  await previewPane.getByRole("button", { name: "Enter fullscreen" }).click();
  await expect(previewPane.and(page.locator(":fullscreen"))).toBeVisible();
  await previewTool(page, "Select element to change");
  await page.evaluate(() =>
    window.postMessage(
      {
        type: "workbench-selection",
        nonce: "forged",
        selector: "#attack",
        text: "untrusted",
        path: "/",
      },
      location.origin,
    ),
  );
  await expect(page.getByLabel("Message to your agent")).toHaveValue("");
  await frame.getByRole("button").click();
  await expect(previewPane.and(page.locator(":fullscreen"))).toHaveCount(0);
  await expect(page.getByLabel("Message to your agent")).toHaveValue(
    'Change #hero on / ("Build something"): ',
  );
  await expect(page.getByLabel("Message to your agent")).toBeFocused();
  expect(f.posts.filter((p) => p.path.endsWith("/continue"))).toHaveLength(0);
  const renewed = page.waitForRequest(
    (r) => r.url().endsWith("/connect") && r.method() === "POST",
  );
  await page
    .getByRole("button", { name: "Reload preview", exact: true })
    .click();
  expect((await renewed).postDataJSON()).toEqual({ embedded: true });
  await expect(frame.getByRole("button")).toBeVisible();
  expect(f.errors).toEqual([]);
});

test("application logs have room to debug, pause updates, filter and draft an agent request", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  f.apps.push({
    id: "service_failed",
    state: "stopped",
    ready: false,
    application: { directory: "my-app", command: "npm run dev", port: 4173 },
    logs: "Starting server\nError: address already in use\n  at listen (server.js:42)",
  });
  await page.goto("/app/projects/A/work?work=work_one");
  await page.getByRole("button", { name: "Start task", exact: true }).click();
  f.finish();
  await expect(page.getByText(f.state.reply, { exact: true })).toBeVisible();
  await conversationTool(page, "Preview");
  await previewTool(page, "Application logs");
  const dialog = page.getByRole("dialog", {
    name: "Application logs",
    exact: true,
  });
  const output = dialog.getByLabel("Application log output");
  const bounds = await dialog.boundingBox();
  const viewport = page.viewportSize()!;
  expect(bounds!.width).toBeGreaterThan(viewport.width * 0.85);
  expect((await output.boundingBox())!.height).toBeGreaterThan(250);
  await expect(dialog.getByText("npm run dev", { exact: true })).toBeVisible();
  await dialog.getByRole("button", { name: "Pause", exact: true }).click();
  f.apps[0].logs += "\nNEW: connection refused";
  // Wait for an actual polling response; the frozen snapshot must stay unchanged.
  await page.waitForResponse(
    (r) => r.url().endsWith("/applications") && r.request().method() === "GET",
  );
  await expect(output).not.toContainText("NEW:");
  await dialog.getByRole("button", { name: "Resume", exact: true }).click();
  await expect(output).toContainText("NEW: connection refused");
  await dialog.getByLabel("Filter log lines").fill("address");
  await expect(output).toHaveText("Error: address already in use");
  await dialog.getByRole("button", { name: "Wrap", exact: true }).click();
  await expect(output).not.toHaveClass(/is-wrapped/);
  const downloading = page.waitForEvent("download");
  await dialog.getByRole("button", { name: "Download", exact: true }).click();
  expect((await downloading).suggestedFilename()).toBe("service_failed.log");
  await page.setViewportSize({ width: 320, height: 740 });
  expect(await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(
    true,
  );
  expect((await output.boundingBox())!.height).toBeGreaterThan(150);
  await dialog
    .getByRole("button", { name: "Ask agent to debug", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByLabel("Message to your agent")).toHaveValue(
    /Error: address already in use/,
  );
  await expect(page.getByLabel("Message to your agent")).toHaveValue(
    /Command: npm run dev/,
  );
  await expect(page.getByLabel("Message to your agent")).toBeFocused();
  expect(f.posts.filter((p) => p.path.endsWith("/continue"))).toHaveLength(0);
  expect(f.errors).toEqual([]);
});

test("detects an agent-started web app and keeps it connected when preview is collapsed", async ({
  page,
  context,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  f.ports.push({ port: 3000, listener: "listener_live" });
  await context.route(
    "http://preview-0123456789abcdef0123456789abcdef.localhost:4173/**",
    (route) =>
      route.fulfill({
        contentType: "text/html",
        body: "<h1>Agent app on 3000</h1>",
      }),
  );
  await page.goto("/app/projects/A/work?work=work_one");
  await conversationTool(page, "Preview");
  const frame = page.frameLocator('iframe[title="Application preview"]');
  await expect(frame.getByRole("heading")).toHaveText("Agent app on 3000");
  expect(
    f.posts.find((p) => p.path.endsWith("/applications"))?.body.application,
  ).toMatchObject({ kind: "forward", port: 3000, listener: "listener_live" });
  await page.getByRole("button", { name: "Conversation", exact: true }).click();
  await expect(
    page.getByRole("complementary", { name: "Preview workspace" }),
  ).toBeHidden();
  await expect(
    page.getByRole("separator", { name: "Resize conversation and preview" }),
  ).toBeHidden();
  await expect(page.getByRole("region", { name: "Agent output" })).toHaveCount(
    0,
  );
  await conversationTool(page, "Preview");
  await expect(frame.getByRole("heading")).toBeVisible();
  expect(f.posts.filter((p) => p.path.endsWith("/applications"))).toHaveLength(
    1,
  );
  f.apps[0].state = "revoked";
  await conversationTool(page, "Preview");
  await expect(
    page.getByRole("button", { name: "Open detected app" }),
  ).toBeVisible();
  expect(f.posts.filter((p) => p.path.endsWith("/applications"))).toHaveLength(
    1,
  );
  await page.getByRole("button", { name: "Open detected app" }).click();
  await expect(frame.getByRole("heading")).toBeVisible();
  expect(f.posts.filter((p) => p.path.endsWith("/applications"))).toHaveLength(
    2,
  );
  expect(f.errors).toEqual([]);
});

test("preview asks the agent to launch inside the workspace", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  await page.goto("/app/projects/A/work?work=work_one");
  await page.getByRole("button", { name: "Start task", exact: true }).click();
  f.finish();
  await conversationTool(page, "Preview");
  await page
    .getByRole("button", { name: "Ask agent to run app", exact: true })
    .click();
  await expect(page.getByLabel("Message to your agent")).toHaveValue(
    /inside the workspace using workbench-preview/,
  );
  await expect(page.getByLabel("Message to your agent")).toBeFocused();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(f.posts.filter((p) => p.path.endsWith("/continue"))).toHaveLength(0);
});

test("renders a generated SVG in the reply without an empty chart panel or executing SVG scripts", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  f.state.reply =
    "Disk has **7.36 GiB available**.\n\n![Disk space visualization](outputs/disk-space.svg)\n\n![Blocked](../secret.svg)";
  let imageRequests = 0;
  await page.route("**/ui-api/work-items/work_one/image", async (route) => {
    imageRequests++;
    expect(route.request().postDataJSON()).toEqual({
      project_id: "A",
      attempt_id: "attempt_1",
      path: "outputs/disk-space.svg",
    });
    await route.fulfill({
      json: {
        path: "outputs/disk-space.svg",
        content: Buffer.from(
          '<svg xmlns="http://www.w3.org/2000/svg" width="600" height="200"><script>parent.alert("unsafe")</script><rect width="580" height="80" fill="#43b581"/><text y="130">7.36 GiB available</text></svg>',
        ).toString("base64"),
      },
    });
  });
  const dialogs: string[] = [];
  page.on("dialog", (d) => {
    dialogs.push(d.message());
    void d.dismiss();
  });
  await page.goto("/app/projects/A/work?work=work_one");
  await page.getByRole("button", { name: "Start task", exact: true }).click();
  f.finish();
  const img = page.getByRole("img", { name: "Disk space visualization" });
  await expect(img).toBeVisible();
  await expect
    .poll(() => img.evaluate((el: HTMLImageElement) => el.naturalWidth))
    .toBe(600);
  await expect(page.locator(".message-markdown strong")).toHaveText(
    "7.36 GiB available",
  );
  await expect(
    page.getByText("Explore the results", { exact: true }),
  ).toHaveCount(0);
  expect(dialogs).toEqual([]);
  const loadedRequests = imageRequests;
  const loadedImage = await img.getAttribute("src");
  await page.waitForResponse(
    (r) => r.url().endsWith("/applications") && r.request().method() === "GET",
  );
  expect(imageRequests).toBe(loadedRequests);
  await expect(img).toHaveAttribute("src", loadedImage!);
  for (const width of [1920, 1280, 940, 390]) {
    await page.setViewportSize({ width, height: 900 });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
  }
  expect(f.errors).toEqual([]);
});

test("preview keeps its status card through a failed check and retry until the app is ready", async ({
  page,
  context,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  f.state.applicationsError = 503;
  await context.route(
    "http://preview-0123456789abcdef0123456789abcdef.localhost:4173/**",
    (route) =>
      route.fulfill({ contentType: "text/html", body: "Live preview" }),
  );
  await page.goto("/app/projects/A/work?work=work_one");
  await conversationTool(page, "Preview");
  const card = page.locator(".preview-recovery");
  const heading = card.getByRole("heading", { name: "Preview unavailable" });
  const retry = card.getByRole("button", { name: "Try again", exact: true });
  await expect(retry).toBeVisible();

  const app = {
    id: "service_live",
    state: "active",
    ready: false,
    application: { directory: "app", port: 3000, command: "serve" },
  };
  let finishCheck!: () => void;
  const pendingCheck = new Promise<void>((resolve) => {
    finishCheck = resolve;
  });
  await page.route(
    "**/ui-api/workspaces/ws_one/applications",
    async (route) => {
      await pendingCheck;
      await route.fulfill({ json: { items: [app] } });
    },
  );
  try {
    await retry.click();
    await expect(heading).toBeVisible();
    await expect(
      card.getByRole("button", { name: "Checking…", exact: true }),
    ).toBeDisabled();
    await expect(card.getByRole("status")).toHaveText(
      "Checking for running applications…",
    );
    await expect(card.getByText("Support details")).toBeVisible();
  } finally {
    finishCheck();
  }
  await expect(card.getByRole("status")).toHaveText(
    "Waiting for the app to respond…",
  );
  await expect(heading).toBeVisible();
  app.ready = true;
  await expect(
    page.frameLocator('iframe[title="Application preview"]').locator("body"),
  ).toHaveText("Live preview");
  await expect(card).toHaveCount(0);
});

test("refreshes web preview when a run completes, preserves it during work, and clears a stopped app", async ({
  page,
  context,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  f.apps.push({
    id: "service_live",
    state: "active",
    ready: true,
    application: { directory: "app", port: 3000, command: "serve" },
  });
  let version = "Before change";
  let loads = 0;
  await context.route(
    "http://preview-0123456789abcdef0123456789abcdef.localhost:4173/**",
    (route) => {
      loads++;
      return route.fulfill({
        contentType: "text/html",
        body: `<h1>${version}</h1>`,
      });
    },
  );
  await page.goto("/app/projects/A/work?work=work_one");
  await conversationTool(page, "Preview");
  const frame = page.frameLocator('iframe[title="Application preview"]');
  await expect(frame.getByRole("heading")).toHaveText("Before change");
  await page.getByRole("button", { name: "Start task", exact: true }).click();
  expect(loads).toBe(1);
  version = "After agent change";
  f.finish();
  await expect(frame.getByRole("heading")).toHaveText("After agent change");
  const afterFirstRun = loads;
  await page.getByLabel("Message to your agent").fill("One more change");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Stop task", exact: true }),
  ).toBeVisible();
  expect(loads).toBe(afterFirstRun);
  f.apps[0].state = "revoked";
  f.finish();
  await expect(page.locator('iframe[title="Application preview"]')).toHaveCount(
    0,
  );
  await expect(
    page.getByRole("heading", { name: "Preview unavailable" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Restart preview", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Workspace" })).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Reconnect preview" }),
  ).toHaveCount(0);
  expect(f.errors).toEqual([]);
});

test("explores agent-supplied data in the browser and drafts a question about a selected row", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  const spec = {
    version: 1,
    title: "Workspace storage",
    type: "donut",
    x: "category",
    y: ["bytes"],
    unit: "bytes",
    data: [
      { category: "Used", bytes: 1024 },
      { category: "Available", bytes: 8192 },
      { category: "Reserved", bytes: 512 },
    ],
  };
  f.state.reply =
    "Here are the measured values.\n\n```workbench-chart\n" +
    JSON.stringify(spec) +
    "\n```";
  await page.goto("/app/projects/A/work?work=work_one");
  await page.getByRole("button", { name: "Start task", exact: true }).click();
  f.finish();
  const chart = page.getByRole("region", {
    name: "Workspace storage",
    exact: true,
  });
  await expect(chart.locator(".browser-chart svg")).toHaveCount(1);
  await expect(chart.getByText("3 of 3 rows", { exact: true })).toBeVisible();
  await chart.getByRole("button", { name: "Explore", exact: true }).click();
  await chart.getByLabel("Chart type").selectOption("bar");
  await chart.getByLabel("Filter chart rows").fill("Available");
  await expect(chart.getByText("1 of 3 rows", { exact: true })).toBeVisible();
  await chart.getByRole("button", { name: "Data", exact: true }).click();
  await expect(
    chart.getByRole("cell", { name: "8192", exact: true }),
  ).toBeVisible();
  await chart
    .getByRole("button", { name: "Inspect row 1", exact: true })
    .click();
  await chart
    .getByRole("button", { name: "Ask agent about this", exact: true })
    .click();
  await expect(page.getByLabel("Message to your agent")).toHaveValue(
    /Workspace storage.*Available.*8192/,
  );
  await expect(page.getByLabel("Message to your agent")).toBeFocused();
  expect(f.posts.filter((p) => p.path.endsWith("/continue"))).toHaveLength(0);
  await chart.getByRole("button", { name: "Export chart" }).click();
  const downloaded = page.waitForEvent("download");
  await page
    .getByRole("menuitem", { name: "Filtered rows (CSV)", exact: true })
    .click();
  expect((await downloaded).suggestedFilename()).toBe("chart-data.csv");
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  expect(f.errors).toEqual([]);
});

test("reads a linked workspace dataset without requiring a server or retained image", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  f.state.reply = "[Explore experiments](outputs/experiments.chart.json)";
  let reads = 0;
  await page.route("**/ui-api/work-items/work_one/data", async (route) => {
    reads++;
    expect(route.request().postDataJSON()).toEqual({
      project_id: "A",
      attempt_id: "attempt_1",
      path: "outputs/experiments.chart.json",
    });
    const spec = {
      version: 1,
      title: "Experiment results",
      type: "line",
      x: "trial",
      y: ["baseline", "revision"],
      data: [
        { trial: "A", baseline: 72, revision: 86 },
        { trial: "B", baseline: 75, revision: 88 },
      ],
    };
    return route.fulfill({
      json: { content: Buffer.from(JSON.stringify(spec)).toString("base64") },
    });
  });
  await page.goto("/app/projects/A/work?work=work_one");
  await page.getByRole("button", { name: "Start task", exact: true }).click();
  f.finish();
  const chart = page.getByRole("region", {
    name: "Experiment results",
    exact: true,
  });
  await expect(chart.locator(".browser-chart svg")).toHaveCount(1);
  await chart.getByRole("button", { name: "Explore", exact: true }).click();
  await chart
    .getByRole("checkbox", { name: "revision", exact: true })
    .uncheck();
  await expect(
    chart.getByRole("img", { name: "baseline by trial", exact: true }),
  ).toBeVisible();
  const count = reads;
  await page.waitForResponse(
    (r) => r.url().endsWith("/applications") && r.request().method() === "GET",
  );
  expect(reads).toBe(count);
  expect(f.posts.filter((p) => p.path.endsWith("/applications"))).toHaveLength(
    0,
  );
  expect(f.errors).toEqual([]);
});

test("thread list shows types and can be hidden and restored", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  f.item.kind = "software";
  f.item.specification_path = "work-requests/original.md";
  const second = {
    ...f.item,
    id: "work_two",
    kind: "desktop",
    specification_path: "work-requests/second.md",
  };
  const third = {
    ...f.item,
    id: "work_three",
    kind: "data-science",
    specification_path: "work-requests/third.md",
  };
  await page.route("**/ui-api/work-items?project_id=A", (route) =>
    route.fulfill({ json: { items: [f.item, second, third] } }),
  );
  await page.route("**/ui-api/work-items/work_two?project_id=A", (route) =>
    route.fulfill({ json: second }),
  );
  await page.route(
    "**/ui-api/work-items/*/instruction?project_id=A",
    (route) => {
      const id = new URL(route.request().url()).pathname.split("/").at(-2);
      const requests = {
        work_one: "Build a photo gallery",
        work_two: "Fix the login form",
        work_three:
          "Selected dataset for this request: dataset_abcd.\n\nCompare monthly totals",
      };
      return route.fulfill({
        json: {
          project_id: "A",
          work_item_id: id,
          instruction:
            "Workbench presentation: interactive-v1.\nChart support.\n\nCurrent request:\n" +
            requests[id as keyof typeof requests],
        },
      });
    },
  );
  await page.setViewportSize({ width: 1600, height: 1000 });
  await page.goto("/app/projects/A/work");
  const list = page.getByRole("complementary", { name: "Threads" });
  const expectControlHeight = async (height: string) => {
    await expect(list.getByRole("button", { name: "New thread" })).toHaveCSS(
      "height",
      height,
    );
    await expect(
      list.getByRole("textbox", { name: "Search threads" }),
    ).toHaveCSS("height", height);
    await expect(
      list.getByRole("button", { name: "Filter threads by status" }),
    ).toHaveCSS("height", height);
  };
  await expectControlHeight("36px");
  const gallery = list.getByRole("button", { name: "Build a photo gallery" });
  for (const [name, type] of [
    ["Build a photo gallery", "Web"],
    ["Fix the login form", "Desktop"],
    ["Compare monthly totals", "Research"],
  ]) {
    const row = list.getByRole("button", { name });
    await expect(row).toBeVisible();
    await expect(row).toHaveAccessibleDescription(`${type}. Draft`);
    await expect(row.getByText(type, { exact: true })).toHaveCount(0);
    await expect(row.getByText("Draft", { exact: true })).toHaveCount(0);
    await expect(row.locator(".thread-state")).toBeVisible();
  }
  await expect(gallery.locator(".conversation-work-meta")).toHaveCount(0);
  const date = (await gallery.locator("time").boundingBox())!;
  const status = (await gallery.locator(".thread-state").boundingBox())!;
  expect(
    Math.abs(date.y + date.height / 2 - status.y - status.height / 2),
  ).toBeLessThan(1);
  const resize = page.getByRole("separator", { name: "Resize thread list" });
  await resize.press("Home");
  await expect(resize).toHaveAttribute("aria-valuenow", "200");
  await page.getByRole("textbox", { name: "Search threads" }).fill("gallery");
  await expect(
    list.getByRole("button", { name: "Fix the login form" }),
  ).toHaveCount(0);
  await list.getByRole("button", { name: "Build a photo gallery" }).click();
  await expect(
    page.getByRole("heading", { name: "Build a photo gallery", exact: true }),
  ).toBeVisible();
  await list.getByRole("button", { name: "Clear filters" }).click();
  const collapsed = page.getByRole("complementary", {
    name: "Thread panel",
    exact: true,
  });
  const show = collapsed.getByRole("button", { name: "Show thread panel" });
  const hide = list.getByRole("button", { name: "Hide thread panel" });
  const create = list.getByRole("button", { name: "New thread" });
  const newThread = (await create.boundingBox())!;
  const collapsePanel = (await hide.boundingBox())!;
  expect(collapsePanel.y).toBe(newThread.y);
  expect(collapsePanel.height).toBe(newThread.height);
  const session = page.getByRole("region", { name: "Work conversation" });
  const dockedWidth = (await session.boundingBox())!.width;
  await hide.click();
  await expect(show).toBeFocused();
  await expect(show).toHaveCSS("height", `${collapsePanel.height}px`);
  await expect(show).toHaveCSS("width", `${collapsePanel.width}px`);
  await expect(collapsed).toHaveCSS("width", "48px");
  await expect
    .poll(async () => {
      const railHeading = (await collapsed
        .locator(".thread-rail-heading")
        .boundingBox())!;
      const threadHeading = (await page
        .locator(".conversation-thread-heading")
        .boundingBox())!;
      return Math.abs(
        railHeading.y +
          railHeading.height -
          threadHeading.y -
          threadHeading.height,
      );
    })
    .toBeLessThan(1);
  await expect(list).toBeHidden();
  const compactGallery = collapsed.getByRole("button", {
    name: "Build a photo gallery",
  });
  const compactDesktop = collapsed.getByRole("button", {
    name: "Fix the login form",
  });
  await compactGallery.hover();
  const arrow = page.locator('.thread-tooltip [data-slot="tooltip-arrow"]');
  await expect(arrow).toHaveCSS("border-right-width", "1px");
  await expect(arrow).toHaveCSS("border-bottom-width", "1px");
  const tooltip = page.locator('[data-slot="tooltip-content"] > .thread-row');
  await expect(tooltip.locator(".conversation-work-name")).toHaveText(
    "Build a photo gallery",
  );
  await expect(tooltip.locator(".thread-type-icon svg")).toBeVisible();
  await expect(tooltip.locator(".thread-state")).toHaveAttribute(
    "data-state",
    "draft",
  );
  await expect(tooltip.locator("time")).toHaveAttribute(
    "datetime",
    f.item.created_at,
  );
  await expect(page.getByRole("tooltip")).toContainText("Web. Draft");
  const restoreBounds = (await show.boundingBox())!;
  const icon = compactGallery.locator(".thread-type-icon svg");
  const iconBounds = (await icon.boundingBox())!;
  expect(iconBounds.x + iconBounds.width / 2).toBeCloseTo(
    restoreBounds.x + restoreBounds.width / 2,
    0,
  );
  await expect(compactGallery).toHaveAttribute("aria-pressed", "true");
  await expect(compactDesktop.locator(".thread-type-icon svg")).toBeVisible();
  await compactDesktop.click();
  await expect(compactDesktop).toHaveAttribute("aria-pressed", "true");
  await expect(
    page.getByRole("heading", { name: "Fix the login form", exact: true }),
  ).toBeVisible();
  await compactGallery.click();
  await expect(compactGallery).toHaveAttribute("aria-pressed", "true");
  expect((await session.boundingBox())!.width).toBeGreaterThan(dockedWidth);
  await page.getByRole("button", { name: "Workspace", exact: true }).click();
  await expect(list).toBeHidden();
  await page.reload();
  await expect(show).toHaveAttribute("aria-expanded", "false");
  await show.click();
  await expect(hide).toBeFocused();
  await expect(resize).toHaveAttribute("aria-valuenow", "200");

  await page.setViewportSize({ width: 1366, height: 650 });
  await expect(hide).toBeHidden();
  await expect(collapsed).toHaveCSS("width", "48px");
  const switchThread = collapsed.getByRole("button", { name: "Switch thread" });
  await expect(switchThread).toHaveCSS("width", "36px");
  await expect(switchThread).toHaveCSS("height", "36px");
  await expect(compactDesktop).toHaveCSS("height", "44px");
  await compactDesktop.click();
  await expect(compactDesktop).toHaveAttribute("aria-pressed", "true");

  await page.setViewportSize({ width: 390, height: 844 });
  await expect(switchThread).toHaveCSS("width", "36px");
  await expect(switchThread).toHaveCSS("height", "36px");
  const switchBounds = (await switchThread.boundingBox())!;
  const railBounds = (await collapsed.boundingBox())!;
  expect(switchBounds.x - railBounds.x).toBeGreaterThan(5);
  expect(
    railBounds.x + railBounds.width - switchBounds.x - switchBounds.width,
  ).toBeGreaterThan(5);
  await page.mouse.click(
    switchBounds.x - 3,
    switchBounds.y + switchBounds.height / 2,
  );
  await expect(gallery).toBeVisible();
  await expectControlHeight("44px");
  await create.click();
  await expect(
    page.getByRole("heading", { name: "New thread", exact: true }),
  ).toBeVisible();
  await expect(switchThread).toBeFocused();
  await expect(compactGallery).toBeVisible();
  await page.setViewportSize({ width: 1600, height: 1000 });
  await expect(hide).toBeVisible();
  await expect(resize).toHaveAttribute("aria-valuenow", "200");
  expect(f.errors).toEqual([]);
});

test("large thread lists load on demand and search every title with honest failure recovery", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  const items = Array.from({ length: 200 }, (_, index) => ({
    ...f.item,
    id: `work_${String(index).padStart(3, "0")}`,
    specification_path: `work-requests/${index}.md`,
  }));
  await page.route("**/ui-api/work-items?project_id=A", (route) =>
    route.fulfill({ json: { items } }),
  );
  const reads: string[] = [];
  let active = 0,
    maximum = 0,
    failLast = true;
  await page.route(
    "**/ui-api/work-items/*/instruction?project_id=A",
    async (route) => {
      const id = new URL(route.request().url()).pathname.split("/").at(-2)!;
      reads.push(id);
      maximum = Math.max(maximum, ++active);
      await new Promise((resolve) => setTimeout(resolve, 25));
      active--;
      if (id === "work_199" && failLast)
        return route.fulfill({
          status: 503,
          json: { error: "INSTRUCTION_UNAVAILABLE" },
        });
      return route.fulfill({
        json: {
          project_id: "A",
          work_item_id: id,
          instruction:
            id === "work_130"
              ? "Unique far review"
              : id === "work_199"
                ? "Unique far reconciliation"
                : `Routine request ${id}`,
        },
      });
    },
  );
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/app/projects/A/work");
  const rail = page.locator("#thread-sidebar");
  await expect(
    rail.getByRole("button", { name: "Routine request work_019" }),
  ).toBeVisible();
  expect(reads).toHaveLength(20);
  await expect(rail.locator(".conversation-work-item")).toHaveCount(20);
  await expect(
    rail.getByText("Showing 20 of 200 threads", { exact: true }),
  ).toBeVisible();
  await rail.getByRole("button", { name: "Show more threads" }).click();
  await expect(
    rail.getByRole("button", { name: "Routine request work_039" }),
  ).toBeVisible();
  expect(reads).toHaveLength(40);
  await expect(rail.locator(".conversation-work-item")).toHaveCount(40);
  await rail
    .getByRole("textbox", { name: "Search threads" })
    .fill(" Unique far ");
  await expect(rail.getByText(/Searching thread titles/)).toBeVisible();
  await expect(rail.getByText("No threads match these filters.")).toHaveCount(
    0,
  );
  await expect(
    rail.getByText(/Search results may be incomplete/),
  ).toBeVisible();
  await expect(
    rail.getByRole("button", { name: "Unique far review" }),
  ).toBeVisible();
  await expect(
    rail.getByRole("button", { name: "Retry titles" }),
  ).toBeEnabled();
  expect(new Set(reads).size).toBe(200);
  expect(maximum).toBeLessThanOrEqual(4);
  await expect(
    rail.getByText("Showing 1 of 1 available match", { exact: true }),
  ).toBeVisible();
  for (const width of [1440, 320]) {
    await page.setViewportSize({ width, height: 900 });
    for (const theme of ["dark", "light"]) {
      const toggle = page.getByRole("button", {
        name: `Switch to ${theme} theme`,
        exact: true,
      });
      if (await toggle.count()) await toggle.click();
      if (width === 320)
        await page
          .getByRole("button", { name: "Switch thread", exact: true })
          .click();
      const panel =
        width === 320
          ? page.getByRole("dialog", { name: "Threads", exact: true })
          : rail;
      await expect(
        panel.getByRole("button", { name: "Retry titles", exact: true }),
      ).toBeVisible();
      if (width === 320)
        expect(
          (await panel
            .getByRole("button", { name: "Retry titles", exact: true })
            .boundingBox())!.height,
        ).toBeGreaterThanOrEqual(44);
      await page.waitForFunction(
        () =>
          !document
            .getAnimations()
            .some(
              (a) => a instanceof CSSTransition && a.playState === "running",
            ),
      );
      const audit = await new AxeBuilder({ page })
        .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
        .analyze();
      expect(audit.violations, `${theme} thread search at ${width}px`).toEqual(
        [],
      );
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
      if (width === 320) await page.keyboard.press("Escape");
    }
  }
  await page.setViewportSize({ width: 1440, height: 900 });
  failLast = false;
  await rail.getByRole("button", { name: "Retry titles" }).click();
  await expect(
    rail.getByRole("button", {
      name: "Unique far reconciliation",
    }),
  ).toBeVisible();
  await expect(rail.getByText(/Search results may be incomplete/)).toHaveCount(
    0,
  );
  await expect(
    rail.getByText("Showing 2 of 2 matches", { exact: true }),
  ).toBeVisible();
  await page.screenshot({ path: "artifacts/thread-search-large.png" });
  await rail.getByRole("button", { name: "Clear filters" }).click();
  await expect(rail.locator(".conversation-work-item")).toHaveCount(20);
  expect(f.posts).toEqual([]);
  expect(f.errors).toEqual([]);
});

test("thread titles refresh when a follow-up changes the saved request", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  f.item.specification_path = "work-requests/original.md";
  await page.goto("/app/projects/A/work");
  const rail = page.locator("#thread-sidebar");
  await expect(
    rail.getByRole("button", { name: f.state.request }),
  ).toBeVisible();
  f.state.request = "A revised request from another tab";
  f.item.specification_digest = "b".repeat(64);
  f.item.specification_path = "work-requests/revision.md";
  await expect(rail.getByRole("button", { name: f.state.request })).toBeVisible(
    { timeout: 10000 },
  );
  expect(f.errors).toEqual([]);
});

test("shows streamed progress before work ends and falls back to recorded history", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  const attempt = f.addRun();
  const progress = "I’m checking the recorded results.";
  await page.route("**/history/events?*", (route) => {
    expect(new URL(route.request().url()).searchParams.get("after")).toBe("2");
    const data = {
      project_id: "A",
      workspace_id: attempt.workspace_id,
      page: {
        records: [
          {
            id: "hist_progress",
            seq: 3,
            episode_id: attempt.task_id,
            at: attempt.created_at,
            captured_at: attempt.created_at,
            source: "codex",
            kind: "assistant.progress",
            content: progress,
            capture_scope: "live",
          },
        ],
        schema_version: 1,
        next_before: 0,
        high_watermark: 3,
        bytes_used: 100,
        capture_limited: false,
      },
    };
    return route.fulfill({
      contentType: "text/event-stream",
      body: `id: 3\nevent: history\ndata: ${JSON.stringify(data)}\n\nevent: unavailable\ndata: {}\n\n`,
    });
  });
  await page.goto("/app/projects/A/work?work=work_one");
  await expect(page.getByText(progress, { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Stop task" })).toBeVisible();
  f.finish();
  await expect(page.getByText(f.state.reply, { exact: true })).toBeVisible();
  await expect(page.getByText(progress, { exact: true })).toHaveCount(1);
  expect(f.errors).toEqual([]);
});

test("reading older activity preserves position and only new records announce new activity", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  f.item.state = "running";
  f.item.latest_attempt_id = "attempt_long";
  f.attempts.push({
    id: "attempt_long",
    work_item_id: f.item.id,
    workspace_id: "ws_one",
    task_id: "task_long",
    state: "running",
    created_at: f.item.created_at,
    updated_at: f.item.updated_at,
    artifacts: [],
  });
  let latest = 60;
  const record = (seq: number) => ({
    id: `record_${seq}`,
    seq,
    episode_id: "task_long",
    at: f.item.created_at,
    captured_at: f.item.created_at,
    source: "codex",
    kind: "assistant.message",
    content:
      `Recorded update ${seq}.\n\n` +
      "Detailed findings remain readable while the task continues. ".repeat(6),
    capture_scope: "live",
  });
  await page.route(
    "**/ui-api/work-items/work_one/attempts/*/history?*",
    (route) => {
      const params = new URL(route.request().url()).searchParams;
      const after = Number(params.get("after"));
      const older = params.has("before");
      const first = older ? 1 : after ? after + 1 : 21;
      const last = older ? 20 : latest;
      return route.fulfill({
        json: {
          project_id: "A",
          workspace_id: "ws_one",
          page: {
            records: Array.from(
              { length: Math.max(0, last - first + 1) },
              (_, i) => record(first + i),
            ),
            next_before: older ? 0 : 21,
            high_watermark: latest,
            schema_version: 1,
            bytes_used: 100,
            capture_limited: false,
          },
        },
      });
    },
  );
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/app/projects/A/work?work=work_one");
  const feed = page.locator(".conversation-feed");
  await expect(feed.locator(".conversation-message")).toHaveCount(40);
  await feed.evaluate((element) => {
    element.scrollTop = 0;
    element.dispatchEvent(new Event("scroll"));
  });
  await page
    .getByRole("button", { name: "Load earlier activity", exact: true })
    .click();
  await expect(feed.locator(".conversation-message")).toHaveCount(60);
  await expect(
    page.getByRole("button", { name: "New activity", exact: true }),
  ).toHaveCount(0);
  await feed.evaluate((element) => {
    element.scrollTop = 600;
    element.dispatchEvent(new Event("scroll"));
  });
  const readingPosition = await feed.evaluate((element) => element.scrollTop);
  latest = 63;
  await expect(
    page.getByRole("button", { name: "New activity", exact: true }),
  ).toBeVisible({ timeout: 10000 });
  expect(await feed.evaluate((element) => element.scrollTop)).toBe(
    readingPosition,
  );
  await expect(feed.locator(".conversation-message")).toHaveCount(63);
  await page.getByRole("button", { name: "New activity", exact: true }).click();
  await expect
    .poll(() =>
      feed.evaluate(
        (element) =>
          element.scrollHeight - element.scrollTop - element.clientHeight,
      ),
    )
    .toBeLessThan(3);
  await feed.focus();
  await expect(feed).toBeFocused();
  await page.keyboard.press("PageUp");
  await expect
    .poll(() =>
      feed.evaluate(
        (element) =>
          element.scrollHeight - element.scrollTop - element.clientHeight,
      ),
    )
    .toBeGreaterThan(100);
  expect(f.errors).toEqual([]);
});

test("switches threads without losing drafts and fills the viewport", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  f.item.state = "completed";
  f.item.latest_attempt_id = "attempt_existing";
  const second = { ...f.item, id: "work_two" };
  await page.route("**/ui-api/work-items?project_id=A", (route) =>
    route.fulfill({ json: { items: [f.item, second] } }),
  );
  await page.route("**/ui-api/work-items/work_two?project_id=A", (route) =>
    route.fulfill({ json: second }),
  );
  await page.route(
    "**/ui-api/work-items/*/instruction?project_id=A",
    (route) => {
      const item = route.request().url().includes("work_two") ? second : f.item;
      return route.fulfill({
        json: {
          project_id: "A",
          work_item_id: item.id,
          instruction:
            (item.id === second.id ? "Fix login" : "Build gallery") +
            "\n" +
            "Request detail\n".repeat(100),
        },
      });
    },
  );
  await page.setViewportSize({ width: 1600, height: 1200 });
  await page.goto("/app/projects/A/work?work=work_one");
  const rail = page.locator("#thread-sidebar");
  const input = page.getByLabel("Message to your agent");
  await expect(
    rail.getByRole("button", { name: "Build gallery" }),
  ).toHaveAttribute("aria-pressed", "true");
  await input.fill("Gallery draft stays here");
  await rail.getByRole("button", { name: "Fix login" }).click();
  await expect(
    page.getByRole("heading", { name: "Fix login", exact: true }),
  ).toBeVisible();
  await expect(input).toHaveValue("");
  await input.fill("Login draft stays here");
  await rail.getByRole("button", { name: "Build gallery" }).click();
  await expect(input).toHaveValue("Gallery draft stays here");
  await expect(rail).toBeVisible();
  for (const viewport of [
    { width: 1600, height: 1200 },
    { width: 1280, height: 900 },
    { width: 1024, height: 900 },
    { width: 390, height: 844 },
    { width: 320, height: 568 },
  ]) {
    await page.setViewportSize(viewport);
    await expect
      .poll(async () => {
        const bounds = await page.locator(".visual-session").boundingBox();
        return Math.abs(viewport.height - bounds!.y - bounds!.height);
      })
      .toBeLessThanOrEqual(2);
    await expect(input).toBeInViewport();
    if (viewport.width < 1600)
      await expect(
        page.getByRole("button", { name: "Switch thread", exact: true }),
      ).toBeVisible();
    expect(
      await page.evaluate(
        () =>
          document.documentElement.scrollHeight <= innerHeight &&
          document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    const feed = page.locator(".conversation-feed");
    await feed.evaluate(
      (element) => (element.scrollTop = element.scrollHeight),
    );
    await expect(input).toBeInViewport();
    await page.screenshot({
      path: `artifacts/thread-switcher-${viewport.width}.png`,
    });
  }
  await page
    .getByRole("button", { name: "Switch thread", exact: true })
    .click();
  const drawer = page.getByRole("dialog", { name: "Threads", exact: true });
  await drawer.getByRole("textbox", { name: "Search threads" }).fill("login");
  await drawer.getByRole("button", { name: "Fix login" }).click();
  await expect(drawer).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Switch thread", exact: true }),
  ).toBeFocused();
  await expect(input).toHaveValue("Login draft stays here");
  expect(f.errors).toEqual([]);
});

test("work navigation keeps controls local and presents a readable conversation", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  const titles = [
    "Generate a preview app landing page",
    "Fix sign-in on mobile",
    "Add a comparison experiment",
    "Explain how the service connects",
    "Improve the gallery layout",
    "Check the release configuration",
  ];
  f.state.request =
    "Generate a preview app landing page with an editable hero, a mobile preview, and an early access form.";
  f.state.reply =
    "Created the landing page and started it in **Web preview**.\n\nThe page includes responsive layouts, an editable hero, and an early access form that saves locally.\n\nPreview is running and ready to review.";
  f.attempts.push({
    id: "attempt_existing",
    work_item_id: f.item.id,
    workspace_id: "ws_one",
    task_id: "task_design",
    state: "awaiting_review",
    created_at: f.item.created_at,
    updated_at: f.item.updated_at,
    artifacts: [],
  });
  const items = titles.map((title, index) => ({
    ...f.item,
    id: index ? `work_${index}` : f.item.id,
    state: index < 2 ? "awaiting_review" : "completed",
    latest_attempt_id: "attempt_existing",
    specification_path: `work-requests/${index}.md`,
  }));
  await page.route("**/ui-api/work-items?project_id=A", (route) =>
    route.fulfill({ json: { items } }),
  );
  await page.route("**/ui-api/work-items/work_one?project_id=A", (route) =>
    route.fulfill({ json: items[0] }),
  );
  await page.route(
    "**/ui-api/work-items/*/instruction?project_id=A",
    (route) => {
      const item = items.find((item) =>
        route.request().url().includes(`/${item.id}/`),
      )!;
      return route.fulfill({
        json: {
          project_id: "A",
          work_item_id: item.id,
          instruction:
            titles[items.indexOf(item)] +
            "\n\nCreate a responsive landing page with an editable hero, a mobile preview, and an early access form. Keep it simple and easy to navigate.",
        },
      });
    },
  );
  await page.setViewportSize({ width: 1600, height: 1000 });
  await page.goto("/app/projects/A/work?work=work_one");
  await expect(
    page.getByRole("button", { name: "Collapse preview", exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("button", { name: "Expand sidebar", exact: true })
    .click();
  const rail = page.locator(".conversation-thread-rail");
  await expect(
    page.getByRole("button", { name: "Hide thread panel" }),
  ).toBeVisible();
  await expect(rail).toBeVisible();
  await expect(
    page.getByRole("button", { name: "New thread", exact: true }),
  ).toHaveCount(1);
  // Review the whole workspace in both project-sidebar states. The header,
  // threads and conversation must meet at shared edges, without card gutters.
  for (const collapsed of [false, true]) {
    if (collapsed)
      await page
        .getByRole("button", { name: "Collapse sidebar", exact: true })
        .click();
    await expect(page.locator(".factory-body")).toHaveCSS(
      "padding-left",
      collapsed ? "64px" : "228px",
    );
    const shell = (await page.locator(".factory-sidebar").boundingBox())!;
    const header = (await page.locator(".factory-header").boundingBox())!;
    const tabs = (await page
      .getByRole("navigation", { name: "Project navigation" })
      .boundingBox())!;
    const threads = (await rail.boundingBox())!;
    const session = (await page.locator(".visual-session").boundingBox())!;
    await expect(
      rail.getByRole("heading", { name: "Threads", exact: true }),
    ).toHaveCount(0);
    expect(header.height).toBe(54);
    expect(tabs.x).toBeGreaterThanOrEqual(shell.x);
    expect(tabs.x + tabs.width).toBeLessThanOrEqual(shell.x + shell.width);
    expect(Math.abs(shell.x + shell.width - threads.x)).toBeLessThan(1);
    expect(Math.abs(header.y + header.height - threads.y)).toBeLessThan(1);
    expect(Math.abs(threads.x + threads.width - session.x)).toBeLessThan(1);
    expect(session.y).toBe(threads.y);
    await expect(page.locator(".visual-session")).toHaveCSS(
      "border-radius",
      "0px",
    );
    await page.screenshot({
      path: `artifacts/work-shell-${collapsed ? "compact" : "expanded"}.png`,
      animations: "disabled",
    });
  }
  await page
    .getByRole("button", { name: "Expand sidebar", exact: true })
    .click();
  await expect(page.locator(".factory-body")).toHaveCSS(
    "padding-left",
    "228px",
  );
  await page.setViewportSize({ width: 1280, height: 900 });
  await expect(
    page.getByRole("link", { name: "Settings", exact: true }),
  ).toBeInViewport({ ratio: 1 });
  await page.screenshot({
    path: "artifacts/work-shell-1280.png",
    animations: "disabled",
  });
  await page.setViewportSize({ width: 1600, height: 1000 });
  await expect(
    page.getByRole("heading", { name: titles[0], exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Preview is running and ready to review.", { exact: true }),
  ).toBeVisible();
  const message = (await page
    .locator(".conversation-message.is-user")
    .boundingBox())!;
  const composer = (await page
    .locator(".conversation-composer form")
    .boundingBox())!;
  expect(message.width).toBeLessThanOrEqual(704);
  expect(composer.width).toBeGreaterThan(message.width);
  expect(composer.width).toBeLessThanOrEqual(1184);
  const footer = page.locator(".thread-footer");
  const composerPanel = footer.locator(".conversation-compose-input");
  const threadHeader = page.locator(".conversation-thread-heading");
  const viewSwitch = threadHeader.getByRole("group", { name: "Thread view" });
  const settings = threadHeader.getByRole("button", { name: "Task settings" });
  const expectHeaderControls = async () => {
    const view = (await viewSwitch.boundingBox())!;
    const settingsBox = (await settings.boundingBox())!;
    expect(view.x + view.width).toBeLessThanOrEqual(settingsBox.x);
    expect(view.y + view.height / 2).toBeCloseTo(
      settingsBox.y + settingsBox.height / 2,
      0,
    );
    await expect(
      footer.getByRole("group", { name: "Thread view" }),
    ).toHaveCount(0);
  };
  await expectHeaderControls();
  await expect(page.locator(".conversation-composer textarea")).toHaveCSS(
    "min-height",
    "42px",
  );
  expect(
    Math.abs(
      (await page.locator(".conversation-activity").boundingBox())!.x -
        message.x,
    ),
  ).toBeLessThan(2);
  await page.screenshot({ path: "artifacts/work-design-expanded.png" });

  await page.setViewportSize({ width: 2160, height: 1000 });
  await expectHeaderControls();
  const widePanel = (await composerPanel.boundingBox())!;
  const wideFooter = (await footer.boundingBox())!;
  expect(widePanel.x - wideFooter.x).toBeCloseTo(
    wideFooter.x + wideFooter.width - widePanel.x - widePanel.width,
    0,
  );

  await page.setViewportSize({ width: 390, height: 844 });
  await expectHeaderControls();
  const switcher = page.getByRole("button", {
    name: "Switch thread",
    exact: true,
  });
  await expect(switcher).toBeVisible();
  expect((await switcher.boundingBox())!.x).toBeLessThan(70);
  await page.screenshot({ path: "artifacts/work-design-mobile.png" });
  await switcher.click();
  await expect(page.getByRole("dialog", { name: "Threads" })).toBeVisible();
  await expect
    .poll(
      async () =>
        (await page.getByRole("dialog", { name: "Threads" }).boundingBox())!.x,
    )
    .toBeGreaterThanOrEqual(0);
  await page.screenshot({
    path: "artifacts/work-design-drawer.png",
    animations: "disabled",
  });
  await page.keyboard.press("Escape");
  await expect(switcher).toBeFocused();
  await conversationTool(page, "Preview");
  await expect(switcher).toBeVisible();
  await switcher.click();
  await expect(page.getByRole("dialog", { name: "Threads" })).toBeVisible();
  await page.keyboard.press("Escape");
  await page.setViewportSize({ width: 1600, height: 1000 });
  await expect(rail).toBeVisible();
  await page
    .getByRole("button", { name: "Switch to light theme", exact: true })
    .click();
  await page.screenshot({
    path: "artifacts/work-design-light.png",
    animations: "disabled",
  });
  expect(f.errors).toEqual([]);
});

test("new thread explains output-file limits before creation", async ({
  page,
}) => {
  const fixture = await conversationFixture(page);
  await page.goto("/app/projects/A/work");
  const start = page.getByRole("button", {
    name: "Create thread",
    exact: true,
  });
  await page
    .getByLabel("Model", { exact: true })
    .selectOption({ label: "worker-model · Codex" });
  await page.getByText("Keep specific output files", { exact: true }).click();
  const outputs = page.getByRole("textbox", {
    name: "Retained output paths",
    exact: true,
  });
  await outputs.fill(
    Array.from({ length: 33 }, (_, index) => `outputs/report-${index}.md`).join(
      "\n",
    ),
  );
  await expect(outputs).toHaveAttribute("aria-invalid", "true");
  await expect(page.getByRole("alert")).toContainText("at most 32");
  await expect(start).toBeDisabled();
  await outputs.fill("outputs/report.md");
  await expect(page.getByRole("alert")).toHaveCount(0);
  await page.getByLabel("Work profile").selectOption("data-science");
  await page
    .getByLabel("Model", { exact: true })
    .selectOption({ label: "worker-model · Codex" });
  await expect(start).toBeEnabled();
  expect(fixture.posts).toEqual([]);
});

test("creates an empty thread after model selection and waits for the first message", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.item.kind = "software";
  await page.goto("/app/projects/A/work");
  await expect(page.getByLabel("Message to your agent")).toHaveCount(0);
  await expect(page.getByLabel("Work profile").locator("option")).toHaveText([
    "Web Development",
    "Desktop App Development",
    "Research / Data Science",
  ]);
  await expect(
    page.getByRole("button", { name: "Create work item", exact: true }),
  ).toHaveCount(0);
  await expect(page.getByLabel("Model ID", { exact: true })).toHaveCount(0);
  const start = page.getByRole("button", {
    name: "Create thread",
    exact: true,
  });
  await expect(start).toBeDisabled();
  await page
    .getByLabel("Model", { exact: true })
    .selectOption({ label: "chosen-model · Claude" });
  await start.click();
  await expect(page).toHaveURL(/work=work_one/);
  expect(
    f.posts.find((p) => p.path === "/ui-api/work-items")?.body,
  ).toMatchObject({
    empty_thread: true,
    kind: "software",
    provider: "claude",
    model: "chosen-model",
  });
  expect(f.posts).toHaveLength(1);
  expect(f.posts[0].body).not.toHaveProperty("instruction");
  await page.reload();
  await page.getByLabel("Message to your agent").fill("Build a landing page.");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Stop task", exact: true }),
  ).toBeVisible();
  expect(f.posts.find((p) => p.path.endsWith("/continue"))?.body).toMatchObject(
    {
      instruction: "Build a landing page.",
      expected_attempt_id: "",
    },
  );
});

test("retrying the worker list does not create a thread", async ({ page }) => {
  const fixture = await conversationFixture(page);
  let offline = true;
  await page.route("**/ui-api/workspaces?**", (route) =>
    offline
      ? route.fulfill({ status: 503, json: { code: "STORAGE_UNAVAILABLE" } })
      : route.fulfill({ json: { items: [] } }),
  );
  await page.goto("/app/projects/A/work");
  await expect(page.getByRole("alert")).toBeVisible();
  offline = false;
  await page
    .getByRole("alert")
    .getByRole("button", { name: "Try again", exact: true })
    .click();
  await expect(page.getByRole("alert")).toHaveCount(0);
  expect(fixture.posts).toEqual([]);
  const start = page.getByRole("button", {
    name: "Create thread",
    exact: true,
  });
  await expect(start).toBeDisabled();
  await page
    .getByLabel("Model", { exact: true })
    .selectOption("worker-default");
  await start.click();
  await expect(page).toHaveURL(/work=work_one/);
  const body = fixture.posts.find((p) => p.path === "/ui-api/work-items")?.body;
  expect(body).toMatchObject({ workspace_id: "", kind: "software" });
  expect(body).not.toHaveProperty("model");
});

test("worker catalog supplies models and restores the worker default", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  await page.goto("/app/projects/A/work");
  const model = page.getByLabel("Model", { exact: true });
  await expect(model).toBeEnabled();
  await expect(model.locator("option")).toHaveText([
    "Choose a model",
    "worker-model · Codex",
    "chosen-model · Claude",
  ]);
  await model.selectOption(JSON.stringify(["claude", "chosen-model"]));
  await model.selectOption({ label: "worker-model · Codex" });
  await page.getByLabel("Reasoning effort").selectOption("high");
  await page.getByLabel("Work profile").selectOption("software");
  await page
    .getByRole("button", { name: "Create thread", exact: true })
    .click();
  await expect(page).toHaveURL(/work=work_one/);
  expect(
    f.posts.find((p) => p.path === "/ui-api/work-items")?.body,
  ).toMatchObject({
    workspace_id: "ws_one",
    provider: "codex",
    model: "worker-model",
    reasoning_effort: "high",
  });
});

test("a follow-up switches model on the same thread and worker", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  await page.goto("/app/projects/A/work?work=work_one");
  await page.getByRole("button", { name: "Start task", exact: true }).click();
  f.finish();
  await expect(
    page.getByRole("button", { name: /^Choose model:/ }),
  ).toBeEnabled({ timeout: 10000 });
  await page.getByRole("button", { name: /^Choose model:/ }).click();
  const model = page.getByLabel("Model", { exact: true });
  await expect(model).toBeEnabled({ timeout: 10000 });
  await model.selectOption(JSON.stringify(["claude", "chosen-model"]));
  await page
    .getByLabel("Message to your agent", { exact: true })
    .fill("Continue from the report using this model.");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect
    .poll(() => f.posts.filter((p) => p.path.endsWith("/continue")).length)
    .toBe(1);
  expect(f.posts.find((p) => p.path.endsWith("/continue"))?.body).toMatchObject(
    {
      project_id: "A",
      expected_attempt_id: "attempt_1",
      instruction: "Continue from the report using this model.",
      selection: { provider: "claude", model: "chosen-model" },
    },
  );
  expect(f.posts.filter((p) => p.path === "/ui-api/work-items")).toHaveLength(
    0,
  );
  await expect(page).toHaveURL(/work=work_one/);
  expect(f.errors).toEqual([]);
});

test("catalog failure offers a retry without creating a thread", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  let unavailable = true;
  await page.route("**/ui-api/workspaces/ws_one/models", (route) =>
    route.fulfill(
      unavailable
        ? { status: 503, json: { code: "MODEL_CATALOG_UNAVAILABLE" } }
        : {
            json: {
              workspace_id: "ws_one",
              items: [{ provider: "codex", model: "fresh-model" }],
            },
          },
    ),
  );
  await page.goto("/app/projects/A/work");
  const retry = page.getByRole("button", { name: "Retry loading models" });
  await expect(retry).toBeVisible();
  const model = page.getByLabel("Model", { exact: true });
  await expect(model).toBeDisabled();
  unavailable = false;
  await retry.click();
  await expect(model).toBeEnabled();
  await expect(
    model.getByRole("option", { name: "fresh-model · Codex" }),
  ).toHaveCount(1);
  expect(f.posts).toEqual([]);
});

test("retry is inside the task error and icon controls have accessible labels", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  await page.goto("/app/projects/A/work?work=work_one");
  await page.getByRole("button", { name: "Start task", exact: true }).click();
  f.item.state = "failed";
  f.attempts[0].state = "failed";
  f.attempts[0].error_code = "TASK_FAILED";
  await page.reload();
  const retry = page
    .getByRole("alert")
    .getByRole("button", { name: "Retry task", exact: true });
  await expect(retry).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Retry task", exact: true }),
  ).toHaveCount(1);
  await expect(
    page.getByRole("button", { name: "Send", exact: true }),
  ).toHaveText("");
  await expect(
    page.getByRole("button", { name: "Task settings", exact: true }),
  ).toHaveText("");
  await expect(retry).toHaveAttribute("data-variant", "outline");
  await expect(retry).toHaveCSS("border-top-width", "1px");
  for (const width of [1600, 390]) {
    await page.setViewportSize({ width, height: 1000 });
    const collapse = page.getByRole("button", {
      name: "Collapse preview",
      exact: true,
    });
    if (await collapse.isVisible()) await collapse.click();
    if (width === 1600) {
      const rail = (await page
        .locator(".conversation-thread-rail")
        .boundingBox())!;
      const search = (await page
        .getByRole("textbox", { name: "Search threads", exact: true })
        .boundingBox())!;
      expect(search.y - rail.y).toBeGreaterThanOrEqual(16);
      expect(search.x - rail.x).toBeGreaterThanOrEqual(12);
    }
    const input = page.getByLabel("Message to your agent");
    for (const draft of [
      "",
      "First line\nSecond line\nThird line\nFourth line",
    ]) {
      await input.fill(draft);
      const field = (await input.boundingBox())!;
      const send = (await page
        .getByRole("button", { name: "Send", exact: true })
        .boundingBox())!;
      const box = (await page
        .locator(".conversation-compose-input")
        .boundingBox())!;
      if (width === 1600) {
        expect(send.x).toBeGreaterThanOrEqual(field.x + field.width);
      } else {
        expect(send.y).toBeGreaterThanOrEqual(field.y + field.height);
      }
      expect(send.x).toBeGreaterThanOrEqual(box.x);
      expect(send.x + send.width).toBeLessThanOrEqual(box.x + box.width);
      expect(send.y + send.height).toBeLessThanOrEqual(box.y + box.height);
    }
    await input.fill("");
    await page.screenshot({
      path: `artifacts/thread-failure-polish-${width}.png`,
    });
  }
  await retry.click();
  await expect(
    page.getByRole("button", { name: "Stop task", exact: true }),
  ).toBeVisible();
});

test("Files shows one retry when both lists fail", async ({ page }) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  f.addRun();
  f.finish();
  let unavailable = true;
  for (const [path, response] of [
    ["files", { paths: ["README.md"] }],
    ["attempts/*/changes", { files: [{ path: "app.js", status: "M" }] }],
  ] as const) {
    await page.route(
      `**/ui-api/work-items/work_one/${path}?project_id=A`,
      (route) =>
        unavailable
          ? route.fulfill({
              status: 404,
              json: { code: "NOT_FOUND_OR_FORBIDDEN" },
            })
          : route.fulfill({ json: response }),
    );
  }
  await page.goto("/app/projects/A/work?work=work_one&view=files");
  const explorer = page.getByRole("complementary", { name: "File explorer" });
  await expect(explorer.getByRole("alert")).toHaveText(
    /Could not load changed files or workspace files/,
  );
  await expect(explorer.getByRole("button", { name: "Try again" })).toHaveCount(
    1,
  );
  unavailable = false;
  await explorer.getByRole("button", { name: "Try again" }).click();
  await expect(
    explorer.getByRole("button", { name: "README.md" }),
  ).toBeVisible();
  await expect(explorer.getByRole("button", { name: /app.js/ })).toBeVisible();
  await expect(explorer.getByRole("alert")).toHaveCount(0);
});

test("Files keeps workspace contents and the last diff after a no-op follow-up", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  const first = f.addRun();
  f.finish();
  f.attempts[0].artifacts = [];
  await page.route(
    "**/ui-api/work-items/work_one/files?project_id=A",
    (route) =>
      route.fulfill({
        json:
          route.request().method() === "GET"
            ? { paths: ["app/index.html", "README.md"] }
            : {
                path: "app/index.html",
                content: Buffer.from("<h1>Live application</h1>\n").toString(
                  "base64",
                ),
              },
      }),
  );
  await page.route(
    "**/ui-api/work-items/work_one/attempts/*/changes?project_id=A",
    (route) =>
      route.fulfill({
        json: route.request().url().includes(first.id)
          ? {
              files: [{ path: "app/index.html", status: "A" }],
              diff: "diff --git a/app/index.html b/app/index.html\n@@ -0,0 +1 @@\n+<h1>Recorded application</h1>\n",
            }
          : { files: [] },
      }),
  );
  await page.goto("/app/projects/A/work?work=work_one&view=files");
  const files = page.getByRole("region", { name: "Task files" });
  const workspace = files.getByRole("region", { name: "Workspace files" });
  await workspace.getByRole("button", { name: "app/index.html" }).click();
  await expect(
    files.getByRole("region", { name: "File contents" }),
  ).toContainText("Live application");
  await files.getByRole("button", { name: "Diff", exact: true }).click();
  await expect(files.getByLabel("Git diff")).toContainText(
    "Recorded application",
  );
  const second = f.addRun(first.id);
  f.finish();
  f.attempts[1].artifacts = [];
  const run = page.getByRole("combobox", { name: "File changes by run" });
  await expect(run).toHaveValue("");
  await expect(files.getByText("Changes from run 1.")).toBeVisible();
  await expect(
    workspace.getByRole("button", { name: "README.md" }),
  ).toBeVisible();
  await expect(files.getByLabel("Git diff")).toContainText(
    "Recorded application",
  );
  await run.selectOption(second.id);
  await expect(
    files.getByText("No project files changed in this run."),
  ).toBeVisible();
  await expect(
    workspace.getByRole("button", { name: "app/index.html" }),
  ).toHaveAttribute("aria-current", "true");
  await expect(
    files.getByRole("region", { name: "File contents" }),
  ).toContainText("Live application");
  await run.selectOption("");
  await expect(
    files.getByRole("button", { name: "app/index.html, Added" }),
  ).toBeVisible();
  expect(new URL(page.url()).searchParams.has("attempt")).toBe(false);
  await page.reload();
  await expect(run).toHaveValue("");
  await expect(files.getByText("Changes from run 1.")).toBeVisible();
});

test("Files reviews diffs and sends line-specific feedback from the workspace", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  f.state.reply = "Saved [measurements](outputs/measurements.csv).";
  await page.route(
    "**/ui-api/work-items/work_one/attempts/attempt_1/changes?project_id=A",
    (route) =>
      route.fulfill({
        json: {
          files: [
            { path: "outputs/measurements.csv", status: "A" },
            {
              path: "src/score model.ts",
              status: "M",
              source: Buffer.from(
                "// The complete file starts here.\n" +
                  "\n".repeat(6) +
                  "const score = 72;\nconst scale = 100;\n",
              ).toString("base64"),
            },
          ],
          diff: [
            "diff --git a/outputs/measurements.csv b/outputs/measurements.csv",
            "@@ -0,0 +1 @@",
            "+experiment,score",
            "diff --git a/src/score model.ts b/src/score model.ts",
            "@@ -8 +8,2 @@",
            "-const score = 0;",
            "+const score = 72;",
            "+const scale = 100;",
            "@@ -30 +31 @@",
            "-total = count;",
            "+total = score * count;",
            "",
          ].join("\n"),
        },
      }),
  );
  await page.route("**/ui-api/work-items/work_one/data", (route) =>
    route.fulfill({
      json: {
        path: "outputs/measurements.csv",
        content: Buffer.from(
          "experiment,score\nbaseline,72\nrevision,86",
        ).toString("base64"),
      },
    }),
  );
  await page.goto("/app/projects/A/work?work=work_one");
  await page.getByRole("button", { name: "Start task", exact: true }).click();
  f.finish();
  f.attempts[0].artifacts = [];
  await page.reload();
  await expect(page.getByText("measurements", { exact: true })).toBeVisible();
  await conversationTool(page, "Files");
  const files = page.getByRole("region", { name: "Task files" });
  const changed = files.getByRole("region", { name: "Changed files" });
  const diff = files.getByLabel("Git diff");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(
    changed.getByRole("button", { name: "outputs/measurements.csv, Added" }),
  ).toHaveAttribute("aria-current", "true");
  const source = files.getByRole("region", { name: "File contents" });
  await expect(source).toContainText("baseline,72");
  await expect(diff).toHaveCount(0);
  await expect(
    files.getByRole("link", { name: "Download", exact: true }),
  ).toBeVisible();
  await files.getByRole("button", { name: "Diff", exact: true }).click();
  await expect(diff).toContainText("experiment,score");
  await expect(diff).not.toContainText("const score = 72;");

  await files
    .getByRole("searchbox", { name: "Filter files" })
    .fill("score model");
  await expect(
    changed.getByRole("button", { name: "src/score model.ts, Modified" }),
  ).toBeVisible();
  await expect(source).toContainText("// The complete file starts here.");
  await expect(source).toContainText("const score = 72;");
  await expect(source.locator("diffs-container [data-line='8']")).toContainText(
    "const score = 72;",
  );
  await expect(
    source.locator("diffs-container [data-line='8'] span[style]").first(),
  ).toBeVisible();
  await expect(diff).toHaveCount(0);
  await files.getByRole("button", { name: "Diff", exact: true }).click();
  await expect(diff).toContainText("const score = 72;");
  await expect(diff).not.toContainText("experiment,score");
  await expect(diff).not.toContainText("// The complete file starts here.");
  await files.getByRole("button", { name: "Next hunk" }).click();
  await files.getByRole("button", { name: "Next hunk" }).click();
  await expect(files.getByText("2 / 2", { exact: true })).toBeVisible();
  await files.getByText("Select lines", { exact: true }).click();
  await files.getByRole("spinbutton", { name: "First line" }).fill("8");
  await files.getByRole("spinbutton", { name: "Last line" }).fill("9");
  await files.getByRole("button", { name: "Explain selected lines" }).click();
  const draft = page.getByLabel("Message to your agent");
  const suggestion = 'Explain "src/score model.ts:8-9" (revision attempt_1): ';
  await expect(draft).toBeFocused();
  await expect(draft).toHaveValue(suggestion);
  expect(
    f.posts.filter((post) => post.path.endsWith("/continue")),
  ).toHaveLength(0);
  await draft.fill(suggestion + "Use the configured threshold.");
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(files).toBeInViewport();
  await expect(draft).toBeInViewport();
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth),
  ).toBeLessThanOrEqual(390);
  await page.getByRole("button", { name: "Preview", exact: true }).click();
  await expect(
    page
      .getByRole("region", { name: "Agent output" })
      .getByRole("region", { name: "measurements" })
      .getByRole("img", { name: "score by experiment", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect
    .poll(() => f.posts.find((post) => post.path.endsWith("/continue"))?.body)
    .toMatchObject({
      expected_attempt_id: "attempt_1",
      instruction: suggestion + "Use the configured threshold.",
    });
  await page
    .getByRole("button", { name: "Task settings", exact: true })
    .click();
  await page
    .getByRole("link", { name: "Worker: Research worker", exact: true })
    .click();
  await expect(page).toHaveURL(/workspaces\/ws_one$/);
  await expect(
    page.getByRole("heading", { name: "Research worker", exact: true }),
  ).toBeVisible();
});

test("expired previews can restart their saved application directly", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  f.apps.push({
    id: "previous",
    state: "expired",
    ready: false,
    application: { directory: "site", command: "npm run dev", port: 3000 },
  });
  await page.goto("/app/projects/A/work?work=work_one");
  await conversationTool(page, "Preview");
  await page
    .getByRole("button", { name: "Restart preview", exact: true })
    .click();
  expect(
    f.posts.find((p) => p.path.endsWith("/applications"))?.body.application,
  ).toEqual({ directory: "site", command: "npm run dev", port: 3000 });
});

test("thread direction stays clear while settings and model controls preserve the draft", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/app/projects/A/work?work=work_one");
  const heading = page.locator(".conversation-thread-heading");
  await expect(heading).not.toContainText("Owner:");
  await expect(heading).not.toContainText("Worker:");
  await expect(page.getByLabel("Model", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "Start task", exact: true }).click();
  const stop = page.getByRole("button", { name: "Stop task", exact: true });
  await expect(stop).toHaveText("Stop");
  const composer = page.getByLabel("Message to your agent", { exact: true });
  await composer.fill("Keep this direction while I inspect settings.");
  await page
    .getByRole("button", { name: "Task settings", exact: true })
    .click();
  const settings = page.getByRole("dialog", {
    name: "Task settings",
    exact: true,
  });
  await expect(
    settings.getByLabel("Agent controller", { exact: true }),
  ).toBeDisabled();
  await expect(
    settings.getByText(
      "Stop the current run before changing control or the reviewer.",
    ),
  ).toBeVisible();
  expect(
    (await new AxeBuilder({ page }).include(".thread-settings-sheet").analyze())
      .violations,
  ).toEqual([]);
  await page.screenshot({
    path: "artifacts/thread-settings.png",
    animations: "disabled",
  });
  await page.keyboard.press("Escape");
  await expect(settings).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Task settings", exact: true }),
  ).toBeFocused();
  await expect(composer).toHaveValue(
    "Keep this direction while I inspect settings.",
  );
  await page.screenshot({
    path: "artifacts/thread-direction-running.png",
    fullPage: true,
    animations: "disabled",
  });
  await page.getByRole("button", { name: "Workspace", exact: true }).click();
  await expect(composer).toHaveValue(
    "Keep this direction while I inspect settings.",
  );
  const titleBox = (await heading.locator("h2").boundingBox())!;
  const actionsBox = (await heading
    .getByRole("button", { name: "Task settings", exact: true })
    .boundingBox())!;
  expect(
    titleBox.x + titleBox.width <= actionsBox.x ||
      titleBox.y + titleBox.height <= actionsBox.y,
  ).toBe(true);
  await page.screenshot({
    path: "artifacts/thread-direction-split.png",
    animations: "disabled",
  });
  await page.getByRole("button", { name: "Conversation", exact: true }).click();
  expect(
    (await new AxeBuilder({ page }).include(".conversation-session").analyze())
      .violations,
  ).toEqual([]);
  await stop.click();
  await expect(
    page.getByRole("button", { name: "Send", exact: true }),
  ).toBeVisible();
  await expect(composer).toHaveValue(
    "Keep this direction while I inspect settings.",
  );
  await page.getByRole("button", { name: /^Choose model:/ }).click();
  await expect(page.getByLabel("Model", { exact: true })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(composer).toHaveValue(
    "Keep this direction while I inspect settings.",
  );
  for (const width of [1440, 1024, 390]) {
    await page.setViewportSize({ width, height: 1000 });
    const send = await page
      .getByRole("button", { name: "Send", exact: true })
      .boundingBox();
    expect(send).not.toBeNull();
    expect(send!.x).toBeGreaterThanOrEqual(0);
    expect(send!.x + send!.width).toBeLessThanOrEqual(width);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
  }
  await page.screenshot({
    path: "artifacts/thread-direction-mobile.png",
    fullPage: true,
    animations: "disabled",
  });
  expect(f.posts.filter((p) => p.path.endsWith("/cancel"))).toHaveLength(1);
});

test("workspace marks new replies on the Conversation button without interrupting the draft", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/app/projects/A/work?work=work_one");
  await page.getByRole("button", { name: "Start task", exact: true }).click();
  await expect(page.getByText("1 tool action", { exact: true })).toBeVisible();
  const composer = page.getByLabel("Message to your agent", { exact: true });
  await composer.fill("Make the welcome page easier to scan.");
  await page.getByRole("button", { name: "Workspace", exact: true }).click();
  const workspace = page.getByRole("region", {
    name: "Task workspace",
    exact: true,
  });
  await expect(workspace).toBeVisible();
  await expect(
    page.getByRole("region", { name: "Conversation and agent activity" }),
  ).toBeHidden();
  await expect(composer).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Stop task", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Open preview", exact: true }),
  ).toHaveCount(0);
  const threadView = page.getByRole("group", { name: "Thread view" });
  const unread = threadView.getByRole("status");
  await expect(unread).toBeEmpty();
  await composer.focus();
  f.finish();
  await expect(unread).toHaveText("New agent message", { timeout: 10000 });
  await expect(
    page.getByRole("button", { name: "View message", exact: true }),
  ).toHaveCount(0);
  await expect(
    workspace.getByRole("tab", { name: "Output", exact: true }),
  ).toHaveCount(0);
  await expect(workspace.getByText(f.state.reply, { exact: true })).toHaveCount(
    0,
  );
  await expect(composer).toBeFocused();
  await expect(composer).toHaveValue("Make the welcome page easier to scan.");
  await expect(workspace).toBeVisible();
  expect(f.posts.filter((p) => p.path.endsWith("/continue"))).toHaveLength(0);
  for (const width of [1440, 1024, 390]) {
    await page.setViewportSize({ width, height: 900 });
    await expect(composer).toBeVisible();
    const composeBox = (await page
      .locator(".conversation-composer")
      .boundingBox())!;
    const outputBox = (await workspace.boundingBox())!;
    expect(outputBox.y + outputBox.height).toBeLessThanOrEqual(composeBox.y);
    expect(composeBox.y + composeBox.height).toBeLessThanOrEqual(901);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: `artifacts/unified-workspace-${width}.png`,
      animations: "disabled",
    });
  }
  await expect(
    page.getByRole("button", { name: "Send", exact: true }),
  ).toBeEnabled();
  expect(
    (await new AxeBuilder({ page }).include(".conversation-session").analyze())
      .violations,
  ).toEqual([]);
  await threadView
    .getByRole("button", { name: "Conversation", exact: true })
    .click();
  await expect(page.getByText(f.state.reply, { exact: true })).toBeVisible();
  await expect(unread).toBeEmpty();
  await expect(composer).toHaveValue("Make the welcome page easier to scan.");
  await page.getByRole("button", { name: "Workspace", exact: true }).click();
  await expect(unread).toBeEmpty();
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Stop task", exact: true }),
  ).toBeVisible();
  await expect(workspace).toBeVisible();
  expect(f.posts.filter((p) => p.path.endsWith("/continue"))).toHaveLength(1);
  await page.getByRole("button", { name: "Stop task", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Retry task", exact: true }),
  ).toBeVisible();
  await expect(workspace).toBeVisible();
  await page.reload();
  await expect(workspace).toBeVisible();
  await expect(unread).toBeEmpty();
  expect(f.errors).toEqual([]);
});

test("Workspace and Conversation keep the same layout across viewports", async ({
  page,
  context,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  f.state.alternateWorker = true;
  f.item.kind = "software";
  f.addRun();
  f.finish();
  f.apps.push({
    id: "service_live",
    state: "active",
    ready: true,
    application: { port: 4173 },
  });
  await context.route(
    "http://preview-0123456789abcdef0123456789abcdef.localhost:4173/**",
    (route) =>
      route.fulfill({ contentType: "text/html", body: "Live preview" }),
  );

  const laptop = { width: 1366, height: 650 };
  await page.setViewportSize(laptop);
  await page.goto("/app/projects/A/work?work=work_one");
  const rail = page.locator("#thread-sidebar");
  await expect(rail).toBeHidden();
  await conversationTool(page, "Preview");

  const preview = page.locator('iframe[title="Application preview"]');
  await expect(preview.contentFrame().locator("body")).toHaveText(
    "Live preview",
  );
  const sharedLayout = () =>
    Promise.all(
      [
        ".conversation-thread-heading",
        ".thread-settings-trigger",
        ".thread-footer",
        ".conversation-composer",
        "#work-followup",
      ].map((selector) => page.locator(selector).boundingBox()),
    );
  const conversation = page.getByRole("button", {
    name: "Conversation",
    exact: true,
  });
  for (const viewport of [
    laptop,
    { width: 1856, height: 917 },
    { width: 1600, height: 1000 },
    { width: 390, height: 844 },
  ]) {
    await page.setViewportSize(viewport);
    const docked = viewport.width === 1600;
    await expect(rail).toBeVisible({ visible: docked });
    if (!docked && viewport.width >= laptop.width) {
      const bounds = (await preview.boundingBox())!;
      expect(bounds.width).toBeGreaterThan(viewport.width * 0.9);
      expect(bounds.height).toBeGreaterThan(viewport.height * 0.6);
    }
    const workspaceLayout = await sharedLayout();
    await expect(
      page
        .locator(".conversation-thread-heading")
        .getByRole("button", { name: "Task settings", exact: true }),
    ).toBeVisible();
    await expect(
      page
        .locator(".conversation-thread-heading")
        .getByRole("group", { name: "Thread view" }),
    ).toBeVisible();
    await conversation.click();
    await expect(conversation).toHaveAttribute("aria-pressed", "true");
    await expect(rail).toBeVisible({ visible: docked });
    await expect(
      page.getByRole("group", { name: "Workspace tools" }),
    ).toHaveCount(0);
    expect(await sharedLayout()).toEqual(workspaceLayout);
    await conversationTool(page, "Preview");
  }
  await page.setViewportSize(laptop);

  const composer = page.getByLabel("Message to your agent", { exact: true });
  const worker = page.getByLabel("Worker for next message");
  await expect(composer).toBeInViewport();
  await expect(worker).toHaveCount(0);
  await page.getByRole("button", { name: "Task settings" }).click();
  await expect(
    page
      .getByRole("dialog", { name: "Task settings" })
      .getByLabel("Worker for next message"),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await composer.fill("Keep this draft while looking up another thread.");
  const switchThread = page.getByRole("button", { name: "Switch thread" });
  await switchThread.click();
  await expect(page.getByRole("dialog", { name: "Threads" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(switchThread).toBeFocused();
  await expect(composer).toHaveValue(
    "Keep this draft while looking up another thread.",
  );

  await conversation.click();
  await expect(rail).toBeHidden();
  await expect(switchThread).toBeVisible();
});

test("reads a stopped worker's cached conversation and sends the next message to another worker", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  f.state.alternateWorker = true;
  f.item.latest_attempt_id = "attempt_1";
  f.attempts.push({
    id: "attempt_1",
    task_id: "task_1",
    workspace_id: "ws_one",
    work_item_id: f.item.id,
    created_at: f.item.created_at,
  });
  f.finish();
  await page.goto("/app/projects/A/work?work=work_one");
  await expect(
    page.getByText("All four tests passed. The report is ready."),
  ).toBeVisible();
  await expect(page.getByText(/Cached history as of/)).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Start worker to load history" }),
  ).toHaveCount(0);
  expect(f.posts).toHaveLength(0);
  await page.getByRole("button", { name: "Task settings" }).click();
  const settings = page.getByRole("dialog", { name: "Task settings" });
  await settings.getByLabel("Worker for next message").selectOption("ws_two");
  await page.keyboard.press("Escape");
  await expect(settings).toHaveCount(0);
  await page
    .getByLabel("Message to your agent")
    .fill("Continue on the backup worker.");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect
    .poll(
      () =>
        f.posts.find((post) => post.path.endsWith("/continue"))?.body
          .workspace_id,
    )
    .toBe("ws_two");
  expect(f.posts.some((post) => post.path.endsWith("/start"))).toBe(false);
});

test("uploads worker data and sends only paginated preview requests", async ({
  page,
}) => {
  await conversationFixture(page);
  const dataset = {
    dataset_id: "dataset_sensor",
    name: "sensor.csv",
    format: "csv",
    size_bytes: 20,
    sha256: "c".repeat(64),
  };
  let uploaded = false;
  const previews: Record<string, unknown>[] = [];
  await page.route("**/workspaces/ws_one/analysis/**", async (route) => {
    const req = route.request();
    const url = new URL(req.url());
    expect(url.searchParams.get("project_id")).toBe("A");
    if (url.pathname.endsWith("/upload")) {
      expect(url.searchParams.get("name")).toBe("sensor.csv");
      expect(req.headers()["content-type"]).toBe("application/octet-stream");
      expect(req.postData()).toBe("x,y\n2,3\n1,4");
      uploaded = true;
      return route.fulfill({ status: 201, json: dataset });
    }
    const columns = [
      { name: "x", type: "BIGINT" },
      { name: "y", type: "BIGINT" },
    ];
    if (url.pathname.endsWith("/schema"))
      return route.fulfill({ json: { ...dataset, columns } });
    if (url.pathname.endsWith("/preview")) {
      const query = req.postDataJSON();
      previews.push(query);
      const more = !query.cursor && !query.filters;
      const rows =
        query.filters?.[0]?.op === "is_null"
          ? []
          : query.cursor
            ? [[1, 4]]
            : [[2, 3]];
      return route.fulfill({
        json: {
          columns,
          rows,
          has_more: more,
          next_cursor: more ? "cursor-2" : null,
          truncated: false,
          sampled: false,
        },
      });
    }
    return route.fulfill({ json: { items: uploaded ? [dataset] : [] } });
  });
  await page.goto("/app/projects/A/workspaces/ws_one?panel=datasets");
  await page.getByLabel("CSV or Parquet file").setInputFiles({
    name: "sensor.csv",
    mimeType: "text/csv",
    buffer: Buffer.from("x,y\n2,3\n1,4"),
  });
  await page.getByRole("button", { name: "Upload dataset" }).click();
  await expect(page.getByRole("table")).toContainText("BIGINT");
  await page.getByRole("button", { name: "Next", exact: true }).click();
  await expect(
    page.getByRole("status").filter({ hasText: "Page 2" }),
  ).toBeVisible();
  await expect(page.getByRole("table")).toContainText("4");
  await page.getByRole("button", { name: "x", exact: true }).click();
  await expect(page.getByRole("columnheader", { name: /^x/ })).toHaveAttribute(
    "aria-sort",
    "ascending",
  );
  await page.getByLabel("Filter column").selectOption("x");
  await page.getByLabel("Value", { exact: true }).fill("2");
  await page.getByRole("button", { name: "Apply filter" }).click();
  await expect
    .poll(() => previews.at(-1))
    .toEqual({
      limit: 100,
      sort: [{ field: "x", direction: "asc" }],
      filters: [{ field: "x", op: "contains", value: "2" }],
    });
  await expect(
    page.getByRole("status").filter({ hasText: "Page 1" }),
  ).toBeVisible();

  const previewCount = previews.length;
  await page.getByRole("button", { name: "Columns", exact: true }).click();
  const columnY = page.getByRole("menuitemcheckbox", {
    name: "y",
    exact: true,
  });
  await columnY.click();
  await expect(columnY).toHaveAttribute("aria-checked", "false");
  await expect(
    page.getByRole("menuitemcheckbox", { name: "x", exact: true }),
  ).toBeDisabled();
  await expect(page.getByRole("columnheader", { name: /^y/ })).toHaveCount(0);
  await columnY.press("Escape");
  expect(previews).toHaveLength(previewCount);

  await page.getByLabel("Match", { exact: true }).selectOption("is_null");
  await expect(page.getByLabel("Value", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "Apply filter" }).click();
  await expect(
    page.getByRole("cell", { name: "No matching rows." }),
  ).toBeVisible();
  expect(previews.at(-1)?.filters).toEqual([{ field: "x", op: "is_null" }]);
  await page.getByRole("button", { name: "Clear filter" }).click();
  await page.getByLabel("Rows per page").selectOption("25");
  await expect
    .poll(() => previews.at(-1))
    .toEqual({ limit: 25, sort: [{ field: "x", direction: "asc" }] });
});

test("renders the same worker chart in a reply and a report, then handles missing worker data", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  const fence = "```harbormaster-visualization\nchart_anomaly\n```";
  f.state.reply = "The tail contains anomalous readings.\n\n" + fence;
  f.state.report =
    "# Sensor investigation\n\n```harbormaster-visualization\nws_one/chart_anomaly\n```";
  f.addRun();
  f.finish();
  let workerExists = true;
  await page.route(
    "**/workspaces/ws_one/analysis/artifacts/chart_anomaly**",
    async (route) => {
      const url = new URL(route.request().url());
      expect(url.searchParams.get("project_id")).toBe("A");
      if (!workerExists)
        return route.fulfill({
          status: 404,
          json: { code: "NOT_FOUND_OR_FORBIDDEN" },
        });
      if (url.pathname.endsWith("/data")) {
        expect(route.request().postDataJSON()).toEqual({});
        return route.fulfill({
          json: {
            columns: [
              { name: "bin", type: "BIGINT" },
              { name: "count", type: "BIGINT" },
            ],
            rows: [
              [0, 108],
              [10, 2],
            ],
            next_cursor: null,
            has_more: false,
            truncated: false,
            sampled: false,
          },
        });
      }
      return route.fulfill({
        json: {
          schema_version: "1.0",
          artifact_id: "chart_anomaly",
          type: "chart",
          title: "Sensor anomaly histogram",
          data: { source_type: "query_result", reference: "result_bins" },
          visualization: {
            chart_type: "histogram",
            encoding: {
              x: { field: "bin", type: "quantitative" },
              y: { field: "count", type: "quantitative" },
            },
          },
        },
      });
    },
  );
  await page.goto("/app/projects/A/work?work=work_one");
  const chart = page.getByRole("region", { name: "Sensor anomaly histogram" });
  await expect(chart.locator("svg")).toBeVisible();
  await conversationTool(page, "Files");
  const files = page.getByRole("region", { name: "Task files" });
  await files
    .getByRole("button", { name: "outputs/report.md", exact: true })
    .click();
  await files.getByRole("button", { name: "Rendered", exact: true }).click();
  await expect(
    files
      .getByRole("region", { name: "Sensor anomaly histogram" })
      .locator("svg"),
  ).toBeVisible();
  workerExists = false;
  await page.reload();
  await conversationTool(page, "Files");
  await files
    .getByRole("button", { name: "outputs/report.md", exact: true })
    .click();
  await files.getByRole("button", { name: "Rendered", exact: true }).click();
  await expect(
    files.getByRole("heading", { name: "Sensor investigation" }),
  ).toBeVisible();
  await expect(files.getByText(/^Worker data is unavailable/)).toBeVisible();
  expect(f.errors).toEqual([]);
});

for (const compatibility of [false, true]) {
  test(`draws geographic and Cartesian point artifacts${compatibility ? " with compatibility heatmaps" : ""}`, async ({
    playwright,
    baseURL,
  }) => {
    const browser = await playwright.chromium.launch({
      args: ["--enable-unsafe-swiftshader", "--use-angle=swiftshader"],
    });
    try {
      const page = await browser.newPage({ baseURL });
      if (compatibility) {
        await page.addInitScript(() => {
          const getExtension = WebGL2RenderingContext.prototype.getExtension;
          WebGL2RenderingContext.prototype.getExtension = function (
            name: string,
          ) {
            return name === "EXT_float_blend"
              ? null
              : getExtension.call(this, name);
          };
        });
      }
      const f = await conversationFixture(page);
      f.state.exists = true;
      f.state.reply =
        "Coordinate checks\n\n```harbormaster-visualization\ngeo_map\n```\n\n```harbormaster-visualization\ngeo_xy\n```\n\n```harbormaster-visualization\ngeo_heat\n```";
      f.addRun();
      f.finish();
      await page.route(
        "**/workspaces/ws_one/analysis/artifacts/geo_**",
        async (route) => {
          const url = new URL(route.request().url());
          const heatmap = url.pathname.includes("geo_heat");
          const geographic = heatmap || url.pathname.includes("geo_map");
          if (url.pathname.endsWith("/data"))
            return route.fulfill({
              json: {
                columns: [
                  { name: "x", type: "DOUBLE" },
                  { name: "y", type: "DOUBLE" },
                  { name: "sensor", type: "VARCHAR" },
                ],
                rows: geographic
                  ? [
                      [-0.12, 51.5, "Above Ground"],
                      [-0.115, 51.502, "Ground Plane"],
                    ]
                  : [[451000, 120000, "Sensor A"]],
                next_cursor: null,
                has_more: true,
                truncated: false,
                sampled: true,
              },
            });
          return route.fulfill({
            json: {
              schema_version: "1.0",
              artifact_id: heatmap
                ? "geo_heat"
                : geographic
                  ? "geo_map"
                  : "geo_xy",
              type: "geo",
              title: heatmap
                ? "Observation density"
                : geographic
                  ? "Geographic map"
                  : "Cartesian display",
              data: { source_type: "dataset", reference: "dataset_coords" },
              visualization: {
                geometry: "point",
                ...(heatmap ? { layer: "heatmap", radius_px: 48 } : {}),
                coordinate_system: geographic
                  ? { type: "geographic", crs: "EPSG:4326" }
                  : { type: "cartesian" },
                encoding: heatmap
                  ? { x: "x", y: "y", group: "sensor" }
                  : { x: "x", y: "y", color: "sensor" },
                view: {
                  mode: geographic ? "map" : "cartesian",
                  auto_fit: true,
                },
              },
            },
          });
        },
      );
      const provider = "https://tiles.openfreemap.org";
      let tileRequests = 0;
      let online = true;
      await page.route(`${provider}/**`, (route) => {
        if (!online) return route.abort("internetdisconnected");
        if (route.request().url().endsWith("/styles/positron")) {
          return route.fulfill({
            json: {
              version: 8,
              sources: {
                streets: {
                  type: "raster",
                  tiles: [`${provider}/fixture/{z}/{x}/{y}.png`],
                  attribution: "Fixture street tiles",
                },
              },
              layers: [{ id: "streets", type: "raster", source: "streets" }],
            },
          });
        }
        tileRequests++;
        return route.fulfill({
          contentType: "image/png",
          body: Buffer.from(
            "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGNYsGABAAPEAeGUtBWxAAAAAElFTkSuQmCC",
            "base64",
          ),
        });
      });
      await page.goto("/app/projects/A/work?work=work_one");
      const map = page.getByRole("region", { name: "Geographic map" });
      await expect(map.locator(".maplibregl-canvas")).toBeVisible();
      await expect.poll(() => tileRequests).toBeGreaterThan(0);
      await expect(map.locator(".maplibregl-ctrl-attrib")).toContainText(
        "Fixture street tiles",
      );
      await expect(
        map.getByRole("button", { name: "Show detailed tiles" }),
      ).toHaveCount(0);
      await expect(map.getByText("Showing a sample of 2 rows.")).toBeVisible();
      await map.getByRole("button", { name: "Zoom in" }).click();

      const layers = map.getByRole("group", { name: "Map layers" });
      const aboveGround = layers.getByRole("checkbox", {
        name: "Above Ground",
      });
      const groundPlane = layers.getByRole("checkbox", {
        name: "Ground Plane",
      });
      const canvas = await map.locator(".maplibregl-canvas").elementHandle();
      await expect(aboveGround).toBeChecked();
      await aboveGround.uncheck();
      await expect(groundPlane).toBeChecked();
      await expect(aboveGround).not.toBeChecked();
      expect(
        await map
          .locator(".maplibregl-canvas")
          .evaluate((node, old) => node === old, canvas),
      ).toBe(true);

      const xy = page.getByRole("region", { name: "Cartesian display" });
      await expect(xy.locator("canvas")).toBeVisible();
      await expect(xy.locator(".maplibregl-canvas")).toHaveCount(0);
      await expect(async () => {
        await xy.locator("canvas").click();
        await expect(xy.getByText("Sensor A", { exact: true })).toBeVisible({
          timeout: 1000,
        });
      }).toPass({ timeout: 10000 });
      const density = page.getByRole("region", { name: "Observation density" });
      await expect(density.locator(".maplibregl-canvas")).toBeVisible();
      const densityLayers = density.getByRole("group", { name: "Map layers" });
      await expect(
        densityLayers.getByRole("checkbox", { name: "Above Ground" }),
      ).toBeChecked();
      await densityLayers
        .getByRole("checkbox", { name: "Ground Plane" })
        .uncheck();
      await expect(
        densityLayers.getByRole("checkbox", { name: "Above Ground" }),
      ).toBeChecked();
      await expect(density.locator(".maplibregl-ctrl-attrib")).toContainText(
        "Fixture street tiles",
      );
      await expect(
        density.getByText(
          /Using the compatibility heatmap renderer|Density is scaled separately|EPSG:4326/,
        ),
      ).toHaveCount(0);
      online = false;
      await page.reload();
      await expect(
        map.getByText(
          "Detailed tiles are unavailable. Showing the offline map.",
        ),
      ).toBeVisible();
      await expect(map.locator(".maplibregl-ctrl-attrib")).toContainText(
        "Natural Earth",
      );
      expect(f.errors).toEqual([]);
    } finally {
      await browser.close();
    }
  });
}

test("analysis renders chart colors and pie percentages", async ({ page }) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  const specs = {
    chart_fur: {
      title: "Fur color observations",
      visualization: {
        chart_type: "bar",
        encoding: {
          x: { field: "category", type: "nominal" },
          y: { field: "count", type: "quantitative" },
        },
        style: {
          category_colors: { Cinnamon: "#d98d59" },
          gridlines: { y: false },
        },
      },
    },
    chart_line: {
      title: "Observations over time",
      visualization: {
        chart_type: "line",
        encoding: {
          x: { field: "category", type: "nominal" },
          y: { field: "count", type: "quantitative", label: "Observations" },
        },
        style: { series_colors: { Observations: "#678fdd" } },
      },
    },
    chart_share: {
      title: "Share of observations by fur color",
      visualization: {
        chart_type: "pie",
        encoding: { category: "category", value: "count" },
        style: { category_colors: { Cinnamon: "#d98d59" } },
      },
    },
  };
  f.state.reply = Object.keys(specs)
    .map((id) => "```harbormaster-visualization\n" + id + "\n```")
    .join("\n\n");
  f.addRun();
  f.finish();
  await page.route(
    "**/workspaces/ws_one/analysis/artifacts/chart_**",
    (route) => {
      const path = new URL(route.request().url()).pathname.split("/");
      const data = path.at(-1) === "data";
      const id = path.at(data ? -2 : -1) as keyof typeof specs;
      return route.fulfill({
        json: data
          ? {
              columns: [
                { name: "category", type: "VARCHAR" },
                { name: "count", type: "BIGINT" },
              ],
              rows: [
                ["Cinnamon", 20],
                ["Gray", 80],
              ],
              next_cursor: null,
              has_more: false,
              sampled: false,
              truncated: false,
            }
          : {
              schema_version: "1.0",
              artifact_id: id,
              type: "chart",
              data: { source_type: "query_result", reference: "result_counts" },
              ...specs[id],
            },
      });
    },
  );
  await page.goto("/app/projects/A/work?work=work_one");
  const bars = page.getByRole("region", { name: specs.chart_fur.title });
  await expect(bars.locator("svg path[fill='#d98d59']")).toBeVisible();
  const line = page.getByRole("region", { name: specs.chart_line.title });
  await expect(
    line.locator("svg path[stroke='#678fdd']").first(),
  ).toBeVisible();
  const pie = page.getByRole("region", { name: specs.chart_share.title });
  await expect(pie.locator("svg path[fill='#d98d59']").first()).toBeVisible();
  await expect(pie.getByText("80%", { exact: true })).toBeVisible();
  const label = bars.getByText("Cinnamon", { exact: true });
  await expect(label).toHaveAttribute("fill", "#b4becd");
  await page.getByRole("button", { name: "Switch to light theme" }).click();
  await expect(label).toHaveAttribute("fill", "#4e5b6c");
  expect(f.errors).toEqual([]);
});

test("worker visualizations persist across runs and can be deleted individually", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.item.kind = "data-science";
  f.state.exists = true;
  f.state.reply = "```harbormaster-visualization\nchart_original\n```";
  f.addRun();
  f.finish();
  const titles: Record<string, string> = {
    chart_original: "Original temperature chart",
    chart_followup: "Follow-up temperature chart",
  };
  const registered = new Set(["chart_original"]);
  const customDetails = new Map<
    string,
    { title?: string; description?: string }
  >();
  await page.route("**/projects/A/analysis/visualization-details", (route) => {
    if (route.request().method() === "POST") {
      const { worker_id, artifact_id, title, description } = route
        .request()
        .postDataJSON();
      expect(worker_id).toBe("ws_one");
      expect(registered.has(artifact_id)).toBe(true);
      const changed = { ...customDetails.get(artifact_id), title, description };
      customDetails.set(artifact_id, changed);
      return route.fulfill({ json: { worker_id, artifact_id, ...changed } });
    }
    return route.fulfill({
      json: {
        titles: [...customDetails].map(([artifact_id, changes]) => ({
          worker_id: "ws_one",
          artifact_id,
          ...changes,
        })),
      },
    });
  });
  await page.route(
    "**/workspaces/ws_one/analysis/artifacts?project_id=A",
    (route) =>
      route.fulfill({
        json: {
          items: [...registered].map((artifact_id) => ({
            artifact_id,
            title: titles[artifact_id],
            type: "chart",
          })),
        },
      }),
  );
  await page.route(
    "**/workspaces/ws_one/analysis/artifacts/chart_**",
    (route) => {
      const path = new URL(route.request().url()).pathname.split("/");
      if (path.at(-1) === "delete") {
        expect(route.request().method()).toBe("POST");
        registered.delete(path.at(-2)!);
        return route.fulfill({ json: { deleted: true } });
      }
      const data = path.at(-1) === "data";
      const id = path.at(data ? -2 : -1)!;
      if (!registered.has(id))
        return route.fulfill({
          status: 404,
          json: { code: "NOT_FOUND_OR_FORBIDDEN" },
        });
      return route.fulfill({
        json: data
          ? {
              columns: [
                { name: "x", type: "DOUBLE" },
                { name: "y", type: "DOUBLE" },
              ],
              rows: [
                [1, 10],
                [2, 12],
              ],
              next_cursor: null,
              has_more: false,
              truncated: false,
              sampled: false,
            }
          : {
              schema_version: "1.0",
              artifact_id: id,
              type: "chart",
              title: titles[id],
              description: "Agent description.",
              data: {
                source_type: "query_result",
                reference: "result_temperature",
              },
              visualization: {
                chart_type: "line",
                encoding: {
                  x: { field: "x", type: "quantitative" },
                  y: { field: "y", type: "quantitative" },
                },
              },
            },
      });
    },
  );

  await page.goto("/app/projects/A/work?work=work_one&view=workspace");
  const visualization = page.getByRole("region", {
    name: titles.chart_original,
  });
  const chart = visualization.locator("[data-analysis-plot]");
  await expect(chart).toBeVisible();
  const chartHeight = (await chart.boundingBox())!.height;
  await visualization.getByRole("button", { name: "Enter fullscreen" }).click();
  await expect(visualization.and(page.locator(":fullscreen"))).toBeVisible();
  expect((await chart.boundingBox())!.height).toBeGreaterThan(chartHeight);
  await visualization.getByRole("button", { name: "Exit fullscreen" }).click();
  await expect(visualization.and(page.locator(":fullscreen"))).toHaveCount(0);
  const editDetails = visualization.getByRole("button", {
    name: "Edit visualization details",
  });
  const dialog = page.getByRole("dialog", {
    name: "Edit visualization details",
  });
  const input = dialog.getByRole("textbox", { name: "Title" });
  const description = dialog.getByRole("textbox", { name: "Description" });
  await editDetails.click();
  await expect(input).toBeFocused();
  await expect(input).toHaveValue(titles.chart_original);
  await input.fill("Discard this edit");
  await input.press("Escape");
  await expect(editDetails).toBeFocused();
  await editDetails.click();
  await expect(input).toHaveValue(titles.chart_original);
  await expect(description).toHaveValue("Agent description.");
  await input.fill("a".repeat(201));
  await expect(
    dialog.getByRole("button", { name: "Save changes" }),
  ).toBeDisabled();
  const renamedTitle = "Morning temperature";
  await input.fill("  " + renamedTitle + "  ");
  await description.fill("Two morning readings.\nTaken before sunrise.");
  await input.press("Enter");
  await expect(dialog).toHaveCount(0);
  const original = page.getByRole("region", { name: renamedTitle });
  await expect(
    original.getByText("Two morning readings. Taken before sunrise."),
  ).toBeVisible();
  await expect(
    original.getByRole("button", { name: "Save visualization" }),
  ).toHaveCount(0);
  const library = page.getByRole("region", {
    name: "Task workspace",
    exact: true,
  });
  await expect(original.locator('div[role="img"] svg')).toBeVisible();
  await library.getByRole("button", { name: "New visualization" }).click();
  const composer = page.getByRole("textbox", { name: "Message to your agent" });
  await expect(composer).toBeFocused();
  await library.getByRole("button", { name: "Actions", exact: true }).click();
  await page
    .getByRole("menuitem", { name: "Ask about this result", exact: true })
    .click();
  await expect(composer).toHaveValue(/Explain visualization chart_original/);
  await expect(page.locator('[data-slot="dropdown-menu-content"]')).toHaveCount(
    0,
  );
  await composer.clear();
  await library.getByRole("button", { name: "Actions", exact: true }).click();
  await page
    .getByRole("menuitem", { name: "Ask agent to edit", exact: true })
    .click();
  await expect(composer).toBeFocused();
  await expect(
    page.getByText("Editing Morning temperature", { exact: true }),
  ).toBeVisible();
  const send = page.getByRole("button", { name: "Send", exact: true });
  await expect(send).toBeDisabled();
  await composer.fill("Use a dashed line.");
  await send.click();
  await expect.poll(() => f.item.latest_attempt_id).toBe("attempt_2");
  const request = f.posts.find(({ path }) => path.endsWith("/continue"));
  expect(request?.body.instruction).toContain(
    "workbench-analysis artifact chart_original",
  );
  expect(request?.body.instruction).toContain(
    "register a new artifact ID on this worker",
  );
  expect(request?.body.instruction).toContain("Use a dashed line.");
  expect(request?.body.workspace_id).toBeUndefined();
  registered.add("chart_followup");
  f.state.reply = "```harbormaster-visualization\nchart_followup\n```";
  f.finish();
  await expect(original).toHaveCount(0);
  const followup = page.getByRole("region", { name: titles.chart_followup });
  await expect(followup.locator('div[role="img"] svg')).toBeVisible();
  await page.reload();
  await expect(library.getByRole("listitem")).toHaveCount(2);
  const originalItem = library.getByRole("button", {
    name: renamedTitle,
    exact: true,
  });
  await originalItem.click();
  await expect(originalItem).toHaveAttribute("aria-pressed", "true");
  await expect(original.locator('div[role="img"] svg')).toBeVisible();
  await expect(
    original.getByText("Two morning readings. Taken before sunrise."),
  ).toBeVisible();
  const sidebarDelete = library.getByRole("button", {
    name: `Delete visualization: ${renamedTitle}`,
  });
  const confirmation = page.getByRole("dialog", {
    name: "Delete visualization",
  });
  await sidebarDelete.click();
  await expect(confirmation.getByText("Morning temperature")).toBeVisible();
  await confirmation.getByRole("button", { name: "Cancel" }).click();
  await expect(originalItem).toBeVisible();
  await sidebarDelete.click();
  await confirmation
    .getByRole("button", { name: "Delete visualization" })
    .click();
  await expect(confirmation).toHaveCount(0);
  await expect(originalItem).toHaveCount(0);
  expect([...registered]).toEqual(["chart_followup"]);
  await page.reload();
  await expect(originalItem).toHaveCount(0);
  await expect(followup.locator('div[role="img"] svg')).toBeVisible();
  expect(f.errors).toEqual([]);
});

test("groups captured revisions under a single task report", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.item.kind = "data-science";
  f.state.exists = true;
  f.state.reply = "Created the report in `reports/field-report.md`.";
  f.addRun();
  f.finish();
  f.addRun("attempt_1");
  f.finish();
  for (const attempt of f.attempts) attempt.artifacts = [];
  const previous = "# First field report\n\nFirst captured observations.\n";
  const completed =
    "# Completed field report\n\n## Findings\n\nMorning readings were higher.\n\n" +
    "| Location | Count |\n| --- | --- |\n| North | 15 |\n\n" +
    "```harbormaster-visualization\nchart_field\n```\n\n## Interpretation\n\nSampling was uneven.\n\n" +
    Array.from(
      { length: 45 },
      (_, index) => `Field note ${index + 1}.\n\n`,
    ).join("");
  let releaseRefresh!: () => void;
  const refreshReady = new Promise<void>((resolve) => {
    releaseRefresh = resolve;
  });
  await page.route("**/work-items/work_one/files?project_id=A", (route) => {
    expect(route.request().method()).toBe("GET");
    return route.fulfill({
      json: { paths: ["reports/report-from-another-task.md"] },
    });
  });
  await page.route(
    "**/work-items/work_one/attempts/*/changes?project_id=A",
    async (route) => {
      const url = route.request().url();
      const earlier = url.includes("/attempt_1/");
      const updated = url.includes("/attempt_3/");
      if (updated) await refreshReady;
      return route.fulfill({
        json: {
          files: [
            {
              path: "reports/field-report.md",
              status: earlier ? "A" : "M",
              source: Buffer.from(
                earlier
                  ? previous
                  : updated
                    ? completed.replace("Field note 45.", "Updated field note.")
                    : completed,
              ).toString("base64"),
            },
            ...(earlier
              ? []
              : [
                  {
                    path: "reports/second-report.md",
                    status: "A",
                    source: Buffer.from(
                      "# Second field report\n\nAnother investigation.\n",
                    ).toString("base64"),
                  },
                ]),
          ],
        },
      });
    },
  );
  await page.route(
    "**/workspaces/ws_one/analysis/artifacts/chart_field**",
    (route) => {
      expect(
        new URL(route.request().url()).searchParams.get("project_id"),
      ).toBe("A");
      return route.fulfill({
        json:
          route.request().method() === "POST"
            ? {
                columns: [
                  { name: "hour", type: "DOUBLE" },
                  { name: "count", type: "DOUBLE" },
                ],
                rows: [
                  [9, 15],
                  [11, 5],
                ],
                next_cursor: null,
                has_more: false,
                truncated: false,
                sampled: false,
              }
            : {
                schema_version: "1.0",
                artifact_id: "chart_field",
                type: "chart",
                title: "Counts by hour",
                data: {
                  source_type: "query_result",
                  reference: "result_field",
                },
                visualization: {
                  chart_type: "line",
                  encoding: {
                    x: { field: "hour", type: "quantitative" },
                    y: { field: "count", type: "quantitative" },
                  },
                },
              },
      });
    },
  );

  await page.goto("/app/projects/A/work?work=work_one&view=report");
  const article = page.getByRole("article", {
    name: "Report content",
  });
  await expect(
    article.getByRole("heading", { name: "Completed field report" }),
  ).toBeVisible();
  const reportList = page.getByRole("complementary", { name: "Reports" });
  await expect(reportList.getByRole("listitem")).toHaveCount(2);
  const version = page.getByRole("combobox", { name: "Report version" });
  await expect(version.locator("option:checked")).toHaveText(
    "Version 2 (latest)",
  );
  await version.selectOption({ label: "Version 1" });
  await expect(article.getByText("First captured observations.")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Edit", exact: true }),
  ).toHaveCount(0);
  await expect(reportList.getByRole("listitem")).toHaveCount(2);
  await version.selectOption("latest");
  await expect(
    article.getByText("Morning readings were higher."),
  ).toBeVisible();
  await expect(
    article.getByRole("cell", { name: "15", exact: true }),
  ).toBeVisible();
  await expect(
    article.getByRole("img", { name: "Counts by hour" }),
  ).toBeVisible();
  await expect(article.getByText("Sampling was uneven.")).toBeVisible();
  await expect(
    page.getByRole("textbox", { name: "Findings (Markdown)" }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Export", exact: true }).click();
  const [markdown] = await Promise.all([
    page.waitForEvent("download"),
    page.getByRole("menuitem", { name: "Download Markdown" }).click(),
  ]);
  expect(markdown.suggestedFilename()).toBe("completed-field-report.md");
  expect(await readFile((await markdown.path())!, "utf8")).toBe(completed);
  await page.getByRole("button", { name: "Export", exact: true }).click();
  const [html] = await Promise.all([
    page.waitForEvent("download"),
    page.getByRole("menuitem", { name: "Download HTML" }).click(),
  ]);
  expect(await readFile((await html.path())!, "utf8")).toContain(
    "data:image/svg+xml,",
  );
  const scroller = page.getByRole("region", { name: "Report body" });
  const chart = article.getByRole("img", { name: "Counts by hour" });
  await chart.evaluate((img) => {
    img.dataset.scrollTest = "original";
  });
  const position = await scroller.evaluate((element) => {
    element.scrollTop = 1100;
    return element.scrollTop;
  });
  expect(position).toBeGreaterThan(500);
  try {
    const refreshing = page.waitForRequest(
      "**/attempts/attempt_3/changes?project_id=A",
    );
    f.addRun("attempt_2");
    f.finish();
    f.attempts.at(-1)!.artifacts = [];
    await refreshing;
    await expect(
      article.getByText("Field note 45.", { exact: true }),
    ).toBeAttached();
    expect(await scroller.evaluate((element) => element.scrollTop)).toBe(
      position,
    );
  } finally {
    releaseRefresh();
  }
  await expect(
    article.getByText("Updated field note.", { exact: true }),
  ).toBeAttached();
  await expect(version.locator("option:checked")).toHaveText(
    "Version 3 (latest)",
  );
  expect(await scroller.evaluate((element) => element.scrollTop)).toBe(
    position,
  );
  await expect(chart).toHaveAttribute("data-scroll-test", "original");

  await page
    .getByRole("button", { name: "Second field report", exact: true })
    .click();
  await expect(article.getByText("Another investigation.")).toBeVisible();
  await page
    .getByRole("button", { name: "Completed field report", exact: true })
    .click();
  await page.getByRole("button", { name: "Edit", exact: true }).click();
  await expect(page.getByRole("textbox", { name: "Report title" })).toHaveValue(
    "Completed field report",
  );
  await expect(
    page.getByRole("textbox", { name: "Findings (Markdown)" }),
  ).toContainText("Morning readings were higher.");
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(
    article.getByRole("heading", { name: "Completed field report" }),
  ).toBeVisible();

  await page.goto(
    "/app/projects/A/work?work=work_one&attempt=attempt_1&view=report",
  );
  await expect(
    article.getByRole("heading", { name: "First field report" }),
  ).toBeVisible();
  await expect(article.getByText("First captured observations.")).toBeVisible();
  expect(f.errors).toEqual([]);
});

test("opens and revises a report visualization from the unified Analysis workspace", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.item.kind = "data-science";
  f.state.exists = true;
  const path = "reports/squirrel-report.md";
  f.state.reply = `Created the report in [Squirrel report](${path}).`;
  f.addRun();
  f.finish();
  const fence = (ref: string) =>
    "```harbormaster-visualization\n" + ref + "\n```";
  const markdown = [
    "# Squirrel report",
    fence("density"),
    fence("ws_one/fur"),
    fence("ws_one/density"),
  ].join("\n\n");
  await page.route(
    "**/work-items/work_one/attempts/*/changes?project_id=A",
    (route) =>
      route.fulfill({
        json: {
          files: [
            {
              path,
              status: "A",
              source: Buffer.from(markdown).toString("base64"),
            },
          ],
        },
      }),
  );
  const titles: Record<string, string> = {
    density: "Squirrel density",
    fur: "Fur colors",
  };
  await page.route("**/projects/A/analysis/visualization-details", (route) =>
    route.fulfill({ json: { titles: [] } }),
  );
  await page.route(
    "**/workspaces/ws_one/analysis/artifacts?project_id=A",
    (route) =>
      route.fulfill({
        json: {
          items: Object.entries(titles).map(([artifact_id, title]) => ({
            artifact_id,
            title,
            type: "table",
          })),
        },
      }),
  );
  await page.route("**/workspaces/ws_one/analysis/artifacts/**", (route) => {
    const artifactID = new URL(route.request().url()).pathname
      .split("/")
      .at(-1)!;
    return route.fulfill({
      json:
        route.request().method() === "GET"
          ? {
              schema_version: "1.0",
              artifact_id: artifactID,
              type: "table",
              title: titles[artifactID],
              data: {
                source_type: "query_result",
                reference: "result_squirrels",
              },
              visualization: { page_size: 25 },
            }
          : {
              columns: [{ name: "Count", type: "BIGINT" }],
              rows: [[3]],
              next_cursor: null,
              has_more: false,
              truncated: false,
              sampled: false,
            },
    });
  });
  const reportWrites: { markdown: string }[] = [];
  await page.route("**/work-items/work_one/reports", (route) => {
    reportWrites.push(route.request().postDataJSON());
    return route.fulfill({ json: { path } });
  });
  await page.goto("/app/projects/A/work?work=work_one&view=workspace");
  const library = page.getByRole("region", {
    name: "Task workspace",
    exact: true,
  });
  const results = library.getByRole("list", { name: "Visualization list" });
  await expect(results.getByRole("listitem")).toHaveCount(2);
  const explorerHeading = library.getByRole("heading", {
    name: "Explore data",
  });
  const visualizationsHeading = library.getByRole("heading", {
    name: "Visualizations",
    exact: true,
  });
  await expect(explorerHeading).toBeVisible();
  await expect(visualizationsHeading).toBeVisible();
  const [explorerText, visualizationsText] = await explorerHeading
    .or(visualizationsHeading)
    .evaluateAll((headings) =>
      headings.map((heading) => {
        const range = document.createRange();
        range.selectNodeContents(heading);
        return range.getBoundingClientRect().left;
      }),
    );
  expect(explorerText).toBe(visualizationsText);
  const table = library.getByRole("region", { name: "Squirrel density" });
  const browseDatasets = library.getByRole("button", {
    name: "Browse datasets",
  });
  const tableHeading = table.getByRole("heading", { name: "Squirrel density" });
  await expect(browseDatasets).toBeVisible();
  await expect(tableHeading).toBeVisible();
  expect((await browseDatasets.boundingBox())?.x).toBe(
    (await tableHeading.boundingBox())?.x,
  );
  await expect(
    table.getByRole("cell", { name: "3", exact: true }),
  ).toBeVisible();
  await expect(table.getByLabel("Rows per page")).toHaveValue("25");
  await expect(
    results.getByRole("button", { name: /^Squirrel density/ }),
  ).toContainText("Used in report");
  await expect(
    results.getByRole("button", { name: /^Fur colors/ }),
  ).toBeVisible();
  await expect(
    page
      .getByRole("group", { name: "Workspace tools" })
      .getByRole("button", { name: "Visualizations", exact: true }),
  ).toHaveCount(0);
  await conversationTool(page, "Report");
  await page.getByRole("button", { name: "Edit", exact: true }).click();
  const picker = page.getByRole("list", {
    name: "Visualizations",
    exact: true,
  });
  await expect(picker.getByRole("listitem")).toHaveCount(2);
  await expect(
    picker.getByRole("button", { name: "Copy Markdown for Squirrel density" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Embedded results" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: /Ask agent to edit/ }),
  ).toHaveCount(0);
  expect(reportWrites).toHaveLength(0);
  const findings = page.getByRole("textbox", { name: "Findings (Markdown)" });
  await findings.fill(
    (await findings.inputValue()) + "\n\nCensus observations.",
  );
  await page.getByRole("button", { name: "Save and preview" }).click();
  const report = page.getByRole("article", { name: "Report content" });
  await expect(report.getByText("Census observations.")).toBeVisible();
  expect(reportWrites).toHaveLength(1);
  expect(reportWrites[0]).toMatchObject({
    path,
    previous: markdown,
    worker_id: "ws_one",
  });
  expect(reportWrites[0].markdown).toContain("Census observations.");
  expect(reportWrites[0].markdown).toContain(fence("ws_one/fur"));
  await page.getByRole("button", { name: "Edit", exact: true }).click();
  await picker.getByRole("link", { name: "Fur colors" }).click();
  await expect(
    results.getByRole("button", { name: /^Fur colors/ }),
  ).toHaveAttribute("aria-pressed", "true");
  await library.getByRole("button", { name: "Actions", exact: true }).click();
  await page
    .getByRole("menuitem", { name: "Ask agent to edit in report" })
    .click();
  const composer = page.getByRole("textbox", { name: "Message to your agent" });
  await expect(composer).toBeFocused();
  await expect(page.getByText("Editing Fur colors")).toBeVisible();
  await composer.fill("Combine the rare color categories.");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect
    .poll(
      () =>
        f.posts.find(({ path }) => path.endsWith("/continue"))?.body
          .instruction,
    )
    .toContain("Combine the rare color categories.");
  const instruction = f.posts.find(({ path }) => path.endsWith("/continue"))
    ?.body.instruction;
  expect(instruction).toContain('["reports/squirrel-report.md"]');
  expect(instruction).toContain("embedded references to ws_one/fur");
  expect(instruction).not.toContain("density");
  expect(f.errors).toEqual([]);
});

test("exports a report with findings, a bounded table, a chart, and a map", async ({
  playwright,
  baseURL,
}) => {
  const browser = await playwright.chromium.launch({
    args: ["--enable-unsafe-swiftshader", "--use-angle=swiftshader"],
  });
  try {
    const page = await browser.newPage({
      baseURL,
      acceptDownloads: true,
      permissions: ["clipboard-read", "clipboard-write"],
    });
    const f = await conversationFixture(page);
    f.item.kind = "data-science";
    f.state.exists = true;
    f.state.reply =
      "## Findings\n\nEarly readings were higher.\n\n```harbormaster-visualization\nold_chart\n```";
    f.addRun();
    f.finish();
    f.attempts[0].artifacts = [];
    const files = new Map<string, string>();
    await page.route("**/work-items/work_one/files?project_id=A", (route) => {
      if (route.request().method() === "GET")
        return route.fulfill({ json: { paths: [...files.keys()] } });
      const { path } = route.request().postDataJSON();
      return route.fulfill({
        json: {
          path,
          content: Buffer.from(files.get(path)!).toString("base64"),
        },
      });
    });
    await page.route("**/work-items/work_one/reports", (route) => {
      const { path, markdown, previous, worker_id } = route
        .request()
        .postDataJSON();
      expect(worker_id).toBe("ws_one");
      expect(previous).toBe(files.get(path));
      files.set(path, markdown);
      return route.fulfill({ json: { path } });
    });
    const items = [
      {
        artifact_id: "table_readings",
        worker_id: "ws_one",
        title: "Recorded readings",
        type: "table",
      },
      {
        artifact_id: "chart_totals",
        worker_id: "ws_one",
        title: "Reading totals",
        type: "chart",
      },
      {
        artifact_id: "geo_readings",
        worker_id: "ws_two",
        title: "Reading locations",
        type: "geo",
      },
      {
        artifact_id: "old_table",
        worker_id: "ws_old",
        title: "Archived table",
        type: "table",
      },
    ];
    await page.route("**/projects/A/analysis/visualization-details", (route) =>
      route.fulfill({
        json: {
          titles: [
            {
              worker_id: "ws_one",
              artifact_id: "chart_totals",
              description: "Checked against the source.",
            },
          ],
        },
      }),
    );
    await page.route(
      "**/workspaces/ws_one/analysis/artifacts?project_id=A",
      (route) =>
        route.fulfill({
          json: {
            items: items.filter(({ worker_id }) => worker_id === "ws_one"),
          },
        }),
    );
    await page.route("https://tiles.openfreemap.org/**", (route) =>
      route.abort(),
    );
    await page.route("**/workspaces/*/analysis/artifacts/**", (route) => {
      const url = new URL(route.request().url());
      expect(url.searchParams.get("project_id")).toBe("A");
      const path = url.pathname.split("/");
      const data = path.at(-1) === "data";
      const id = path.at(data ? -2 : -1);
      const item = items.find(({ artifact_id }) => artifact_id === id);
      if (!item || id === "old_table")
        return route.fulfill({
          status: 404,
          json: { code: "ANALYSIS_ARTIFACT_UNAVAILABLE" },
        });
      expect(url.pathname).toContain("/workspaces/" + item.worker_id + "/");
      if (data) {
        expect(route.request().postDataJSON()).toEqual(
          item.type === "table" ? { limit: 25 } : {},
        );
        return route.fulfill({
          json: {
            ...{
              table: {
                columns: [
                  { name: "Sensor | label", type: "VARCHAR" },
                  { name: "Count", type: "BIGINT" },
                ],
                rows: [
                  ["South <gate>", 15],
                  ["North", null],
                ],
              },
              chart: {
                columns: [
                  { name: "bin", type: "DOUBLE" },
                  { name: "count", type: "DOUBLE" },
                ],
                rows: [
                  [1, 10],
                  [2, 15],
                ],
              },
              geo: {
                columns: [
                  { name: "lon", type: "DOUBLE" },
                  { name: "lat", type: "DOUBLE" },
                  { name: "group", type: "VARCHAR" },
                ],
                rows: [
                  [-0.118, 51.5, "North"],
                  [-0.122, 51.498, "South"],
                ],
              },
            }[item.type],
            next_cursor: item.type === "table" ? "another_page" : null,
            has_more: item.type === "table",
            sampled: item.type === "geo",
            truncated: false,
          },
        });
      }
      return route.fulfill({
        json: {
          ...item,
          schema_version: "1.0",
          data: { source_type: "query_result", reference: "result_readings" },
          visualization: {
            table: { page_size: 25 },
            chart: {
              chart_type: "line",
              encoding: {
                x: { field: "bin", type: "quantitative" },
                y: { field: "count", type: "quantitative" },
              },
            },
            geo: {
              geometry: "point",
              coordinate_system: { type: "geographic", crs: "EPSG:4326" },
              encoding: { x: "lon", y: "lat", group: "group" },
              view: { mode: "map" },
            },
          }[item.type],
        },
      });
    });
    await page.goto("/app/projects/A/work?work=work_one&view=report");
    const findings = page.getByRole("textbox", { name: "Findings (Markdown)" });
    await expect(findings).toHaveValue(
      "## Findings\n\nEarly readings were higher.",
    );
    await page
      .getByRole("textbox", { name: "Report title" })
      .fill("Field report");
    await findings.fill("");
    await page.getByRole("button", { name: "Heading", exact: true }).click();
    await expect(findings).toHaveValue("## Heading");
    await expect
      .poll(() =>
        findings.evaluate((input: HTMLTextAreaElement) =>
          input.value.slice(input.selectionStart, input.selectionEnd),
        ),
      )
      .toBe("Heading");
    await page.keyboard.insertText("Findings");
    await findings.press("End");
    await findings.press("Enter");
    await findings.press("Enter");
    await page.getByRole("button", { name: "Bold", exact: true }).click();
    await expect(findings).toHaveValue("## Findings\n\n**bold text**");
    await expect
      .poll(() =>
        findings.evaluate((input: HTMLTextAreaElement) =>
          input.value.slice(input.selectionStart, input.selectionEnd),
        ),
      )
      .toBe("bold text");
    await page.keyboard.insertText("Early readings were higher.");
    for (const { title, worker_id, artifact_id } of [...items, items[1]]) {
      const snippet = [
        "```harbormaster-visualization",
        `${worker_id}/${artifact_id}`,
        "```",
      ].join("\n");
      if (worker_id === "ws_one") {
        await page
          .getByRole("button", {
            name: `Copy Markdown for ${title}`,
            exact: true,
          })
          .click();
        await expect(
          page
            .getByRole("status")
            .filter({ hasText: `Copied Markdown for ${title}.` }),
        ).toBeVisible();
        expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
          snippet,
        );
      } else {
        await page.evaluate(
          (value) => navigator.clipboard.writeText(value),
          snippet,
        );
      }
      const before = await findings.inputValue();
      await findings.fill(before + "\n\n");
      await findings.press("Control+End");
      await findings.press("Control+v");
      await expect(findings).toHaveValue(before + "\n\n" + snippet);
    }
    await page.getByRole("button", { name: "Save and preview" }).click();
    const report = page.getByRole("article", { name: "Report content" });
    await expect(
      report.getByRole("heading", { name: "Field report" }),
    ).toBeVisible({ timeout: 20000 });
    await expect(report.getByText("Early readings were higher.")).toBeVisible();
    await expect(
      report.getByRole("columnheader", { name: "Sensor | label" }),
    ).toBeVisible();
    await expect(
      report.getByRole("cell", { name: "South <gate>" }),
    ).toBeVisible();
    await expect(report.getByText("Showing the first 2 rows.")).toBeVisible();
    await expect(
      report.getByText("Data is unavailable on the originating worker."),
    ).toBeVisible();
    await expect(
      report.getByRole("img", { name: "Reading locations" }),
    ).toHaveAttribute("src", /^data:image\/png;/);
    await expect(report.getByText("Natural Earth")).toBeVisible();
    await expect(
      report.getByRole("img", { name: "Reading totals" }),
    ).toHaveCount(2);
    await expect(
      report.getByRole("img", { name: "Reading totals" }).first(),
    ).toHaveAttribute("src", /^data:image\/svg\+xml,/);

    const reportBody = page.getByRole("region", { name: "Report body" });
    const edit = page.getByRole("button", { name: "Edit", exact: true });
    const exportButton = page.getByRole("button", {
      name: "Export",
      exact: true,
    });
    const editTop = (await edit.boundingBox())?.y;
    const exportTop = (await exportButton.boundingBox())?.y;
    await reportBody.hover();
    await page.mouse.wheel(0, 600);
    await expect
      .poll(() => reportBody.evaluate((body) => body.scrollTop))
      .toBeGreaterThan(0);
    expect((await edit.boundingBox())?.y).toBe(editTop);
    expect((await exportButton.boundingBox())?.y).toBe(exportTop);

    async function download(label: string, file: string) {
      await page.getByRole("button", { name: "Export", exact: true }).click();
      const pending = page.waitForEvent("download");
      await page.getByRole("menuitem", { name: label }).click();
      const result = await pending;
      expect(result.suggestedFilename()).toBe(file);
      return readFile((await result.path())!, "utf8");
    }
    const markdown = await download("Download Markdown", "field-report.md");
    expect(markdown).toContain("ws_one/chart_totals");
    expect(markdown).toContain("ws_two/geo_readings");
    expect(markdown).not.toContain("old_chart");
    const html = await download("Download HTML", "field-report.html");
    const offline = await browser.newPage({ offline: true });
    await offline.setContent(html);
    await expect(
      offline.getByText("Early readings were higher."),
    ).toBeVisible();
    await expect
      .poll(() =>
        offline
          .locator("figure img")
          .evaluateAll((images) =>
            images.map((image) => (image as HTMLImageElement).naturalWidth),
          ),
      )
      .toEqual([880, 880, 880]);
    await offline.close();

    await page.evaluate(() => {
      window.print = () => {
        throw new Error("Native PDF export must not open the print dialog");
      };
    });
    const pdf = await download("Download PDF", "field-report.pdf");
    expect(pdf.startsWith("%PDF-")).toBe(true);
    expect(pdf).toContain("/Subtype /Image");
    expect(pdf).toContain("/Type /Font");
    expect(pdf.length).toBeGreaterThan(10_000);

    await page.getByRole("button", { name: "Edit", exact: true }).click();
    await expect(
      page.getByRole("textbox", { name: "Report title" }),
    ).toHaveValue("Field report");
    await page
      .getByRole("textbox", { name: "Report title" })
      .fill("Revised field report");
    await page.getByRole("button", { name: "Save and preview" }).click();
    await expect(
      report.getByRole("heading", { name: "Revised field report" }),
    ).toBeVisible();
    expect(files.size).toBe(1);
    await page.getByRole("button", { name: "New report", exact: true }).click();
    await page.getByRole("textbox", { name: "Report title" }).fill("Follow-up");
    await findings.fill("Separately collected readings.");
    await page.getByRole("button", { name: "Save and preview" }).click();
    await expect(
      report.getByText("Separately collected readings."),
    ).toBeVisible();
    expect(files.size).toBe(2);
    await page.reload();
    await page
      .getByRole("button", { name: "Revised field report", exact: true })
      .click();
    await expect(report.getByText("Early readings were higher.")).toBeVisible();
    await page.getByRole("button", { name: "Follow-up", exact: true }).click();
    await expect(
      report.getByText("Separately collected readings."),
    ).toBeVisible();
    expect(f.errors).toEqual([]);
  } finally {
    await browser.close();
  }
});

test("selecting Research / Data Science starts in the analysis workspace", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1600, height: 900 });
  const f = await conversationFixture(page);
  f.item.kind = "data-science";
  await page.route("**/workspaces/ws_one/analysis/datasets?**", (route) =>
    route.fulfill({
      json: {
        items: [
          {
            dataset_id: "dataset_sensor",
            name: "sensors.csv",
            format: "csv",
            size_bytes: 32,
            sha256: "a".repeat(64),
          },
        ],
      },
    }),
  );
  await page.route("**/workspaces/ws_one/analysis/artifacts?**", (route) =>
    route.fulfill({ json: { items: [] } }),
  );
  await page.route("**/analysis/datasets/dataset_sensor/schema?**", (route) =>
    route.fulfill({
      json: {
        name: "sensors.csv",
        columns: [{ name: "reading", type: "DOUBLE" }],
      },
    }),
  );
  await page.route("**/analysis/datasets/dataset_sensor/preview?**", (route) =>
    route.fulfill({
      json: { columns: [{ name: "reading", type: "DOUBLE" }], rows: [[42]] },
    }),
  );
  await page.goto("/app/projects/A/work");
  const start = page.getByRole("button", {
    name: "Create thread",
    exact: true,
  });
  await expect(start).toBeDisabled();
  await page.getByLabel("Work profile").selectOption({
    label: "Research / Data Science",
  });
  await page
    .getByLabel("Model", { exact: true })
    .selectOption({ label: "worker-model · Codex" });
  await start.click();
  await page
    .getByLabel("Message to your agent")
    .fill("Find anomalies in the sensor readings.");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  const datasetList = page.getByRole("region", { name: "Worker datasets" });
  const tools = page.getByRole("group", { name: "Workspace tools" });
  const analysis = tools.getByRole("button", { name: "Analysis", exact: true });
  await expect(analysis).toHaveAttribute("aria-pressed", "true");
  await expect(
    tools.getByRole("button", { name: "Report", exact: true }),
  ).toBeVisible();
  const workspace = page.getByRole("region", { name: "Task workspace" });
  await expect(
    workspace.getByText("Agent is working", { exact: true }),
  ).toBeVisible();
  await expect(datasetList).toHaveCount(0);
  await tools.getByRole("button", { name: "Datasets", exact: true }).click();
  await expect(page).toHaveURL(/view=datasets/);
  await expect(
    page.getByRole("region", { name: "Datasets page" }),
  ).toBeVisible();
  await expect(datasetList).toContainText("sensors.csv");
  await expect(
    page.getByRole("region", { name: "Task workspace" }),
  ).toBeHidden();
  await page.getByRole("button", { name: "Conversation", exact: true }).click();
  await expect(datasetList).toHaveCount(0);
  await page.getByRole("button", { name: "Workspace", exact: true }).click();
  await expect(page).toHaveURL(/view=datasets/);
  await expect(datasetList).toContainText("sensors.csv");
  await analysis.click();
  await expect(datasetList).toHaveCount(0);
  f.finish();
  await expect(
    workspace.getByRole("heading", { name: "No visualizations yet" }),
  ).toBeVisible();
  await expect(
    tools.getByRole("button", { name: "Preview", exact: true }),
  ).toHaveCount(0);
  expect(f.posts.find((p) => p.path === "/ui-api/work-items")?.body.kind).toBe(
    "data-science",
  );
  const explorer = workspace.getByRole("region", { name: "Explore data" });
  const datasetSelect = explorer.getByRole("combobox", {
    name: "Dataset to explore",
  });
  await expect(datasetSelect).toHaveValue("dataset_sensor");
  const heading = (await explorer.getByRole("heading").boundingBox())!;
  const datasetControl = (await datasetSelect.boundingBox())!;
  const newVisualization = (await explorer
    .getByRole("button", { name: "New visualization" })
    .boundingBox())!;
  expect(datasetControl.y).toBe(newVisualization.y);
  expect(heading.y + heading.height / 2).toBeCloseTo(
    datasetControl.y + datasetControl.height / 2,
  );
  await explorer.getByRole("button", { name: "Profile dataset" }).click();
  const composer = page.getByRole("textbox", { name: "Message to your agent" });
  await expect(composer).toHaveValue(/Profile dataset dataset_sensor/);
  expect(f.posts.filter(({ path }) => path.endsWith("/continue"))).toHaveLength(
    1,
  );
  await composer.fill("What is the distribution of readings?");
  const analysisURL = page.url();
  await explorer.getByRole("button", { name: "Preview rows" }).click();
  const rowPreview = explorer.getByRole("region", {
    name: "Dataset schema and preview",
  });
  await expect(rowPreview.getByRole("cell", { name: "42" })).toBeVisible();
  await expect(page).toHaveURL(analysisURL);
  await expect(analysis).toHaveAttribute("aria-pressed", "true");
  await expect(datasetList).toHaveCount(0);
  await explorer.getByRole("button", { name: "Hide rows" }).click();
  await expect(rowPreview).toHaveCount(0);
  await expect(composer).toHaveValue("What is the distribution of readings?");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect
    .poll(
      () =>
        f.posts.filter(({ path }) => path.endsWith("/continue"))[1]?.body
          .instruction,
    )
    .toContain("Selected dataset for this request: dataset_sensor.");
});

test("renders the geographic aggregation and shape layers", async ({
  playwright,
  baseURL,
}) => {
  const browser = await playwright.chromium.launch({
    args: ["--enable-unsafe-swiftshader", "--use-angle=swiftshader"],
  });
  try {
    const page = await browser.newPage({ baseURL });
    const f = await conversationFixture(page);
    f.state.exists = true;
    f.state.reply = "```harbormaster-visualization\ngeo_layers\n```";
    f.addRun();
    f.finish();
    const path = [
      [-0.12, 51.5],
      [-0.113, 51.51],
      [-0.103, 51.503],
    ];
    const rings = [[...path, path[0]]];
    const shapes = {
      path,
      polygon: rings,
      geojson: { type: "Polygon", coordinates: rings },
    };
    let layer:
      "grid" | "hexagon" | "contour" | "screen_grid" | keyof typeof shapes =
      "grid";
    await page.route("https://tiles.openfreemap.org/**", (route) =>
      route.abort(),
    );
    await page.route(
      "**/workspaces/ws_one/analysis/artifacts/geo_layers**",
      (route) => {
        const shape =
          layer in shapes ? (layer as keyof typeof shapes) : undefined;
        if (new URL(route.request().url()).pathname.endsWith("/data")) {
          return route.fulfill({
            json: {
              columns: shape
                ? [{ name: "shape", type: "VARCHAR" }]
                : [
                    { name: "x", type: "DOUBLE" },
                    { name: "y", type: "DOUBLE" },
                  ],
              rows: shape ? [[JSON.stringify(shapes[shape])]] : path,
              next_cursor: null,
              has_more: false,
              truncated: false,
              sampled: false,
            },
          });
        }
        return route.fulfill({
          json: {
            schema_version: "1.0",
            artifact_id: "geo_layers",
            type: "geo",
            title: "Geographic layers",
            data: { source_type: "dataset", reference: "dataset_geo" },
            visualization: {
              ...(shape
                ? { geometry: shape, encoding: { shape: "shape" } }
                : {
                    geometry: "point",
                    layer,
                    encoding: { x: "x", y: "y" },
                  }),
              coordinate_system: { type: "geographic", crs: "EPSG:4326" },
              view: { mode: "map", auto_fit: true },
            },
          },
        });
      },
    );
    for (const next of [
      "grid",
      "hexagon",
      "contour",
      "screen_grid",
      "path",
      "polygon",
      "geojson",
    ] as const) {
      layer = next;
      await page.goto("/app/projects/A/work?work=work_one");
      const map = page.getByRole("region", { name: "Geographic layers" });
      await expect(
        map.getByLabel(`Geographic ${next.replace("_", " ")}`),
      ).toBeVisible();
      await expect(map.locator(".maplibregl-ctrl-attrib")).toContainText(
        "Natural Earth",
      );
      // A click after initialization schedules another render with the deck.gl overlay.
      await map.getByRole("button", { name: "Zoom in" }).click();
      await expect(map.locator(".maplibregl-canvas")).toBeVisible();
      await expect(
        map.getByText(/geographic renderer could not draw/i),
      ).toHaveCount(0);
    }
    expect(f.errors).toEqual([]);
  } finally {
    await browser.close();
  }
});
