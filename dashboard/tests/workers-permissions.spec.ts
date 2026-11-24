import { test, expect } from "@playwright/test";

test("restricted workers show one stable explanation while service health remains readable", async ({
  page,
}) => {
  let workerReads = 0,
    profileReads = 0;
  await page.clock.install();
  await page.route("**/auth/session", (route) =>
    route.fulfill({
      json: {
        status: "authenticated",
        user: { id: "alice", name: "Alice", email: "alice@example.com" },
      },
    }),
  );
  await page.route("**/dashboard-api/v1/snapshot", (route) =>
    route.fulfill({
      json: {
        scope: "services",
        generated_at: "2026-11-17T10:00:00Z",
        services: {
          items: [
            {
              name: "Control plane",
              status: "healthy",
              reason_code: "PROCESS_READY",
              observed_at: "2026-11-17T10:00:00Z",
            },
          ],
        },
      },
    }),
  );
  await page.route("**/ui-api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/ui-api/projects")
      return route.fulfill({
        json: {
          items: [
            { id: "A", name: "Product", created_at: "2026-11-17T10:00:00Z" },
          ],
        },
      });
    if (path === "/ui-api/workspaces") {
      workerReads++;
      return route.fulfill({
        status: 409,
        json: { code: "PROJECT_ISOLATED_WORKERS_REQUIRED" },
      });
    }
    if (path === "/ui-api/connection-profiles") profileReads++;
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto("/app/projects/A/workspaces");
  await expect(page.getByRole("alert")).toHaveCount(1);
  await expect(page.getByRole("alert")).toContainText(
    "shared worker across projects",
  );
  await expect(
    page.getByRole("button", { name: "Create worker", exact: true }),
  ).toBeDisabled();
  await page.clock.runFor(31000);
  expect(workerReads).toBe(1);
  expect(profileReads).toBe(0);
  await expect(page.getByRole("alert")).toHaveCount(1);
  await page
    .getByRole("button", { name: "Refresh workers", exact: true })
    .click();
  await expect.poll(() => workerReads).toBe(2);
  await page.goto("/app/platform-services");
  await expect(page.getByText("Responding to health checks.")).toBeVisible();
  await expect(page.getByRole("alert")).toHaveCount(0);
});
