import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

async function fixture(page: Page) {
  const workers = ["Stopped worker", "Source worker", "Destination worker"].map(
    (name, i) => ({
      id: `ws_${i}`,
      project_id: "audit",
      name,
      agent_kind: "codex",
      state: i ? "READY" : "STOPPED",
      ready: !!i,
      active_runs: 0,
      background_jobs: 0,
      runtime_type: "firecracker",
      cpu_limit: 1,
      memory_limit_bytes: 1073741824,
      connection_profile_id: "team",
      connection_profile_revision: 1,
      connections: { mode: "blocked", destinations: [] },
      idle_timeout_minutes: 60,
      created_at: "2026-11-14T12:00:00Z",
      updated_at: "2026-11-14T12:00:00Z",
    }),
  );
  const state = {
    profileError: false,
    writes: [] as { path: string; body: Record<string, unknown> }[],
  };
  await page.route("**/ui-api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (route.request().method() === "POST") {
      if (path.endsWith("/terminals"))
        return route.fulfill({ status: 403, json: { code: "FORBIDDEN" } });
      state.writes.push({ path, body: route.request().postDataJSON() });
      return route.fulfill({
        status: 503,
        json: { code: "RUNTIME_UNAVAILABLE" },
      });
    }
    if (path === "/ui-api/projects")
      return route.fulfill({
        json: { items: [{ id: "audit", name: "Customer operations" }] },
      });
    if (path === "/ui-api/workspaces")
      return route.fulfill({ json: { items: workers } });
    if (path === "/ui-api/connection-profiles")
      return route.fulfill(
        state.profileError
          ? { status: 503, json: { code: "STORAGE_UNAVAILABLE" } }
          : {
              json: {
                items: [
                  {
                    id: "team",
                    name: "Team profile",
                    revision: 1,
                    policy: { mode: "blocked", destinations: [] },
                  },
                ],
              },
            },
      );
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto("/app/projects/audit/workspaces");
  await expect(page.getByText("Stopped worker", { exact: true })).toBeVisible();
  return state;
}
async function action(page: Page, name: string, worker = "Stopped worker") {
  await page
    .getByRole("link", { name: `Open worker ${worker}`, exact: true })
    .click();
  await page
    .getByRole("navigation", { name: "Worker sections" })
    .getByRole("link", { name: "Settings", exact: true })
    .click();
  if (name !== "Settings")
    await page.getByRole("button", { name, exact: true }).click();
}

test("worker creation guards empty names and preserves its draft during pending failure", async ({
  page,
}) => {
  const state = await fixture(page);
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/ui-api/workspaces", async (route) => {
    if (route.request().method() === "POST") await gate;
    await route.fallback();
  });
  await page
    .getByRole("button", { name: "Create worker", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Create worker",
    exact: true,
  });
  const input = dialog.getByRole("textbox", {
    name: "Worker name",
    exact: true,
  });
  await input.fill("   ");
  await expect(
    dialog.getByRole("button", { name: "Create worker", exact: true }),
  ).toBeDisabled();
  await input.press("Enter");
  expect(state.writes).toHaveLength(0);
  await input.fill("Support agent");
  await input.press("Enter");
  await expect(input).toBeDisabled();
  await expect(
    dialog.getByRole("button", { name: "Close", exact: true }),
  ).toBeDisabled();
  await page.keyboard.press("Escape");
  await expect(dialog).toBeVisible();
  release();
  await expect(dialog.getByRole("alert")).toBeVisible();
  await expect(input).toHaveValue("Support agent");
  await expect(input).toBeEnabled();
  expect(state.writes).toHaveLength(1);
});

test("worker settings distinguish failed profile lookup and retry without saving", async ({
  page,
}) => {
  const state = await fixture(page);
  state.profileError = true;
  await page.reload();
  await action(page, "Settings");
  const dialog = page.getByRole("region", {
    name: "Worker settings",
    exact: true,
  });
  await expect(dialog.getByRole("alert")).toBeVisible();
  const profiles = dialog.getByRole("combobox", {
    name: "Global connection profile",
    exact: true,
  });
  await expect(profiles).toBeDisabled();
  await expect(profiles.locator("option:checked")).toHaveText(
    "Selected profile could not be checked",
  );
  await dialog
    .getByRole("combobox", { name: "Automatic idle shutdown", exact: true })
    .selectOption("30");
  await expect(
    dialog.getByRole("button", { name: "Save settings", exact: true }),
  ).toBeDisabled();
  state.profileError = false;
  await dialog.getByRole("button", { name: "Try again", exact: true }).click();
  await expect(profiles).toBeEnabled();
  await expect(profiles.locator("option:checked")).toHaveText(
    "Team profile · version 1",
  );
  await expect(
    dialog.getByRole("combobox", {
      name: "Automatic idle shutdown",
      exact: true,
    }),
  ).toHaveValue("30");
  expect(state.writes).toHaveLength(0);
});

