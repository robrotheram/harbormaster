import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

async function fixture(page: Page, connected = false, workspaceOnly = false) {
  const config = {
    project_id: "hermes_project",
    provider: "openai-codex",
    model: "approved-model",
    mode: workspaceOnly ? "workspace" : "both",
    connection_profile_id: "",
    matrix: {
      homeserver: "https://matrix.example.org",
      bot_user_id: "@factory:example.org",
      room_ids: ["!room:example.org"],
      sender_ids: ["@operator:example.org"],
    },
    revision: 3,
    updated_at: null,
  };
  const snapshot = {
    config,
    saved: true,
    runtime_available: true,
    runtime_status: "available",
    matrix_available: true,
    workspace_ready: true,
    workspace_id: "ws_hermes",
    matrix_state: connected ? "connected" : "disconnected",
    matrix_credential_configured: connected,
  };
  const posts: { path: string; body: Record<string, unknown> }[] = [];
  await page.route("**/ui-api/**", async (route) => {
    const request = route.request(),
      path = new URL(request.url()).pathname;
    if (request.method() === "POST") {
      const body = request.postDataJSON();
      posts.push({ path, body });
      if (path.endsWith("/discover"))
        return route.fulfill({ json: { homeserver: body.homeserver } });
      if (path.endsWith("/matrix-credential")) {
        snapshot.matrix_credential_configured = true;
        return route.fulfill({ json: { credential_configured: true } });
      }
      if (path.endsWith("/matrix/start")) {
        snapshot.matrix_state = "connected";
        return route.fulfill({ json: { state: "connected" } });
      }
      if (path.endsWith("/matrix/stop")) {
        snapshot.matrix_state = "disconnected";
        return route.fulfill({ json: { state: "disconnected" } });
      }
      if (path.endsWith("/hermes"))
        Object.assign(config, body, { revision: config.revision + 1 });
    }
    if (path === "/ui-api/projects")
      return route.fulfill({
        json: {
          items: [
            {
              id: config.project_id,
              name: "Orchestration development",
              created_at: "2026-11-12T09:00:00Z",
            },
          ],
        },
      });
    if (path.endsWith("/hermes")) return route.fulfill({ json: snapshot });
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto("/app/projects/hermes_project/hermes");
  await expect(
    page.getByRole("heading", { name: "Agent connection", exact: true }),
  ).toBeVisible();
  return { posts, config };
}

async function expectWorkspaceWidth(page: Page) {
  const bounds = await page.evaluate(() => {
    const header = document
      .querySelector(".factory-header")!
      .getBoundingClientRect();
    const setup = document
      .querySelector(".hermes-setup")!
      .getBoundingClientRect();
    const panel = document
      .querySelector('[role="tabpanel"][data-state="active"]')!
      .getBoundingClientRect();
    return {
      expectedGutter: Math.max(0, (header.width - 1800) / 2) + 24,
      leftGutter: setup.left - header.left,
      rightGutter: header.right - setup.right,
      panelLeft: panel.left - setup.left,
      panelRight: setup.right - panel.right,
      overflow: document.documentElement.scrollWidth > innerWidth,
    };
  });
  expect(bounds.leftGutter).toBe(bounds.expectedGutter);
  expect(bounds.rightGutter).toBe(bounds.expectedGutter);
  expect(bounds.panelLeft).toBe(1);
  expect(bounds.panelRight).toBe(1);
  expect(bounds.overflow).toBe(false);
}

test("Hermes settings use standard page gutters on both tabs and wide screens", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  const { posts } = await fixture(page, true);
  const panel = page.getByRole("tabpanel");
  await expectWorkspaceWidth(page);
  await expect(
    panel.getByRole("heading", { name: "Workspace agent", exact: true }),
  ).toBeVisible();
  await expect(
    panel.getByRole("link", { name: "Open Hermes workspace", exact: true }),
  ).toBeVisible();
  await expect(page.locator("main aside")).toHaveCount(0);
  await page.screenshot({
    path: "artifacts/hermes-agent-desktop.png",
    animations: "disabled",
    fullPage: true,
  });
  await page.getByRole("tab", { name: "Agent", exact: true }).focus();
  await page.keyboard.press("ArrowRight");
  await expect(
    page.getByRole("tab", { name: "Matrix", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  const labels = [
    "Homeserver URL",
    "Bot user ID",
    "Allowed room IDs",
    "Allowed sender IDs",
  ];
  let lastBottom = 0;
  let left = 0;
  for (const label of labels) {
    const box = (await page.getByLabel(label, { exact: true }).boundingBox())!;
    if (left) expect(box.x).toBe(left);
    expect(box.y).toBeGreaterThan(lastBottom);
    left = box.x;
    lastBottom = box.y + box.height;
  }
  await expect(
    page.getByLabel("Bot access token", { exact: true }),
  ).not.toBeVisible();
  expect(
    (await page.getByRole("region", { name: "Matrix controls" }).boundingBox())!
      .y,
  ).toBeGreaterThan(lastBottom);
  await page.screenshot({
    path: "artifacts/hermes-matrix-desktop.png",
    animations: "disabled",
    fullPage: true,
  });
  for (const width of [1440, 2000, 2560]) {
    await page.setViewportSize({ width, height: 1173 });
    for (const collapsed of [false, true]) {
      const toggle = page.getByRole("button", {
        name: collapsed ? "Collapse sidebar" : "Expand sidebar",
        exact: true,
      });
      if (await toggle.isVisible()) await toggle.click();
      for (const name of ["Agent", "Matrix"]) {
        await page.getByRole("tab", { name, exact: true }).click();
        await expectWorkspaceWidth(page);
        if (width === 2000)
          await page.screenshot({
            path: `artifacts/hermes-${name.toLowerCase()}-wide-${collapsed ? "collapsed" : "expanded"}.png`,
            animations: "disabled",
            fullPage: true,
          });
      }
    }
  }
  await page.setViewportSize({ width: 320, height: 840 });
  await expect(
    page.getByLabel("Homeserver URL", { exact: true }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: "artifacts/hermes-matrix-mobile.png",
    animations: "disabled",
    fullPage: true,
  });
  expect(posts).toHaveLength(0);
});

test("tab switching preserves edits and credential saving remains explicit", async ({
  page,
}) => {
  const { posts } = await fixture(page);
  await page.getByLabel("Model", { exact: true }).fill("updated-model");
  await page.getByRole("tab", { name: "Matrix", exact: true }).click();
  await page
    .getByLabel("Allowed room IDs", { exact: true })
    .fill("!second:example.org");
  await page
    .getByLabel("Bot access token", { exact: true })
    .fill("synthetic-test-token");
  await expect(
    page.getByLabel("Bot access token", { exact: true }),
  ).toHaveAttribute("type", "password");
  await expect(
    page.getByRole("button", { name: "Save bot credential", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole("button", { name: "Connect Matrix", exact: true }),
  ).toBeDisabled();
  await page.getByRole("tab", { name: "Agent", exact: true }).click();
  await expect(page.getByLabel("Model", { exact: true })).toHaveValue(
    "updated-model",
  );
  await page.getByRole("tab", { name: "Matrix", exact: true }).click();
  await expect(
    page.getByLabel("Allowed room IDs", { exact: true }),
  ).toHaveValue("!second:example.org");
  expect(posts).toHaveLength(0);
  await page
    .getByRole("button", { name: "Save settings", exact: true })
    .click();
  await expect(
    page.getByText("Hermes configuration saved for this project.", {
      exact: true,
    }),
  ).toBeVisible();
  const saved = posts.find((p) => p.path.endsWith("/hermes"))!;
  expect(saved.body).toMatchObject({
    model: "updated-model",
    matrix: { room_ids: ["!second:example.org"] },
  });
  expect(JSON.stringify(saved.body)).not.toContain("synthetic-test-token");
  await page
    .getByRole("button", { name: "Save bot credential", exact: true })
    .click();
  await expect(
    page.getByText("Bot credential stored · replace credential", {
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    page.getByLabel("Bot access token", { exact: true }),
  ).toHaveValue("");
  expect(
    posts.find((p) => p.path.endsWith("/matrix-credential"))!.body,
  ).toMatchObject({
    access_token: "synthetic-test-token",
    revision: 4,
    accept_invitations: false,
  });
  expect(posts.filter((p) => p.path.endsWith("/matrix/start"))).toHaveLength(0);
  await page
    .getByRole("button", { name: "Connect Matrix", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Disconnect", exact: true }),
  ).toBeVisible();
  expect(posts.filter((p) => p.path.endsWith("/matrix/start"))).toHaveLength(1);
});

test("a connected bot can still be disconnected in workspace-only mode", async ({
  page,
}) => {
  const { posts } = await fixture(page, true, true);
  await page.getByRole("tab", { name: "Matrix", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Matrix is not enabled", exact: true }),
  ).toBeVisible();
  await expect(page.getByLabel("Homeserver URL", { exact: true })).toHaveCount(
    0,
  );
  await page.getByRole("button", { name: "Disconnect", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Disconnect", exact: true }),
  ).toHaveCount(0);
  expect(posts).toEqual([
    { path: "/ui-api/projects/hermes_project/hermes/matrix/stop", body: {} },
  ]);
});

test("pending credential checks lock related edits without losing the token", async ({
  page,
}) => {
  const { posts } = await fixture(page);
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/hermes/matrix-credential", async (route) => {
    await gate;
    await route.fallback();
  });
  await page.getByRole("tab", { name: "Matrix", exact: true }).click();
  const token = page.getByLabel("Bot access token", { exact: true });
  await token.fill("synthetic-pending-token");
  await page
    .getByRole("button", { name: "Save bot credential", exact: true })
    .click();
  await expect(token).toBeDisabled();
  await expect(token).toHaveValue("synthetic-pending-token");
  await expect(
    page.getByLabel("Homeserver URL", { exact: true }),
  ).toBeDisabled();
  await expect(page.getByRole("checkbox")).toBeDisabled();
  await expect(
    page.getByRole("button", { name: "Connect Matrix", exact: true }),
  ).toBeDisabled();
  await page.getByRole("tab", { name: "Agent", exact: true }).click();
  await expect(page.getByLabel("Model", { exact: true })).toBeDisabled();
  release();
  await expect(page.getByLabel("Model", { exact: true })).toBeEnabled();
  await page.getByRole("tab", { name: "Matrix", exact: true }).click();
  await expect(token).toHaveValue("");
  expect(
    posts.filter((post) => post.path.endsWith("/matrix-credential")),
  ).toHaveLength(1);
  expect(
    posts.filter((post) => post.path.endsWith("/matrix/start")),
  ).toHaveLength(0);
});

test("incomplete Matrix settings explain why save is unavailable across tabs", async ({
  page,
}) => {
  const { posts } = await fixture(page);
  await page.getByRole("tab", { name: "Matrix", exact: true }).click();
  await page.getByLabel("Allowed room IDs", { exact: true }).fill("");
  await page.getByRole("tab", { name: "Agent", exact: true }).click();
  await page.getByLabel("Model", { exact: true }).fill("updated-model");
  await expect(
    page.getByRole("button", {
      name: "Save settings",
      exact: true,
    }),
  ).toBeDisabled();
  await expect(
    page.getByText(/Complete the homeserver, bot user ID/),
  ).toBeVisible();
  await page.getByRole("tab", { name: "Matrix", exact: true }).click();
  await page
    .getByLabel("Allowed room IDs", { exact: true })
    .fill("!room:example.org");
  await expect(
    page.getByRole("button", {
      name: "Save settings",
      exact: true,
    }),
  ).toBeEnabled();
  expect(posts).toEqual([]);
});

test("failed status refresh disables Matrix actions until recovery and preserves edited settings", async ({
  page,
}) => {
  const { posts } = await fixture(page, true);
  let unavailable = true;
  await page.route("**/projects/hermes_project/hermes", async (route) => {
    if (unavailable && route.request().method() === "GET")
      return route.fulfill({
        status: 503,
        json: { code: "RUNTIME_UNAVAILABLE" },
      });
    await route.fallback();
  });
  await page.getByLabel("Model", { exact: true }).fill("unsaved-model");
  await page
    .getByRole("button", { name: "Refresh runtime status", exact: true })
    .click();
  await expect(page.getByRole("alert")).toBeVisible();
  await expect(
    page.getByText("Runtime status unavailable", { exact: true }),
  ).toBeVisible();
  await expect(page.getByText("Unknown", { exact: true })).toHaveCount(2);
  await page.getByRole("tab", { name: "Matrix", exact: true }).click();
  await expect(
    page.getByRole("heading", {
      name: "Connection status unavailable",
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    page.getByText("The bot is listening in your configured rooms."),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Disconnect", exact: true }),
  ).toBeDisabled();
  unavailable = false;
  await page
    .getByRole("alert")
    .getByRole("button", { name: "Try again", exact: true })
    .click();
  await expect(page.getByRole("alert")).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Disconnect", exact: true }),
  ).toBeEnabled();
  await page.getByRole("tab", { name: "Agent", exact: true }).click();
  await expect(page.getByLabel("Model", { exact: true })).toHaveValue(
    "unsaved-model",
  );
  expect(posts).toEqual([]);
});

test("profile lookup failures do not claim a saved profile was removed or submit edits on retry", async ({
  page,
}) => {
  const { posts, config } = await fixture(page);
  config.connection_profile_id = "team";
  let unavailable = true;
  await page.route("**/ui-api/connection-profiles", (route) =>
    unavailable
      ? route.fulfill({ status: 503, json: { code: "STORAGE_UNAVAILABLE" } })
      : route.fulfill({
          json: {
            items: [
              {
                id: "team",
                name: "Team profile",
                policy: { mode: "blocked", destinations: [] },
              },
            ],
          },
        }),
  );
  await page.reload();
  await expect(page.getByRole("alert")).toBeVisible();
  const profile = page.getByRole("combobox", {
    name: "Connection profile",
    exact: true,
  });
  await expect(profile).toBeDisabled();
  await expect(profile.locator("option:checked")).toHaveText(
    "Selected profile could not be checked",
  );
  await page.getByLabel("Model", { exact: true }).fill("unsaved-model");
  unavailable = false;
  await page
    .getByRole("alert")
    .getByRole("button", { name: "Try again", exact: true })
    .click();
  await expect(profile).toBeEnabled();
  await expect(profile.locator("option:checked")).toHaveText("Team profile");
  await expect(page.getByLabel("Model", { exact: true })).toHaveValue(
    "unsaved-model",
  );
  expect(posts).toEqual([]);
});

test("both Hermes tabs remain accessible with unsaved settings at desktop and phone widths", async ({
  page,
}) => {
  test.setTimeout(120000);
  await fixture(page);
  await page.getByLabel("Model", { exact: true }).fill("unsaved-model");
  for (const width of [1440, 320]) {
    await page.setViewportSize({ width, height: 900 });
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
        undefined,
        { timeout: 5000 },
      );
      for (const tab of ["Agent", "Matrix"]) {
        await page.getByRole("tab", { name: tab, exact: true }).click();
        const result = await new AxeBuilder({ page })
          .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
          .analyze();
        expect
          .soft(
            result.violations.map((violation) => ({
              id: violation.id,
              nodes: violation.nodes.map((node) => ({
                target: node.target,
                summary: node.failureSummary,
              })),
            })),
            `${tab}, ${theme}, ${width}px`,
          )
          .toEqual([]);
        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth,
          ),
        ).toBe(true);
      }
    }
  }
});
