import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

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
    foreignHistory: false,
    request: "Investigate the experiment.",
    reply: "All four tests passed. The report is ready.",
    connects: 0,
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
      return route.fulfill({ json: { items: apps } });
    }
    if (path.endsWith("/services/service_live/revoke")) {
      apps[0].state = "revoked";
      return route.fulfill({ json: apps[0] });
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
                    state: "READY",
                    ready: true,
                  },
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
        body: "# Results\nAll four tests passed.\n<script>window.untrusted=true</script>",
        contentType: "application/octet-stream",
      });
    if (path.endsWith("/history")) {
      const task = url.searchParams.get("episode"),
        a = attempts.find((a) => a.task_id === task)!;
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
              a.message
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
          workspace_id: "ws_one",
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
    return route.fulfill({ json: { items: [] } });
  });
  return { item, attempts, posts, state, finish, errors, apps, ports };
}

test("starts work, shows live tools, inspects files, accepts, and sends changes in one conversation", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  await page.goto("/app/projects/A/work");
  await page
    .getByLabel("Message to your agent")
    .fill("Investigate the experiment.");
  await page.getByLabel("Worker", { exact: true }).selectOption("ws_one");
  await page
    .getByLabel("Model", { exact: true })
    .selectOption(JSON.stringify(["claude", "chosen-model"]));
  await page.getByText("Keep specific output files", { exact: true }).click();
  await page.getByLabel("Retained output paths").fill("outputs/report.md");
  await page.getByRole("button", { name: "Start work", exact: true }).click();
  await expect(page).toHaveURL(/work=work_one/);
  expect(
    f.posts.find((p) => p.path === "/ui-api/work-items")?.body,
  ).toMatchObject({ provider: "claude", model: "chosen-model" });
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
  await expect(page.getByRole("link", { name: "Review output" })).toBeVisible({
    timeout: 10000,
  });
  await expect(
    page.getByText("All four tests passed. The report is ready.", {
      exact: true,
    }),
  ).toBeVisible({ timeout: 10000 });
  await conversationTool(page, "Files");
  await page.getByRole("button", { name: "Inspect", exact: true }).click();
  await expect(page.getByLabel("Output preview")).toContainText(
    "All four tests passed.",
  );
  expect(await page.evaluate(() => "untrusted" in window)).toBe(false);
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Close", exact: true })
    .click();
  await page.getByRole("button", { name: "Conversation", exact: true }).click();
  await page.getByRole("link", { name: "Review output", exact: true }).click();
  await expect(page).toHaveURL(/attempt=attempt_1/);
  await page
    .getByRole("button", { name: "Approve this revision", exact: true })
    .click();
  await expect(page.getByText("This revision was approved.")).toBeVisible();
  await page
    .getByRole("link", { name: "Open conversation", exact: true })
    .click();
  await page.getByRole("button", { name: "Filter threads by status" }).click();
  await expect(
    page.getByRole("menuitemradio", { name: "Done (1)", exact: true }),
  ).toBeVisible();
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
  const revisions = f.posts.filter((p) => p.path.endsWith("/continue"));
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
    await expect(
      page.getByRole("link", { name: "Review output", exact: true }),
    ).toBeVisible({ timeout: 10000 });
    await page.setViewportSize({ width, height: 900 });
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
  await expect(page.getByLabel("Message to your agent")).toHaveValue("");
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
  await popup.close();
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
  expect(f.errors).toEqual([]);
});