test("copying a file clears a destination that becomes the source and locks pending fields", async ({
  page,
}) => {
  const state = await fixture(page);
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/ui-api/file-handoffs", async (route) => {
    await gate;
    await route.fallback();
  });
  await page
    .getByRole("button", { name: "Project actions", exact: true })
    .click();
  await page
    .getByRole("menuitem", { name: "Copy a file", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Copy a project file",
    exact: true,
  });
  await dialog
    .getByRole("combobox", { name: "From worker", exact: true })
    .selectOption("ws_1");
  await dialog
    .getByRole("combobox", { name: "To worker", exact: true })
    .selectOption("ws_2");
  await dialog
    .getByRole("textbox", { name: "Source file", exact: true })
    .fill("report.md");
  await dialog
    .getByRole("textbox", { name: "New destination file", exact: true })
    .fill("incoming/report.md");
  await dialog
    .getByRole("combobox", { name: "From worker", exact: true })
    .selectOption("ws_2");
  await expect(
    dialog.getByRole("combobox", { name: "To worker", exact: true }),
  ).toHaveValue("");
  await expect(
    dialog.getByRole("button", { name: "Copy file", exact: true }),
  ).toBeDisabled();
  await dialog
    .getByRole("combobox", { name: "To worker", exact: true })
    .selectOption("ws_1");
  await dialog.getByRole("button", { name: "Copy file", exact: true }).click();
  await expect(
    dialog.getByRole("textbox", { name: "Source file", exact: true }),
  ).toBeDisabled();
  await expect(
    dialog.getByRole("button", { name: "Close", exact: true }),
  ).toBeDisabled();
  release();
  await expect(dialog.getByRole("alert")).toBeVisible();
  await expect(
    dialog.getByRole("textbox", { name: "Source file", exact: true }),
  ).toHaveValue("report.md");
  expect(state.writes).toHaveLength(1);
});

test("worker deletion explains running state and retains confirmation on failure", async ({
  page,
}) => {
  const state = await fixture(page);
  await action(page, "Delete worker", "Source worker");
  const dialog = page.getByRole("dialog", {
    name: "Delete worker?",
    exact: true,
  });
  await expect(dialog.getByRole("status")).toContainText(
    "Deleting stops this worker first",
  );
  await expect(
    dialog.getByRole("button", { name: "Delete worker", exact: true }),
  ).toBeDisabled();
  await dialog
    .getByRole("textbox", {
      name: "Type the worker name to confirm",
      exact: true,
    })
    .fill("Source worker");
  await expect(
    dialog.getByRole("button", { name: "Delete worker", exact: true }),
  ).toBeEnabled();
  await page.keyboard.press("Escape");
  await expect(
    page.getByRole("button", {
      name: "Delete worker",
      exact: true,
    }),
  ).toBeFocused();
  await page.goto("/app/projects/audit/workspaces");
  await action(page, "Delete worker");
  await expect(dialog).toContainText("Private disks remain on the host");
  const confirmation = dialog.getByRole("textbox", {
    name: "Type the worker name to confirm",
    exact: true,
  });
  await confirmation.fill("Wrong worker");
  await expect(
    dialog.getByRole("button", { name: "Delete worker", exact: true }),
  ).toBeDisabled();
  expect(state.writes).toHaveLength(0);
  await confirmation.fill("Stopped worker");
  await dialog
    .getByRole("button", { name: "Delete worker", exact: true })
    .click();
  await expect(dialog.getByRole("alert")).toBeVisible();
  await expect(confirmation).toHaveValue("Stopped worker");
  expect(state.writes).toEqual([
    { path: "/ui-api/workspaces/ws_0/delete", body: { confirm_id: "ws_0" } },
  ]);
});

test("worker dialogs remain accessible on narrow screens in both themes", async ({
  page,
}) => {
  test.setTimeout(120000);
  await page.setViewportSize({ width: 320, height: 800 });
  await fixture(page);
  for (const theme of ["dark", "light"]) {
    const toggle = page.getByRole("button", {
      name: `Switch to ${theme} theme`,
      exact: true,
    });
    if (await toggle.count()) await toggle.click();
    for (const name of [
      "Create worker",
      "Settings",
      "Delete worker",
      "Copy a file",
    ]) {
      if (name === "Create worker")
        await page.getByRole("button", { name, exact: true }).click();
      else if (name === "Copy a file") {
        await page
          .getByRole("button", { name: "Project actions", exact: true })
          .click();
        await page.getByRole("menuitem", { name, exact: true }).click();
      } else await action(page, name);
      const dialog =
        name === "Settings"
          ? page.getByRole("region", { name: "Worker settings" })
          : page.getByRole("dialog");
      await expect(dialog).toBeVisible();
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
          `${name} ${theme}`,
        )
        .toEqual([]);
      expect(
        await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth),
      ).toBe(true);
      if (name !== "Settings") {
        await page.keyboard.press("Escape");
        await expect(dialog).toHaveCount(0);
      }
      if (name === "Settings" || name === "Delete worker")
        await page.goto("/app/projects/audit/workspaces");
    }
  }
});
