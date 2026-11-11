import { test, expect } from "@playwright/test";
test("deletes a project only after the operator confirms", async ({ page }) => {
  let deleted = false;
  await page.route("**/ui-api/**", async (route) => {
    const req = route.request(),
      path = new URL(req.url()).pathname;
    let value: unknown = { items: [] };
    if (path === "/ui-api/projects")
      value = {
        items: deleted ? [] : [{ id: "A", name: "Disposable project" }],
      };
    if (path === "/ui-api/projects/A/delete") {
      expect(req.postDataJSON()).toEqual({ confirm_id: "A" });
      deleted = true;
      value = { deleted: true, private_disks: "retained" };
    }
    await route.fulfill({ json: value });
  });
  await page.goto("/app/projects/A/factory");
  await expect(
    page
      .getByRole("banner")
      .getByRole("button", { name: "Delete project", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Delete project", exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("link", { name: "Project settings", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Project settings", exact: true }),
  ).toBeVisible();
  page.once("dialog", (dialog) => dialog.dismiss());
  await page
    .getByRole("button", { name: "Delete project", exact: true })
    .click();
  expect(deleted).toBe(false);
  page.once("dialog", async (dialog) => {
    expect(dialog.message()).toContain("Private disks remain on the host");
    await dialog.accept();
  });
  await page
    .getByRole("button", { name: "Delete project", exact: true })
    .click();
  await expect.poll(() => deleted).toBe(true);
});
