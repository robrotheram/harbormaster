import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

async function fixture(page: Page) {
  const workers = ["Support worker", "Operations worker"].map((name, i) => ({
    id: `ws_${i}`,
    name,
    project_id: "audit",
    agent_kind: "codex",
    state: "READY",
    ready: true,
    active_runs: 0,
    background_jobs: 0,
    cpu_limit: 1,
    memory_limit_bytes: 1073741824,
    created_at: "2026-11-14T12:00:00Z",
    updated_at: "2026-11-14T12:00:00Z",
  }));
  const state = {
    logsFail: false,
    historyStopped: false,
    exportFail: true,
    incompleteExport: false,
    logReads: 0,
    historyReads: [] as string[],
    writes: [] as string[],
  };
  await page.route("**/ui-api/**", async (route) => {
    const url = new URL(route.request().url()),
      path = url.pathname;
    if (route.request().method() === "POST") {
      state.writes.push(path);
      state.historyStopped = false;
      return route.fulfill({ json: workers[0] });
    }
    if (path === "/ui-api/projects")
      return route.fulfill({
        json: { items: [{ id: "audit", name: "Customer operations" }] },
      });
    if (path === "/ui-api/workspaces")
      return route.fulfill({ json: { items: workers } });
    if (path.endsWith("/logs") || path === "/ui-api/workspace-logs") {
      state.logReads++;
      if (state.logsFail)
        return route.fulfill({
          status: 503,
          json: { code: "STORAGE_UNAVAILABLE" },
        });
      return route.fulfill({
        json: {
          project_id: "audit",
          workspace_id: "ws_0",
          state: "READY",
          active_runs: 1,
          background_jobs: 0,
          items: [
            {
              id: "event1",
              at: "2026-11-14T12:00:00Z",
              source: "codex",
              kind: "task.started",
              task_id: "task_" + "x".repeat(100),
              project_id: "audit",
              workspace_id: "ws_0",
            },
          ],
        },
      });
    }
    if (path.endsWith("/history/export")) {
      if (state.exportFail)
        return route.fulfill({
          status: 503,
          json: { code: "HISTORY_UNAVAILABLE" },
        });
      const records = [
        {
          type: "manifest",
          schema_version: 1,
          project_id: "audit",
          workspace_id: "ws_0",
          high_watermark: 0,
        },
        state.incompleteExport
          ? { type: "export_error", code: "HISTORY_UNAVAILABLE" }
          : {
              type: "export_complete",
              records: 0,
              high_watermark: 0,
              sha256: "fixture",
            },
      ];
      return route.fulfill({
        contentType: "application/x-ndjson",
        body: records.map((r) => JSON.stringify(r)).join("\n") + "\n",
      });
    }
    if (path.endsWith("/history")) {
      state.historyReads.push(url.search);
      if (state.historyStopped)
        return route.fulfill({
          status: 409,
          json: { code: "HISTORY_WORKSPACE_STOPPED" },
        });
      const older = url.searchParams.has("before");
      return route.fulfill({
        json: {
          project_id: "audit",
          workspace_id: path.split("/")[3],
          page: {
            schema_version: 1,
            high_watermark: 40,
            next_before: older ? 0 : 20,
            capture_limited: false,
            bytes_used: 1000,
            records: Array.from({ length: 20 }, (_, i) => ({
              id: `message_${older ? i : i + 20}`,
              seq: older ? i + 1 : i + 21,
              at: "2026-11-14T12:00:00Z",
              source: "codex",
              kind: i % 2 ? "assistant.message" : "user.message",
              purpose: "test",
              content:
                `${older ? "Older" : "Latest"} message ${i} ` + "x".repeat(100),
            })),
          },
        },
      });
    }
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto("/app/projects/audit/workspaces");
  await expect(page.getByText("Support worker", { exact: true })).toBeVisible();
  return state;
}
async function open(page: Page, item: string) {
  await page
    .getByRole("button", { name: "Actions for Support worker", exact: true })
    .click();
  await page.getByRole("menuitem", { name: item, exact: true }).click();
}

test("log failures never claim there is no activity and retry preserves filters", async ({
  page,
}) => {
  const state = await fixture(page);
  state.logsFail = true;
  await open(page, "Activity logs");
  const dialog = page.getByRole("dialog", {
    name: "Worker activity logs",
    exact: true,
  });
  await expect(dialog.getByRole("alert")).toBeVisible();
  await expect(dialog.getByText(/No activity recorded yet/)).toHaveCount(0);
  await dialog
    .getByRole("button", { name: "Pause updates", exact: true })
    .click();
  await dialog
    .getByRole("combobox", { name: "Log source", exact: true })
    .selectOption("codex");
  state.logsFail = false;
  await dialog.getByRole("button", { name: "Try again", exact: true }).click();
  await expect(dialog.getByRole("alert")).toHaveCount(0);
  await expect(dialog.getByText(/Task started/)).toBeVisible();
  await expect(
    dialog.getByRole("combobox", { name: "Log source", exact: true }),
  ).toHaveValue("codex");
  await expect(
    dialog.getByText(/Ready · 1 run · 0 background processes/),
  ).toBeVisible();
  expect(state.writes).toHaveLength(0);
  state.logsFail = true;
  await dialog.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(dialog.getByRole("alert")).toBeVisible();
  await expect(dialog.getByText(/Last successful update/)).toBeVisible();
  await expect(dialog.getByText(/Ready · 1 run/)).toHaveCount(0);
  await expect(dialog.getByText(/Task started/)).toBeVisible();
});

test("history export errors stay in the app and incomplete downloads are rejected", async ({
  page,
}) => {
  const state = await fixture(page);
  await open(page, "Message history");
  const dialog = page.getByRole("dialog", {
    name: "Message history",
    exact: true,
  });
  await expect(dialog.getByText(/Latest message 0 /)).toBeVisible();
  await dialog
    .getByRole("button", { name: "Export JSONL", exact: true })
    .click();
  await expect(dialog.getByRole("alert")).toBeVisible();
  await expect(page).toHaveURL(/\/app\/projects\/audit\/workspaces$/);
  await expect(dialog.getByText(/Latest message 0 /)).toBeVisible();
  state.exportFail = false;
  state.incompleteExport = true;
  const downloads: string[] = [];
  page.on("download", (download) =>
    downloads.push(download.suggestedFilename()),
  );
  await dialog
    .getByRole("button", { name: "Export JSONL", exact: true })
    .click();
  await expect(dialog.getByRole("alert")).toContainText(
    "No file was downloaded",
  );
  expect(downloads).toEqual([]);
  state.incompleteExport = false;
  const download = page.waitForEvent("download");
  await dialog
    .getByRole("button", { name: "Export JSONL", exact: true })
    .click();
  expect((await download).suggestedFilename()).toBe("ws_0-history.jsonl");
  await expect(dialog.getByRole("alert")).toHaveCount(0);
  await expect(dialog.getByRole("status")).toContainText(
    "Check your browser downloads",
  );
});

test("newest history resets the cursor without refetching the old page", async ({
  page,
}) => {
  const state = await fixture(page);
  await open(page, "Message history");
  const dialog = page.getByRole("dialog", {
    name: "Message history",
    exact: true,
  });
  await expect(dialog.getByText(/Latest message 0 /)).toBeVisible();
  await dialog.getByRole("button", { name: "Older", exact: true }).click();
  await expect(dialog.getByText(/Older message 0 /)).toBeVisible();
  state.historyReads = [];
  await dialog.getByRole("button", { name: "Newest", exact: true }).click();
  await expect(dialog.getByText(/Latest message 0 /)).toBeVisible();
  expect(state.historyReads.length).toBeGreaterThan(0);
  expect(state.historyReads.every((query) => !query.includes("before="))).toBe(
    true,
  );
});

test("starting from history locks worker selection until recovery completes", async ({
  page,
}) => {
  const state = await fixture(page);
  state.historyStopped = true;
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/ui-api/workspaces/ws_0/start", async (route) => {
    await gate;
    await route.fallback();
  });
  await open(page, "Message history");
  const dialog = page.getByRole("dialog", {
    name: "Message history",
    exact: true,
  });
  await dialog
    .getByRole("button", { name: "Start worker to view messages", exact: true })
    .click();
  await expect(
    dialog.getByRole("combobox", { name: "History worker", exact: true }),
  ).toBeDisabled();
  await expect(
    dialog.getByRole("button", { name: "Close", exact: true }),
  ).toBeDisabled();
  release();
  await expect(dialog.getByText(/Latest message 0 /)).toBeVisible();
  await expect(
    dialog.getByRole("combobox", { name: "History worker", exact: true }),
  ).toBeEnabled();
  expect(state.writes).toEqual(["/ui-api/workspaces/ws_0/start"]);
});

