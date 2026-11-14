import { test, expect, type Page } from "@playwright/test";

async function fixture(page: Page, count = 1) {
  const projects = Array.from({ length: count }, (_, i) => ({
    id: `project_${i}`,
    name: `Customer operations ${i + 1}`,
    created_at: "2026-11-14T12:00:00Z",
  }));
  await page.route("**/ui-api/**", (route) =>
    route.fulfill({
      json: {
        items:
          new URL(route.request().url()).pathname === "/ui-api/projects"
            ? projects
            : [],
      },
    }),
  );
  await page.goto("/app/");
  await expect(
    page.getByRole("heading", { name: "Projects", exact: true }),
  ).toBeVisible();
  return projects;
}

test("project creation rejects blank names and preserves a failed draft while locking pending operations", async ({
  page,
}) => {
  await fixture(page);
  let writes = 0;
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/ui-api/projects", async (route) => {
    if (route.request().method() !== "POST") return route.fallback();
    writes++;
    expect(route.request().postDataJSON().name).toBe("Customer support");
    await gate;
    return route.fulfill({
      status: 503,
      json: { code: "STORAGE_UNAVAILABLE" },
    });
  });
  const open = page.getByRole("button", { name: "New project", exact: true });
  await open.click();
  const dialog = page.getByRole("dialog", {
    name: "Create project",
    exact: true,
  });
  const input = dialog.getByRole("textbox", {
    name: "Project name",
    exact: true,
  });
  const submit = dialog.getByRole("button", {
    name: "Create project",
    exact: true,
  });
  await expect(input).toBeFocused();
  await input.fill("   ");
  await expect(submit).toBeDisabled();
  await input.press("Enter");
  expect(writes).toBe(0);
  await input.fill("Customer support");
  await input.press("Enter");
  await expect(input).toBeDisabled();
  await expect(
    dialog.getByRole("button", { name: "Cancel", exact: true }),
  ).toBeDisabled();
  await page.keyboard.press("Escape");
  await expect(
    dialog.getByRole("button", { name: "Close", exact: true }),
  ).toBeDisabled();
  await expect(dialog).toBeVisible();
  release();
  await expect(dialog.getByRole("alert")).toBeVisible();
  await expect(input).toHaveValue("Customer support");
  await expect(submit).toBeEnabled();
  expect(writes).toBe(1);
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(open).toBeFocused();
});

test("large project portfolios remain searchable and keyboard reachable", async ({
  page,
}) => {
  const queriedProjects = new Set<string>();
  page.on("request", (request) => {
    const url = new URL(request.url());
    if (
      url.pathname === "/ui-api/work-items" &&
      url.searchParams.has("project_id")
    )
      queriedProjects.add(url.searchParams.get("project_id")!);
  });
  await fixture(page, 120);
  await expect(page.locator(".project-inventory-row")).toHaveCount(20);
  await expect.poll(() => queriedProjects.size).toBe(20);
  await expect(
    page.getByRole("navigation", { name: "Project pages" }),
  ).toContainText("Showing 1–20 of 120 projects");
  expect(
    [...queriedProjects].every((id) => Number(id.replace("project_", "")) < 20),
  ).toBe(true);
  await page.getByRole("button", { name: "Next page", exact: true }).click();
  await expect(
    page.getByRole("navigation", { name: "Project pages" }),
  ).toContainText("Showing 21–40 of 120 projects");
  await expect(page.locator(".project-inventory-row").first()).toContainText(
    "Customer operations 21",
  );
  await page.setViewportSize({ width: 320, height: 700 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page
    .getByRole("button", { name: "Next page", exact: true })
    .scrollIntoViewIfNeeded();
  await page.screenshot({ path: "artifacts/project-pagination-phone.png" });
  await page.setViewportSize({ width: 1440, height: 900 });
  const search = page.getByRole("textbox", {
    name: "Search projects",
    exact: true,
  });
  await search.fill("  Customer operations 120  ");
  await expect(page.locator(".project-inventory-row")).toHaveCount(1);
  await expect(page.locator(".inventory-toolbar")).toContainText(
    "1 of 120 projects",
  );
  await search.press("Tab");
  await expect(page.locator(".project-inventory-row")).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/projects\/project_119\/work/);
  const switcher = page.getByRole("button", {
    name: "Switch project: Customer operations 120",
    exact: true,
  });
  await switcher.focus();
  await page.keyboard.press("Enter");
  await page.keyboard.press("End");
  await expect(
    page.getByRole("menuitem", { name: "New project", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(switcher).toBeFocused();
});

test("phone navigation and project dialogs keep keyboard focus contained and restore it", async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 700 });
  await fixture(page);
  await expect(page.locator(".inventory-toolbar")).toContainText("1 project");
  const open = page.getByRole("button", {
    name: "Open navigation",
    exact: true,
  });
  await open.focus();
  await page.keyboard.press("Enter");
  const navigation = page.getByRole("dialog", {
    name: "Factory navigation",
    exact: true,
  });
  await expect(navigation).toBeVisible();
  for (let i = 0; i < 18; i++) {
    await page.keyboard.press("Tab");
    expect(
      await navigation.evaluate((el) => el.contains(document.activeElement)),
    ).toBe(true);
  }
  await page.keyboard.press("Escape");
  await expect(navigation).toHaveCount(0);
  await expect(open).toBeFocused();
  await page.getByRole("button", { name: "New project", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "Create project",
    exact: true,
  });
  for (let i = 0; i < 6; i++) {
    await page.keyboard.press("Tab");
    expect(
      await dialog.evaluate((el) => el.contains(document.activeElement)),
    ).toBe(true);
  }
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.keyboard.press("Escape");
  await expect(
    page.getByRole("button", { name: "New project", exact: true }),
  ).toBeFocused();
});

test("project dialogs remain readable with doubled text at phone width", async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 700 });
  await fixture(page);
  await page.getByRole("button", { name: "New project", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "Create project",
    exact: true,
  });
  await dialog.evaluate((el) => {
    // A text-only enlargement stress test, not a substitute for browser zoom/AT review.
    const elements = [...el.querySelectorAll<HTMLElement>("*")];
    const sizes = elements.map((item) =>
      parseFloat(getComputedStyle(item).fontSize),
    );
    elements.forEach((item, i) =>
      item.style.setProperty("font-size", `${sizes[i] * 2}px`, "important"),
    );
  });
  const dimensions = await dialog.evaluate((el) => {
    const note = el.querySelector(".form-note")!;
    const styles = getComputedStyle(note);
    return {
      width: el.clientWidth,
      contentWidth: el.scrollWidth,
      fontSize: parseFloat(styles.fontSize),
      lineHeight: parseFloat(styles.lineHeight),
      top: el.getBoundingClientRect().top,
      bottom: el.getBoundingClientRect().bottom,
    };
  });
  expect(dimensions.contentWidth).toBeLessThanOrEqual(dimensions.width);
  expect(dimensions.lineHeight).toBeGreaterThanOrEqual(
    dimensions.fontSize * 1.5,
  );
  expect(dimensions.top).toBeGreaterThanOrEqual(0);
  expect(dimensions.bottom).toBeLessThanOrEqual(700);
  const inputDimensions = await dialog
    .getByRole("textbox", { name: "Project name", exact: true })
    .evaluate((el) => ({
      height: el.getBoundingClientRect().height,
      fontSize: parseFloat(getComputedStyle(el).fontSize),
    }));
  expect(inputDimensions.height).toBeGreaterThanOrEqual(
    inputDimensions.fontSize * 1.5,
  );
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).toHaveCount(0);
});
