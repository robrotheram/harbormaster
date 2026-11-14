import { test, expect } from "@playwright/test";

test("scheduling pages and navigation are removed", async ({ page }) => {
  await page.route("**/ui-api/**", (route) =>
    route.fulfill({
      json: {
        items: route.request().url().includes("/projects")
          ? [{ id: "project_one", name: "Factory" }]
          : [],
      },
    }),
  );
  for (const section of ["schedules", "runs"]) {
    await page.goto(`/app/projects/project_one/${section}`);
    await expect(
      page.getByRole("heading", { name: "Page unavailable" }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: "Schedules", exact: true }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("link", { name: "Scheduled runs", exact: true }),
    ).toHaveCount(0);
  }
});
