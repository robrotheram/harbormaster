import { test, expect, type Page } from "@playwright/test";

async function fixture(page: Page) {
  const worker = {
    id: "ws_readiness",
    project_id: "factory",
    name: "Implementation worker",
    agent_kind: "codex",
    state: "STOPPED",
    ready: false,
    active_runs: 0,
    background_jobs: 0,
    cpu_limit: 1,
    memory_limit_bytes: 1073741824,
    runtime_type: "firecracker",
    isolation_status: "unverified",
    network_status: "unverified",
    created_at: "2026-11-11T12:00:00Z",
    updated_at: "2026-11-11T12:00:00Z",
  };
  const state = {
    worker,
    canStart: false,
    failCheck: false,
    failStart: false,
    startPending: false,
    checks: 0,
    starts: [] as { idempotency_key: string }[],
  };
  await page.route("**/ui-api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/ui-api/projects")
      return route.fulfill({
        json: {
          items: [
            {
              id: "factory",
              name: "Factory project",
              created_at: worker.created_at,
            },
          ],
        },
      });
    if (path === "/ui-api/workspaces")
      return route.fulfill({ json: { items: [worker] } });
    if (path.endsWith("/start-readiness")) {
      state.checks++;
      if (state.failCheck)
        return route.fulfill({
          status: 503,
          json: { code: "RUNTIME_UNAVAILABLE" },
        });
      return route.fulfill({
        json: {
          workspace_id: worker.id,
          project_id: worker.project_id,
          checked_at: worker.created_at,
          can_start: state.canStart,
          checks: [
            {
              id: "runtime",
              label: "Runtime compatibility",
              status: state.canStart ? "passed" : "blocked",
              code: state.canStart
                ? undefined
                : "RUNTIME_CAPABILITIES_INCOMPATIBLE",
              message: state.canStart
                ? "The controller reports a supported capability format."
                : "The runtime controller uses an incompatible capability format. Update the host controller to match the control plane, then check again.",
            },
          ],
        },
      });
    }
    if (path.endsWith("/start")) {
      state.starts.push(route.request().postDataJSON());
      if (state.failStart)
        return route.fulfill({
          status: 503,
          json: { code: "RUNTIME_UNAVAILABLE" },
        });
      if (state.startPending) {
        worker.state = "STARTING";
        return route.fulfill({ status: 503, json: { code: "NOT_READY" } });
      }
      worker.state = "READY";
      worker.ready = true;
      return route.fulfill({ json: worker });
    }
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto("/app/projects/factory/workspaces");
  await expect(page.getByText(worker.name, { exact: true })).toBeVisible();
  return state;
}

