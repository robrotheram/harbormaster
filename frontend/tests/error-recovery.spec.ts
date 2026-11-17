import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

test("network errors explain recovery and keep support details out of the main message", async ({
  page,
}) => {
  let unavailable = true;
  await page.route("**/ui-api/projects", (route) =>
    unavailable
      ? route.abort("failed")
      : route.fulfill({ json: { items: [] } }),
  );
  await page.goto("/app/");
  await expect(
    page.getByRole("heading", { name: "Projects unavailable", level: 1 }),
  ).toBeVisible();
  await expect(page).toHaveTitle("Projects unavailable · Workbench");
  const alert = page.getByRole("alert");
  await expect(alert).toContainText("Check your connection");
  await expect(alert).not.toContainText("Failed to fetch");
  await expect(alert.getByText("Code: NETWORK_UNAVAILABLE")).not.toBeVisible();
  await alert.getByText("Support details").click();
  await expect(alert.getByText("Code: NETWORK_UNAVAILABLE")).toBeVisible();
  unavailable = false;
  await page.getByRole("button", { name: "Try again" }).click();
  await expect(alert).toHaveCount(0);
  await expect(
    page.getByRole("heading", { name: "Projects", exact: true }),
  ).toBeVisible();
  await expect(page).toHaveTitle("Projects · Workbench");
  await expect(
    page.getByRole("heading", { name: "Projects", exact: true }),
  ).toBeFocused();
});

test("saved theme bootstrap works under production script policy", async ({
  page,
}) => {
  await page.addInitScript(() =>
    localStorage.setItem("workbench.theme", "light"),
  );
  await page.route("**/ui-api/projects", (route) =>
    route.fulfill({ json: { items: [] } }),
  );
  await page.route("**/app/", async (route) => {
    const response = await route.fetch();
    await route.fulfill({
      response,
      headers: {
        ...response.headers(),
        "content-security-policy": "script-src 'self'",
      },
    });
  });
  await page.goto("/app/");
  await expect(page.locator("html")).not.toHaveClass(/dark/);
  await expect(page.locator('script[src="/app/theme.js"]')).toHaveCount(1);
});

test("unknown pages provide an accessible recovery action on phones", async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 700 });
  await page.goto("/app/missing-page");
  await expect(page.getByRole("main")).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Page not found", level: 1 }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Open Projects" }),
  ).toHaveAttribute("href", "/app/");
  await expect(page).toHaveTitle("Page not found · Workbench");
  await expect(
    page.getByRole("heading", { name: "Page not found" }),
  ).toBeFocused();
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth),
  ).toBeLessThanOrEqual(320);
});

for (const scenario of [
  {
    name: "network profiles",
    path: "/app/connection-profiles",
    endpoint: "/ui-api/connection-profiles",
    misleading: "No profiles match your search.",
    recovered:
      "No network profiles yet. Create a profile to reuse network rules across projects.",
  },
  {
    name: "reviews",
    path: "/app/projects/recovery/reviews",
    endpoint: "/ui-api/work-items",
    misleading: "Your review queue is clear",
    recovered: "Your review queue is clear",
  },
]) {
  test(`${scenario.name} distinguishes unavailable data from empty data and retries in place`, async ({
    page,
  }) => {
    let failed = true;
    await page.route("**/ui-api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === "/ui-api/projects")
        return route.fulfill({
          json: { items: [{ id: "recovery", name: "Recovery" }] },
        });
      if (path === scenario.endpoint && failed)
        return route.fulfill({
          status: 503,
          json: { error: "internal upstream message must not be displayed" },
        });
      return route.fulfill({ json: { items: [] } });
    });
    await page.goto(scenario.path);
    const alert = page.getByRole("alert");
    await expect(alert).toBeVisible();
    await expect(alert).not.toContainText("internal upstream");
    await expect(
      page.getByText(scenario.misleading, { exact: true }),
    ).toHaveCount(0);
    failed = false;
    await alert.getByRole("button", { name: "Try again" }).click();
    await expect(alert).toHaveCount(0);
    await expect(
      page.getByText(scenario.recovered, { exact: true }),
    ).toBeVisible();
    await expect(page).toHaveURL(new RegExp(scenario.path + "$"));
  });
}

