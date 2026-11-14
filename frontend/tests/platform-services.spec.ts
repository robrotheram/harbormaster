import { test, expect } from "@playwright/test";

test("service health explains failed and missing observations and recovers from an outage", async ({
  page,
}) => {
  let unavailable = false;
  await page.route("**/ui-api/**", (route) =>
    route.fulfill({ json: { items: [] } }),
  );
  await page.route("**/dashboard-api/**", (route) =>
    unavailable
      ? route.fulfill({ status: 503, json: { code: "SNAPSHOT_UNAVAILABLE" } })
      : route.fulfill({
          json: {
            generated_at: new Date().toISOString(),
            services: {
              items: [
                {
                  name: "Runtime controller",
                  status: "unavailable",
                  reason_code: "HEALTH_CHECK_FAILED",
                  observed_at: "2026-11-14T12:00:00Z",
                },
                {
                  name: "Inference gateway",
                  status: "unknown",
                  reason_code: "NOT_PROBED",
                },
                {
                  name: "Credential broker",
                  status: "healthy",
                  reason_code: "PROCESS_READY",
                },
              ],
            },
          },
        }),
  );
  await page.setViewportSize({ width: 320, height: 900 });
  await page.goto("/app/platform-services");
  await expect(page.getByText(/did not pass its health check/)).toBeVisible();
  await expect(
    page.getByText(/No health check has been reported/),
  ).toBeVisible();
  await expect(page.getByText("Responding to health checks.")).toBeVisible();
  await expect(page.getByText("Code: HEALTH_CHECK_FAILED")).not.toBeVisible();
  await page.getByText("Support details", { exact: true }).first().click();
  await expect(page.getByText("Code: HEALTH_CHECK_FAILED")).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  unavailable = true;
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText(
    "temporarily unavailable",
  );
  await expect(page.getByText("Responding to health checks.")).toHaveCount(0);
  unavailable = false;
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(page.getByRole("alert")).toHaveCount(0);
  await expect(page.getByText("Responding to health checks.")).toBeVisible();
});