test("worker startup is read-only until checks pass and the user starts it", async ({
  page,
}) => {
  const state = await fixture(page);
  await expect(page.getByLabel("Current project", { exact: true })).toHaveCount(
    0,
  );
  await expect(page.getByText("Unknown /", { exact: false })).toHaveCount(0);
  const firstCell = page.locator('[data-workspace="ws_readiness"] td').first();
  expect(
    await firstCell.evaluate((el) =>
      parseFloat(getComputedStyle(el).paddingLeft),
    ),
  ).toBe(20);
  const action = page.getByRole("link", {
    name: /^Open worker /,
    exact: true,
  });
  expect((await action.boundingBox())!.width).toBeLessThan(150);
  await action.click();
  await page
    .getByRole("button", { name: "Check & start", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("Action needed before starting");
  await expect(dialog).toContainText("Update the host controller");
  await expect(
    dialog.getByRole("link", { name: "Platform services" }),
  ).toHaveAttribute("href", "/app/platform-services");
  await expect(
    dialog.getByRole("button", { name: "Start worker", exact: true }),
  ).toBeDisabled();
  expect(state.starts).toHaveLength(0);
  await page.screenshot({ path: "artifacts/worker-start-blocked.png" });
  state.canStart = true;
  await dialog
    .getByRole("button", { name: "Check again", exact: true })
    .click();
  await expect(dialog).toContainText("Startup checks passed");
  expect(state.starts).toHaveLength(0);
  await dialog
    .getByRole("button", { name: "Start worker", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Run task", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Stop worker", exact: true }),
  ).toBeEnabled();
  expect(state.starts).toHaveLength(1);
  expect(state.starts[0].idempotency_key).toBeTruthy();
});

test("a slow start follows worker readiness without another start or a reload", async ({
  page,
}) => {
  await page.clock.install();
  const state = await fixture(page);
  state.canStart = true;
  state.startPending = true;
  await page.getByRole("link", { name: /^Open worker / }).click();
  await page
    .getByRole("button", { name: "Check & start", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("Startup checks passed");
  await dialog
    .getByRole("button", { name: "Start worker", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Starting…", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole("region", { name: "Worker terminal" }),
  ).toHaveText("Waiting for the worker to become ready…");
  await expect(page.getByRole("alert")).toHaveCount(0);

  state.worker.state = "READY";
  state.worker.ready = true;
  await page.clock.fastForward(10000);
  await expect(
    page.getByRole("button", { name: "Stop worker", exact: true }),
  ).toBeEnabled();
  expect(state.starts).toHaveLength(1);
  expect(state.checks).toBe(1);
});

test("unavailable checks fail closed and failed starts can be retried safely", async ({
  page,
}) => {
  const state = await fixture(page);
  state.failCheck = true;
  await page.getByRole("link", { name: /^Open worker / }).click();
  await page
    .getByRole("button", { name: "Check & start", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("Startup check unavailable");
  await expect(
    dialog.getByRole("button", { name: "Start worker", exact: true }),
  ).toBeDisabled();
  expect(state.starts).toHaveLength(0);
  state.failCheck = false;
  state.canStart = true;
  state.failStart = true;
  await dialog
    .getByRole("button", { name: "Check again", exact: true })
    .click();
  await expect(dialog).toContainText("Startup checks passed");
  await dialog
    .getByRole("button", { name: "Start worker", exact: true })
    .click();
  await expect(dialog.getByRole("alert")).toContainText(
    "runtime could not complete",
  );
  await expect(
    dialog.getByRole("button", { name: "Start worker", exact: true }),
  ).toBeEnabled();
  state.failStart = false;
  await dialog
    .getByRole("button", { name: "Start worker", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  expect(state.starts).toHaveLength(2);
  expect(state.starts[0].idempotency_key).toBe(state.starts[1].idempotency_key);
});

test("startup review fits mobile and cancellation never starts a worker", async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 740 });
  const state = await fixture(page);
  await page.getByRole("link", { name: /^Open worker / }).click();
  await page
    .getByRole("button", { name: "Check & start", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("Action needed before starting");
  expect(await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(
    true,
  );
  await page.screenshot({ path: "artifacts/worker-start-mobile.png" });
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(state.starts).toHaveLength(0);
});

test("worker terminal renders output, sends input, resizes, and closes its session", async ({
  page,
  context,
}) => {
  const state = await fixture(page);
  state.canStart = true;
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  const exchanges: { input: string; cols: number; rows: number }[] = [];
  let closes = 0;
  let opened = 0;
  await page.route("**/assets/ghostty-vt-*.wasm", (route) => route.abort(), {
    times: 1,
  });
  await page.route(
    "**/ui-api/workspaces/ws_readiness/terminals**",
    async (route) => {
      const path = new URL(route.request().url()).pathname;
      const body = route.request().postDataJSON();
      expect(route.request().headers()["x-workbench-request"]).toBe("1");
      if (path.endsWith("/close")) {
        closes++;
        return route.fulfill({ json: { closed: true } });
      }
      if (path.endsWith("/exchange")) {
        exchanges.push(body);
        const output = body.offset
          ? ""
          : "\x1b[32mworker shell ready\x1b[0m\r\n<script>window.shellExecuted=true</script>\r\n$ ";
        return route.fulfill({
          json: {
            output: Buffer.from(output).toString("base64"),
            offset: 100,
            exited: false,
            truncated: !body.offset,
          },
        });
      }
      opened++;
      return route.fulfill({ status: 201, json: { id: body.id } });
    },
  );
  await page.getByRole("link", { name: /^Open worker / }).click();
  await page
    .getByRole("button", { name: "Check & start", exact: true })
    .click();
  await page.getByRole("button", { name: "Start worker", exact: true }).click();
  const workerHeading = page.locator("main h1");
  const headingLeft = (await workerHeading.boundingBox())!.x;
  const dialog = page.getByRole("region", { name: "Worker terminal" });
  await expect(dialog.getByRole("status")).toHaveText("Disconnected");
  expect(opened).toBe(0);
  await dialog.getByRole("button", { name: "New shell" }).click();
  await expect(dialog.getByRole("status")).toHaveText("Connected");
  const surface = dialog.locator('[contenteditable="true"]');
  await expect(surface).toBeFocused();
  await expect(surface).toHaveCSS("caret-color", "rgba(0, 0, 0, 0)");
  await expect(
    dialog.getByRole("button", { name: "Close terminal" }),
  ).toHaveCount(0);
  expect((await workerHeading.boundingBox())!.x).toBeCloseTo(headingLeft, 0);
  await expect.poll(() => exchanges.at(-1)?.input).toBe("");
  const canvas = dialog.locator("canvas");
  await expect(canvas).toBeVisible();
  const rowHeight = (await canvas.boundingBox())!.height / exchanges[0].rows;
  await expect(async () => {
    await canvas.dblclick({ position: { x: 12, y: rowHeight * 1.5 } });
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
      "worker",
    );
  }).toPass();
  await page.keyboard.type("pwd");
  await page.keyboard.press("Enter");
  await page.keyboard.press("Control+c");
  await page.keyboard.press("Tab");
  await page.keyboard.press("Escape");
  await page.keyboard.press("Control+v");
  await expect(dialog).toBeVisible();
  await expect
    .poll(() => exchanges.map((e) => e.input).join(""))
    .toContain("pwd\r\x03\t\x1bworker");
  await expect.poll(() => exchanges.length).toBeGreaterThan(0);
  const initialCols = exchanges.at(-1)!.cols;
  await page.setViewportSize({ width: 390, height: 844 });
  await expect.poll(() => exchanges.at(-1)!.cols).toBeLessThan(initialCols);
  expect(
    await page.evaluate(
      () => (window as unknown as { shellExecuted?: boolean }).shellExecuted,
    ),
  ).toBeUndefined();
  await page
    .getByRole("navigation", { name: "Worker sections" })
    .getByRole("link", { name: "Settings", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  await expect.poll(() => closes).toBeGreaterThan(0);
  expect(opened).toBe(1);
});
