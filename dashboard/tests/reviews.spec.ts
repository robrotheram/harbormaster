import { test, expect, type Page } from "@playwright/test";

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
    if (path.endsWith("/changes"))
      return route.fulfill({ json: { files: [] } });
    if (path.endsWith("/files")) return route.fulfill({ json: { paths: [] } });
    if (path.endsWith("/instruction"))
      return route.fulfill({
        json: {
          project_id: project.id,
          work_item_id: work.id,
          instruction: "Make checkout clearer",
        },
      });
    if (path.endsWith("/history")) {
      const task = attempts.find(
        (attempt) => attempt.id === path.split("/").at(-2),
      )?.task_id;
      const old = task === "task_old";
      const content = old
        ? "# Previous delivery\nThe original checkout."
        : '# Checkout delivery\nValidation now explains missing fields.\n\n```workbench-chart\n{"version":1,"type":"bar","title":"Validation results","x":"name","y":["value"],"data":[{"name":"Pass","value":8},{"name":"Fail","value":2}]}\n```';
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
                    episode_id: task,
                    kind: "assistant.message",
                    content,
                  },
                  {
                    id: "edit",
                    source: "codex",
                    at: project.created_at,
                    seq: 1,
                    episode_id: task,
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
  await page.goto(
    `/app/projects/${project.id}/work?work=${work.id}&view=workspace`,
  );
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
  const output = page.getByRole("region", { name: "Agent output" });
  await expect(
    output.getByRole("heading", { name: "Checkout delivery" }),
  ).toHaveCount(0);
  await expect(
    output
      .getByRole("region", { name: "Validation results" })
      .getByRole("img", { name: "value by name", exact: true }),
  ).toBeVisible();
  await expect(
    output.getByRole("heading", { name: "Previous delivery" }),
  ).toHaveCount(0);
  await expect(
    output
      .getByRole("region", { name: "measurements.csv" })
      .getByRole("img", { name: "value by name", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Review evidence" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Visualize data" }),
  ).toHaveCount(0);
  const tools = page.getByRole("group", { name: "Workspace tools" });
  await expect(
    tools.getByRole("button", { name: "Task settings" }),
  ).toBeVisible();
  await page.route(
    "**/ui-api/work-items/work_review/files?project_id=prj_review",
    (route) => route.fulfill({ json: { items: [] } }),
  );
  await tools.getByRole("button", { name: "Files" }).click();
  const files = page.getByRole("region", { name: "Task files" });
  await expect(
    files.getByText("Could not load files from this worker."),
  ).toBeVisible();
  await files.getByRole("button", { name: "fix.patch", exact: true }).click();
  await files.getByRole("button", { name: "Diff", exact: true }).click();
  await expect(
    files
      .getByRole("region", { name: "Git diff" })
      .getByText("clearLabel", { exact: true }),
  ).toBeVisible();
  await files
    .getByRole("button", { name: "Review tools", exact: true })
    .click();
  await page
    .getByRole("menuitem", { name: "Recorded edits", exact: true })
    .click();
  await files.getByRole("button", { name: /Edit 1/ }).click();
  await expect(
    files.getByText("Captured activity for this task", { exact: false }),
  ).toBeVisible();
  await files.getByRole("button", { name: "report.md", exact: true }).click();
  await files.getByRole("button", { name: "Rendered", exact: true }).click();
  await expect(
    files.getByRole("heading", { name: "Verification report" }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => (window as unknown as Record<string, unknown>).reviewInjected,
    ),
  ).toBeUndefined();
  await tools.getByRole("button", { name: "Preview", exact: true }).click();
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

test("plain task replies stay in Conversation while retained charts appear in Output", async ({
  page,
}) => {
  await setup(page, "research", true);
  const output = page.getByRole("region", { name: "Agent output" });
  await expect(
    output.getByRole("region", { name: "measurements.csv" }),
  ).toBeVisible();
  await expect(
    output.getByRole("heading", { name: "Retained task response" }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: /^Conversation/ }).click();
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
  await page.getByRole("tab", { name: "Web", exact: true }).click();
  await page.getByRole("button", { name: "Preview options" }).click();
  await page.getByRole("menuitem", { name: /Preview settings/ }).click();
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
    .getByRole("button", { name: "Restart preview", exact: true })
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

test("Workspace sends feedback against the current attempt", async ({
  page,
}) => {
  const { writes } = await setup(page);
  const send = page.getByRole("button", { name: "Send", exact: true });
  await expect(send).toBeDisabled();
  await page
    .getByRole("textbox", { name: "Message to your agent" })
    .fill("Make the button easier to read.");
  await send.click();
  await expect
    .poll(() => writes[0]?.body.instruction)
    .toBe("Make the button easier to read.");
  expect(writes[0].path).toBe("/ui-api/work-items/work_review/continue");
  expect(writes[0].body.expected_attempt_id).toBe("attempt_new");
});

test("Workspace omits the close action and the old closed badge", async ({
  page,
}) => {
  const { work, attempts, writes } = await setup(page);
  await expect(
    page.getByRole("button", { name: "Approve this revision" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Close task", exact: true }),
  ).toHaveCount(0);
  work.state = "closed";
  attempts[1].review_state = "closed";
  await page.reload();
  const thread = page.getByRole("region", { name: "Work conversation" });
  await expect(
    thread.getByRole("heading", { name: "Make checkout clearer" }),
  ).toBeVisible();
  await expect(thread.getByText(/closed without approval/i)).toHaveCount(0);
  await expect(thread.getByText("Closed", { exact: true })).toHaveCount(0);
  await expect(
    thread.getByRole("button", { name: "Approve this revision" }),
  ).toHaveCount(0);
  expect(writes).toHaveLength(0);
});

async function mockLiveReviewPreview(
  page: Page,
  ready: () => boolean,
  html = "<p>Checkout preview</p>",
) {
  let connections = 0;
  await page.route("**/ui-api/workspaces/ws_review/applications", (route) =>
    route.fulfill({
      json: {
        items: [
          {
            id: "app_preview",
            state: "active",
            ready: ready(),
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
        body: html,
      }),
  );
  return () => connections;
}

test("preview gets a fresh single-use link after reopening or a readiness change", async ({
  page,
}) => {
  await setup(page);
  let ready = true;
  const connections = await mockLiveReviewPreview(page, () => ready);
  const frame = page.getByTitle("Application preview", { exact: true });
  await expect(frame).toHaveAttribute("src", /ticket=\d+$/);
  const first = await frame.getAttribute("src");
  const opened = connections();
  // Polling alone must not reload a working preview.
  await page.waitForResponse((response) =>
    response.url().endsWith("/applications"),
  );
  expect(connections()).toBe(opened);
  ready = false;
  await expect(frame).toHaveCount(0);
  ready = true;
  await expect(frame).toBeVisible({ timeout: 10000 });
  await expect(frame).not.toHaveAttribute("src", first!);
  const second = await frame.getAttribute("src");
  await page.reload();
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

test("preview areas follow scrolling and ship together with their comments in one image", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 960 });
  const { writes } = await setup(page);
  const connections = await mockLiveReviewPreview(
    page,
    () => true,
    `
    <!doctype html><style>
      body { margin: 0; padding: 30px; font: 20px sans-serif; }
      section { width: 300px; height: 120px; background: #ff8844; }
      #second { margin-top: 500px; background: #44bb88; }
    </style>
    <section id="first">Checkout heading</section>
    <section id="second">Order summary</section>
    <script defer src="/.workbench/preview-review.js" data-parent-origin="http://127.0.0.1:4173"></script>
  `,
  );
  await page.route(
    "http://preview-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.localhost:4173/.workbench/preview-review.js",
    (route) =>
      route.fulfill({
        path: "dist/preview-review.js",
        contentType: "application/javascript",
      }),
  );
  const preview = page.getByTitle("Application preview", { exact: true });
  await expect(preview).toBeVisible();
  const toolbar = page.locator(".preview-toolbar");
  await expect(
    toolbar.getByRole("tab", { name: "Web", exact: true }),
  ).toBeVisible();
  await expect(
    toolbar.getByRole("tab", { name: "Desktop", exact: true }),
  ).toHaveCount(0);
  await expect(toolbar.getByLabel("Preview application")).toBeVisible();
  await expect(toolbar.getByRole("button", { name: "Markup" })).toBeVisible();
  const child = preview.contentFrame();
  await expect(child.locator("#first")).toBeVisible();
  const initialConnections = connections();

  async function drawArea(selector: string) {
    await page.getByRole("button", { name: "Markup" }).click();
    const target = child.locator(selector);
    await target.scrollIntoViewIfNeeded();
    const bounds = (await target.boundingBox())!;
    await page.mouse.move(bounds.x + 12, bounds.y + 12);
    await page.mouse.down();
    await page.mouse.move(bounds.x + 175, bounds.y + 78, { steps: 4 });
    await page.mouse.up();
  }

  await drawArea("#first");
  const first = child.locator('[data-review-area="1"]');
  await expect(first).toBeVisible();
  const offset = await first.evaluate((el) => el.getBoundingClientRect().top);
  const scrollBefore = await child
    .locator("body")
    .evaluate(() => window.scrollY);
  const frameBounds = (await preview.boundingBox())!;
  await page.mouse.move(frameBounds.x + 20, frameBounds.y + 20);
  await page.mouse.wheel(0, 60);
  await expect
    .poll(() => child.locator("body").evaluate(() => window.scrollY))
    .toBe(scrollBefore + 60);
  await expect
    .poll(() => first.evaluate((el) => el.getBoundingClientRect().top))
    .toBe(offset - 60);
  await page
    .getByRole("textbox", { name: "Area 1" })
    .fill("Increase the heading contrast.");
  await drawArea("#second");
  await expect(child.locator('[data-review-area="2"]')).toBeVisible();
  const send = page.getByRole("button", { name: "Send", exact: true });
  await expect(send).toBeDisabled();
  await page
    .getByRole("textbox", { name: "Area 2" })
    .fill("Add a total above the summary.");
  expect(connections()).toBe(initialConnections);
  await send.click();
  await expect
    .poll(() => writes.filter(({ path }) => path.endsWith("/continue")).length)
    .toBe(1);
  const revision = writes.find(({ path }) => path.endsWith("/continue"))!;
  expect(revision.body.instruction).toContain("Increase the heading contrast.");
  expect(revision.body.instruction).toContain("Add a total above the summary.");
  expect(revision.body.instruction).not.toContain("CSS pixels");
  expect(revision.body.screenshots).toEqual([
    {
      content: expect.any(String),
      note: "Numbered preview areas and reviewer comments.",
    },
  ]);
  const [image] = revision.body.screenshots as { content: string }[];
  const panels = await page.evaluate(async (content) => {
    const source = new Image();
    source.src = "data:image/jpeg;base64," + content;
    await source.decode();
    const canvas = document.createElement("canvas");
    canvas.width = source.naturalWidth;
    canvas.height = source.naturalHeight;
    const ctx = canvas.getContext("2d")!;
    ctx.drawImage(source, 0, 0);
    const pixels = ctx.getImageData(0, 0, canvas.width, canvas.height).data;
    let first = 0,
      second = 0,
      outline = 0;
    for (let i = 0; i < pixels.length; i += 4) {
      const [red, green, blue] = [pixels[i], pixels[i + 1], pixels[i + 2]];
      if (red > 225 && green > 95 && green < 160 && blue < 95) first++;
      if (red < 100 && green > 150 && blue > 110 && blue < 155) second++;
      if (red < 85 && green < 115 && blue > 165) outline++;
    }
    return { first, second, outline };
  }, image.content);
  expect(panels.first).toBeGreaterThan(1000);
  expect(panels.second).toBeGreaterThan(1000);
  expect(panels.outline).toBeGreaterThan(100);
});

test("screenshot markup and notes survive a failed send and retry", async ({
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
    .getByLabel("Message to your agent", { exact: true })
    .fill("Fix the highlighted layout.");
  const chooser = page.waitForEvent("filechooser");
  await page.getByRole("button", { name: "Attach screenshot" }).click();
  await (
    await chooser
  ).setFiles({
    name: "capture.png",
    mimeType: "image/png",
    buffer: Buffer.from(
      await page.evaluate(() => {
        const canvas = document.createElement("canvas");
        canvas.width = 1280;
        canvas.height = 720;
        const context = canvas.getContext("2d")!;
        context.fillStyle = "#fff";
        context.fillRect(0, 0, canvas.width, canvas.height);
        return canvas.toDataURL("image/png").split(",")[1];
      }),
      "base64",
    ),
  });
  const thumbnail = page.getByAltText("Review screenshot 1");
  await expect(thumbnail).toBeVisible();
  const original = await thumbnail.getAttribute("src");
  const openEditor = page.getByRole("button", { name: "Mark up screenshot 1" });
  await openEditor.click();
  const editor = page.getByRole("dialog", { name: "Mark up screenshot" });
  await expect(editor).toBeVisible();
  await editor
    .getByLabel("Screenshot notes")
    .fill("The button overlaps the title.");
  const canvas = editor.getByLabel("Screenshot drawing layer");
  await expect(canvas).toBeVisible();
  await editor.evaluate((node) =>
    Promise.all(node.getAnimations().map((animation) => animation.finished)),
  );
  const bounds = (await canvas.boundingBox())!;
  async function draw(from: [number, number], to: [number, number]) {
    await page.mouse.move(
      bounds.x + from[0] * bounds.width,
      bounds.y + from[1] * bounds.height,
    );
    await page.mouse.down();
    await page.mouse.move(
      bounds.x + to[0] * bounds.width,
      bounds.y + to[1] * bounds.height,
      { steps: 5 },
    );
    await page.mouse.up();
  }
  await draw([0.15, 0.2], [0.55, 0.65]);
  await editor.getByRole("button", { name: "Arrow", exact: true }).click();
  await draw([0.85, 0.7], [0.56, 0.47]);
  const undo = editor.getByRole("button", { name: "Undo", exact: true });
  const redo = editor.getByRole("button", { name: "Redo", exact: true });
  await undo.click();
  await expect(redo).toBeEnabled();
  await redo.click();
  await expect(redo).toBeDisabled();
  await editor.getByRole("button", { name: "Save markup" }).click();
  await expect(openEditor).toBeFocused();
  await expect(thumbnail).not.toHaveAttribute("src", original!);
  const annotated = (await thumbnail.getAttribute("src"))!;
  await openEditor.click();
  await expect(undo).toBeEnabled();
  await undo.click();
  await editor.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(thumbnail).toHaveAttribute("src", annotated);
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect.poll(() => writes.length).toBe(1);
  expect(writes[0].body.screenshots).toEqual([
    {
      content: annotated.split(",")[1],
      note: "The button overlaps the title.",
    },
  ]);
  const imagePixels = await page.evaluate(async (src) => {
    const image = new Image();
    image.src = src;
    await image.decode();
    const rendered = document.createElement("canvas");
    rendered.width = image.naturalWidth;
    rendered.height = image.naturalHeight;
    const context = rendered.getContext("2d")!;
    context.drawImage(image, 0, 0);
    const hasRedStroke = (x: number, y: number) => {
      const { data } = context.getImageData(x - 10, y - 10, 20, 20);
      return data.some(
        (red, index) => index % 4 === 0 && red > 150 && data[index + 1] < 100,
      );
    };
    return {
      highlight: context.getImageData(450, 300, 1, 1).data[2],
      arrow: hasRedStroke(902, 421),
      outside: Array.from(context.getImageData(30, 30, 1, 1).data),
    };
  }, annotated);
  expect(imagePixels.highlight).toBeLessThan(220);
  expect(imagePixels.arrow).toBe(true);
  expect(imagePixels.outside).toEqual([255, 255, 255, 255]);
  await expect(page.getByAltText("Review screenshot 1")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Send", exact: true }),
  ).toBeEnabled();
  await page.getByRole("button", { name: "Send", exact: true }).click();
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
  await page.getByRole("button", { name: "Files", exact: true }).click();
  await page.getByRole("button", { name: "Review tools", exact: true }).click();
  await page
    .getByRole("menuitem", { name: "Agent review", exact: true })
    .click();
  await page.getByLabel("Agent review focus").fill("Check accessibility");
  await page.getByRole("button", { name: "Start agent review" }).click();
  await expect(
    page.getByRole("link", { name: "Open agent review conversation" }),
  ).toBeVisible();
  expect(requests.map((r) => r.path)).toEqual(["/work-items", "/run"]);
  expect(requests[0].body.instruction).toContain("attempt_new");
  expect(requests[0].body.instruction).toContain("Check accessibility");
  expect(requests[0].body.kind).toBe("research");
  await expect(page.getByRole("region", { name: "Task files" })).toBeVisible();
});