test("workspace supports charts, desktop launch and mobile switching with a shared composer", async ({
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
        body: "<h1>Desktop connected</h1>",
      }),
  );
  await page.goto("/app/projects/A/work?work=work_one");
  await conversationTool(page, "Visualize data");
  await page
    .getByLabel("Chart data")
    .fill("experiment,score\nbaseline,72\nrevision,86");
  await page.getByRole("button", { name: "Draw chart" }).click();
  await expect(
    page.getByRole("img", { name: "score by experiment" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Explore", exact: true }).click();
  await page.getByLabel("Chart type").selectOption("line");
  await expect(page.locator(".browser-chart svg")).toHaveCount(1);
  await conversationTool(page, "Preview");
  await page.getByRole("tab", { name: "Desktop", exact: true }).click();
  await page.getByRole("button", { name: "Start worker desktop" }).click();
  await expect(
    page.frameLocator('iframe[title="Workspace desktop"]').getByRole("heading"),
  ).toHaveText("Desktop connected");
  expect(
    f.posts.find((p) => p.path.endsWith("/applications"))?.body.application,
  ).toEqual({
    kind: "desktop",
    directory: ".",
    command: "workbench-desktop",
    port: 6080,
  });
  await conversationTool(page, "Visualize data");
  await expect(
    page.getByRole("img", { name: "score by experiment" }),
  ).toBeVisible();
  await conversationTool(page, "Preview");
  await page.getByRole("tab", { name: "Desktop", exact: true }).click();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "Conversation", exact: true }).click();
  await expect(
    page.getByRole("complementary", { name: "Preview workspace" }),
  ).toBeHidden();
  await conversationTool(page, "Preview");
  await expect(
    page.frameLocator('iframe[title="Workspace desktop"]').getByRole("heading"),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  expect(f.errors).toEqual([]);
});