test("history and activity dialogs remain accessible at phone widths", async ({
  page,
}) => {
  test.setTimeout(90000);
  await page.setViewportSize({ width: 320, height: 800 });
  await fixture(page);
  for (const theme of ["dark", "light"]) {
    const toggle = page.getByRole("button", {
      name: `Switch to ${theme} theme`,
      exact: true,
    });
    if (await toggle.count()) await toggle.click();
    for (const item of ["Message history", "Activity logs"]) {
      await open(page, item);
      const dialog = page.getByRole("dialog");
      await expect(
        dialog.getByText(
          item === "Message history" ? /Latest message 0 / : "Task started",
          { exact: false },
        ),
      ).toBeVisible();
      const workerSelector = dialog.getByRole("combobox", {
        name:
          item === "Message history" ? "History worker" : "Worker log filter",
        exact: true,
      });
      expect(
        (await workerSelector.boundingBox())!.width,
      ).toBeGreaterThanOrEqual(200);
      if (item === "Message history") {
        await dialog.getByRole("button", { name: "Export JSONL" }).hover();
      }
      await page.waitForFunction(
        () =>
          !document
            .getAnimations()
            .some(
              (a) => a instanceof CSSTransition && a.playState === "running",
            ),
        undefined,
        { timeout: 5000 },
      );
      const result = await new AxeBuilder({ page })
        .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
        .analyze();
      expect
        .soft(
          result.violations.map((v) => ({
            id: v.id,
            nodes: v.nodes.map((n) => ({
              target: n.target,
              summary: n.failureSummary,
            })),
          })),
          `${item} ${theme}`,
        )
        .toEqual([]);
      expect(
        await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth),
      ).toBe(true);
      await page.keyboard.press("Escape");
      await expect(dialog).toHaveCount(0);
    }
  }
});

