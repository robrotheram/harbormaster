import { test, expect } from "@playwright/test";

for (const [oldSection, heading] of [
  ["homes", "Workers"],
  ["factory", "Project work"],
]) {
  test(`old ${oldSection} bookmark opens ${heading} without retired requests`, async ({
    page,
  }) => {
    const requests: string[] = [];
    await page.route("**/dashboard-api/**", (r) => r.fulfill({ json: {} }));
    await page.route("**/ui-api/**", (r) => {
      const path = new URL(r.request().url()).pathname;
      requests.push(path);
      return r.fulfill({
        json: {
          items:
            path === "/ui-api/projects" ? [{ id: "audit", name: "Audit" }] : [],
        },
      });
    });
    await page.goto(`/app/projects/audit/${oldSection}`);
    await expect(
      page.getByRole("heading", { name: heading, exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: "Agent homes", exact: true }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("link", { name: "Outputs & previews", exact: true }),
    ).toHaveCount(0);
    expect(requests.some((p) => /agent-homes|recipe-jobs/.test(p))).toBe(false);
  });
}