for (const theme of ["dark", "light"]) {
  for (const width of [320, 1440]) {
    test(`failed page modules recover with clear controls in ${theme} at ${width}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 900 });
      await page.addInitScript(
        (theme) => localStorage.setItem("workbench.theme", theme),
        theme,
      );
      await page.route("**/ui-api/**", (route) => {
        expect(route.request().method()).toBe("GET");
        return route.fulfill({ json: { items: [] } });
      });
      let failed = true;
      await page.route("**/app/assets/projects-*.js", (route) =>
        failed
          ? route.fulfill({
              status: 503,
              contentType: "text/javascript",
              body: "",
            })
          : route.continue(),
      );
      await page.goto("/app/");
      const heading = page.getByRole("heading", {
        name: "This page could not load",
        level: 1,
      });
      await expect(heading).toBeFocused();
      await expect(page).toHaveTitle("This page could not load · Workbench");
      await expect(page.getByRole("main")).not.toContainText("TypeError");
      await expect(
        page.getByText("Code: UI_PAGE_LOAD_FAILED"),
      ).not.toBeVisible();
      await page.getByText("Support details").click();
      await expect(page.getByText("Code: UI_PAGE_LOAD_FAILED")).toBeVisible();
      const results = await new AxeBuilder({ page })
        .withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
        .analyze();
      expect(
        results.violations.map((v) => ({
          id: v.id,
          nodes: v.nodes.map((n) => n.failureSummary),
        })),
      ).toEqual([]);
      await page.evaluate(() => {
        document.documentElement.style.fontSize = "200%";
      });
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth),
      ).toBeLessThanOrEqual(width);
      await page.screenshot({
        path: `artifacts/route-error-${theme}-${width}.png`,
        fullPage: true,
      });
      failed = false;
      await page.getByRole("button", { name: "Reload page" }).click();
      await expect(
        page.getByRole("heading", { name: "Projects", exact: true }),
      ).toBeVisible();
      await expect(page).toHaveTitle("Projects · Workbench");
    });
  }
}

test("a project-list outage preserves an open work draft and recovers without submitting", async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 700 });
  await page.clock.install();
  let unavailable = false;
  let projectReads = 0;
  await page.route("**/ui-api/**", (route) => {
    expect(route.request().method()).toBe("GET");
    const path = new URL(route.request().url()).pathname;
    if (path === "/ui-api/projects") {
      projectReads++;
      return route.fulfill(
        unavailable
          ? { status: 503, json: { code: "STORAGE_UNAVAILABLE" } }
          : {
              json: {
                items: [{ id: "recovery", name: "Customer operations" }],
              },
            },
      );
    }
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto("/app/projects/recovery/work");
  const draft = page.getByRole("textbox", { name: "Message to your agent" });
  await draft.fill("Keep this draft while project navigation recovers.");
  unavailable = true;
  await page.clock.fastForward(16000);
  await page.evaluate(() =>
    window.dispatchEvent(new Event("visibilitychange")),
  );
  const status = page.getByRole("region", { name: "Project list status" });
  await expect(status).toContainText(
    "Previously loaded projects remain available",
  );
  expect(projectReads).toBeGreaterThan(1);
  expect((await status.boundingBox())!.height).toBeLessThanOrEqual(280);
  const retry = status.getByRole("button", { name: "Try again" });
  const retryBox = (await retry.boundingBox())!;
  const statusBox = (await status.boundingBox())!;
  expect(retryBox.y + retryBox.height).toBeLessThanOrEqual(
    statusBox.y + statusBox.height,
  );
  await expect(draft).toBeInViewport();
  await page.screenshot({
    path: "artifacts/project-list-outage-draft.png",
    fullPage: true,
  });
  await expect(draft).toHaveValue(
    "Keep this draft while project navigation recovers.",
  );
  await expect(page).toHaveTitle("Project work · Workbench");
  unavailable = false;
  await retry.click();
  await expect(status).toHaveCount(0);
  await expect(draft).toHaveValue(
    "Keep this draft while project navigation recovers.",
  );
});

test("help remains available and can retry failed project navigation", async ({
  page,
}) => {
  let unavailable = true;
  await page.route("**/ui-api/projects", (route) =>
    route.fulfill(
      unavailable
        ? { status: 503, json: { code: "STORAGE_UNAVAILABLE" } }
        : { json: { items: [] } },
    ),
  );
  await page.goto("/app/help");
  await expect(
    page.getByRole("heading", { name: "Getting started", level: 1 }),
  ).toBeVisible();
  const status = page.getByRole("region", { name: "Project list status" });
  await expect(status).toContainText("You can continue using this page");
  await expect(page).toHaveTitle("Getting started · Workbench");
  await expect(
    page.getByRole("button", { name: "New project", exact: true }),
  ).toHaveCount(0);
  unavailable = false;
  await page
    .getByRole("button", { name: "Retry project list", exact: true })
    .click();
  await expect(status).toHaveCount(0);
  await expect(
    page.getByRole("heading", { name: "Getting started", level: 1 }),
  ).toBeVisible();
});