test("failed clipboard access explains how to copy a pairing link manually", async ({
  page,
}) => {
  const id = "ws_" + "a".repeat(32);
  await page.addInitScript(() =>
    Object.defineProperty(navigator, "clipboard", {
      value: {
        writeText: async () => {
          throw new DOMException("Denied", "NotAllowedError");
        },
      },
    }),
  );
  await page.route("**/ui-api/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/projects"))
      return route.fulfill({
        json: { items: [{ id: "audit", name: "Customer operations" }] },
      });
    if (path.endsWith("/workspaces"))
      return route.fulfill({
        json: {
          items: [
            {
              id,
              project_id: "audit",
              name: "Desktop worker",
              agent_kind: "t3",
              ready: true,
              state: "READY",
              active_runs: 0,
              background_jobs: 0,
            },
          ],
        },
      });
    if (path.endsWith("/desktop-pair"))
      return route.fulfill({
        json: {
          pairing_url: `http://desktop-ws-${"a".repeat(32)}.localhost:8080/pair#token=synthetic-test`,
          expires_at: "2030-01-01T00:00:00Z",
        },
      });
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto("/app/projects/audit/workspaces");
  await page
    .getByRole("button", { name: "Connect T3 desktop", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Connect T3 desktop",
    exact: true,
  });
  await dialog
    .getByRole("button", { name: "Copy pairing URL", exact: true })
    .click();
  await expect(dialog.getByRole("alert")).toContainText("copy it manually");
  await expect(
    dialog.getByRole("textbox", { name: "Private pairing URL", exact: true }),
  ).toHaveValue(/synthetic-test/);
});

test("closing a pending history export cancels the read without downloading", async ({
  page,
}) => {
  const state = await fixture(page);
  state.exportFail = false;
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route(
    "**/ui-api/workspaces/ws_0/history/export",
    async (route) => {
      await gate;
      await route.fallback();
    },
  );
  const downloads: string[] = [];
  page.on("download", (download) =>
    downloads.push(download.suggestedFilename()),
  );
  await open(page, "Message history");
  const dialog = page.getByRole("dialog", {
    name: "Message history",
    exact: true,
  });
  await expect(dialog.getByText(/Latest message 0 /)).toBeVisible();
  await dialog
    .getByRole("button", { name: "Export JSONL", exact: true })
    .click();
  await expect(
    dialog.getByRole("button", { name: "Preparing export…", exact: true }),
  ).toBeDisabled();
  const failure = page.waitForEvent("requestfailed", (request) =>
    request.url().endsWith("/history/export"),
  );
  await page.keyboard.press("Escape");
  release();
  await failure;
  await expect(dialog).toHaveCount(0);
  expect(downloads).toEqual([]);
});