test("preview selection adds a draft only from the selected frame and returns after switching tabs", async ({
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
  await expect(page.getByRole("link", { name: "Review output" })).toBeVisible();
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
  await expect(page.getByLabel("Message to your agent")).toHaveValue(
    'Change #hero on / ("Build something"): ',
  );
  await expect(page.getByLabel("Message to your agent")).toBeFocused();
  expect(f.posts.filter((p) => p.path.endsWith("/continue"))).toHaveLength(0);
  await page.getByRole("tab", { name: "Desktop", exact: true }).click();
  const renewed = page.waitForRequest(
    (r) => r.url().endsWith("/connect") && r.method() === "POST",
  );
  await page.getByRole("tab", { name: "Web", exact: true }).click();
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
  await expect(
    page.getByRole("link", { name: "Review output", exact: true }),
  ).toBeVisible();
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
  await expect(
    page.getByRole("region", { name: "Conversation visualization" }),
  ).toHaveCount(0);
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

test("conversation list shows request titles and searches their text", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  f.item.specification_path = "work-requests/original.md";
  const second = {
    ...f.item,
    id: "work_two",
    specification_path: "work-requests/second.md",
  };
  await page.route("**/ui-api/work-items?project_id=A", (route) =>
    route.fulfill({ json: { items: [f.item, second] } }),
  );
  await page.route(
    "**/ui-api/work-items/*/instruction?project_id=A",
    (route) => {
      const id = route.request().url().includes("work_two")
        ? second.id
        : f.item.id;
      return route.fulfill({
        json: {
          project_id: "A",
          work_item_id: id,
          instruction:
            "Workbench presentation: interactive-v1.\nChart support.\n\nCurrent request:\n" +
            (id === second.id ? "Fix the login form" : "Build a photo gallery"),
        },
      });
    },
  );
  await page.goto("/app/projects/A/work");
  const list = page.getByRole("complementary", { name: "Threads" });
  await expect(
    list.getByRole("button", { name: "Build a photo gallery" }),
  ).toBeVisible();
  await expect(
    list.getByRole("button", { name: "Fix the login form" }),
  ).toBeVisible();
  await page.getByRole("textbox", { name: "Search threads" }).fill("gallery");
  await expect(
    list.getByRole("button", { name: "Fix the login form" }),
  ).toHaveCount(0);
  await list.getByRole("button", { name: "Build a photo gallery" }).click();
  await expect(
    page.getByRole("heading", { name: "Build a photo gallery", exact: true }),
  ).toBeVisible();
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
  await page.route("**/ui-api/workspaces/ws_one/history?*", (route) => {
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
  });
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
    if (viewport.width < 1200)
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
    page.getByRole("button", { name: /Hide threads|Show threads/ }),
  ).toHaveCount(0);
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
  expect(composer.width).toBeLessThanOrEqual(960);
  expect(
    Math.abs(message.x + message.width / 2 - composer.x - composer.width / 2),
  ).toBeLessThan(2);
  await expect(page.locator(".conversation-composer textarea")).toHaveCSS(
    "min-height",
    "78px",
  );
  expect(
    Math.abs(
      (await page.locator(".conversation-activity").boundingBox())!.x -
        message.x,
    ),
  ).toBeLessThan(2);
  await page.screenshot({ path: "artifacts/work-design-expanded.png" });
  await page.setViewportSize({ width: 390, height: 844 });
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
  await page.getByRole("button", { name: "Preview", exact: true }).click();
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

test("new work explains byte and output-file limits without submitting or losing the draft", async ({
  page,
}) => {
  const fixture = await conversationFixture(page);
  await page.goto("/app/projects/A/work");
  const message = page.getByRole("textbox", {
    name: "Message to your agent",
    exact: true,
  });
  const start = page.getByRole("button", { name: "Start work", exact: true });
  await message.fill("😀".repeat(16385));
  await expect(message).toHaveAttribute("aria-invalid", "true");
  await expect(page.getByRole("alert")).toContainText("exceeds 64 KiB");
  await expect(start).toBeDisabled();
  await message.fill("Review the source and save a report.");
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
  await expect(message).toHaveValue("Review the source and save a report.");
  await outputs.fill("outputs/report.md");
  await expect(page.getByRole("alert")).toHaveCount(0);
  await expect(start).toBeEnabled();
  expect(fixture.posts).toEqual([]);
});

test("thread creation has one composer and no separate work-item form", async ({
  page,
}) => {
  await conversationFixture(page);
  await page.goto("/app/projects/A/work");
  await expect(page.getByLabel("Message to your agent")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Create work item", exact: true }),
  ).toHaveCount(0);
  await expect(page.getByLabel("Model ID", { exact: true })).toHaveCount(0);
});

test("retrying the worker list never submits the work form or clears its draft", async ({
  page,
}) => {
  const fixture = await conversationFixture(page);
  let offline = true;
  await page.route("**/ui-api/workspaces?**", (route) =>
    offline
      ? route.fulfill({ status: 503, json: { code: "STORAGE_UNAVAILABLE" } })
      : route.fulfill({ json: { items: [] } }),
  );
  await page.goto("/app/projects/A/work");
  const message = page.getByRole("textbox", {
    name: "Message to your agent",
    exact: true,
  });
  await message.fill("Keep this draft while refreshing workers.");
  await expect(page.getByRole("alert")).toBeVisible();
  offline = false;
  await page
    .getByRole("alert")
    .getByRole("button", { name: "Try again", exact: true })
    .click();
  await expect(page.getByRole("alert")).toHaveCount(0);
  await expect(message).toHaveValue(
    "Keep this draft while refreshing workers.",
  );
  expect(fixture.posts).toEqual([]);
});

test("worker catalog supplies models and restores the worker default", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  await page.goto("/app/projects/A/work");
  const model = page.getByLabel("Model", { exact: true });
  await expect(model).toBeEnabled();
  await expect(model.locator("option")).toHaveText([
    "worker-model (worker default)",
    "worker-model · Codex",
    "chosen-model · Claude",
  ]);
  await model.selectOption(JSON.stringify(["claude", "chosen-model"]));
  await model.selectOption("");
  await page
    .getByLabel("Message to your agent")
    .fill("Use the worker configuration.");
  await page.getByRole("button", { name: "Start work", exact: true }).click();
  await expect(page).toHaveURL(/work=work_one/);
  expect(
    f.posts.find((p) => p.path === "/ui-api/work-items")?.body,
  ).toMatchObject({
    workspace_id: "ws_one",
    provider: "codex",
    model: "worker-model",
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

test("catalog failure keeps the message draft and offers a catalog retry", async ({
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
  await page.getByLabel("Message to your agent").fill("Keep my draft.");
  await expect(page.getByLabel("Model", { exact: true })).toBeDisabled();
  unavailable = false;
  await page.getByRole("button", { name: "Retry loading models" }).click();
  await expect(page.getByLabel("Model", { exact: true })).toBeEnabled();
  await expect(page.getByLabel("Message to your agent")).toHaveValue(
    "Keep my draft.",
  );
  expect(f.posts).toEqual([]);
});

test("retry is inside the task error and send has a visible label", async ({
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
  ).toHaveText("Send");
  await expect(retry).toHaveAttribute("data-variant", "outline");
  await expect(retry).toHaveCSS("border-top-width", "1px");
  for (const width of [1600, 390]) {
    await page.setViewportSize({ width, height: 900 });
    const collapse = page.getByRole("button", {
      name: "Collapse preview",
      exact: true,
    });
    if (await collapse.isVisible()) await collapse.click();
    if (width === 1600) {
      const rail = (await page
        .locator(".conversation-thread-rail")
        .boundingBox())!;
      const newThread = (await page
        .getByRole("button", { name: "New thread", exact: true })
        .boundingBox())!;
      expect(newThread.y - rail.y).toBeGreaterThanOrEqual(16);
      expect(newThread.x - rail.x).toBeGreaterThanOrEqual(12);
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
      expect(send.y).toBeGreaterThanOrEqual(field.y + field.height);
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

test("workspace actions use reply data and link to the assigned worker", async ({
  page,
}) => {
  const f = await conversationFixture(page);
  f.state.exists = true;
  f.state.reply = "Saved [measurements](outputs/measurements.csv).";
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
  await page
    .getByRole("button", { name: "outputs/measurements.csv", exact: true })
    .click();
  await expect(
    page.getByRole("region", { name: "Linked file preview" }).locator("pre"),
  ).toContainText("baseline,72");
  await expect(
    page.getByRole("link", { name: "Download", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Close", exact: true })
    .click();
  await conversationTool(page, "Visualize data");
  await expect(
    page.getByRole("img", { name: "score by experiment" }),
  ).toBeVisible();
  await expect(
    page.getByRole("dialog", { name: "Visualize data" }),
  ).toHaveCount(0);
  await conversationTool(page, "Visualize data");
  await expect(
    page.getByRole("img", { name: "score by experiment" }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Task settings", exact: true })
    .click();
  await page
    .getByRole("link", { name: "Worker: Research worker", exact: true })
    .click();
  await expect(page).toHaveURL(/workspaces\?worker=ws_one/);
  await expect(page.getByRole("table", { name: "Workers" })).toContainText(
    "Research worker",
  );
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
    .locator(".conversation-heading-actions")
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

test("workspace keeps the composer visible and alerts on new replies without interrupting direction", async ({
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
  await expect(
    page.getByRole("button", { name: "View message", exact: true }),
  ).toHaveCount(0);
  await composer.focus();
  f.finish();
  const alert = page.locator(".thread-message-notice");
  await expect(alert).toContainText("New agent message", { timeout: 10000 });
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
  await page.getByRole("button", { name: "View message", exact: true }).click();
  await expect(page.getByText(f.state.reply, { exact: true })).toBeVisible();
  await expect(alert).toHaveCount(0);
  await expect(composer).toHaveValue("Make the welcome page easier to scan.");
  await page.getByRole("button", { name: "Workspace", exact: true }).click();
  await expect(alert).toHaveCount(0);
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
  await expect(
    page.getByRole("button", { name: "View message", exact: true }),
  ).toHaveCount(0);
  expect(f.errors).toEqual([]);
});
