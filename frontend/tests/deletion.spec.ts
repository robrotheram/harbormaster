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
  await page.getByRole("link", { name: "Settings", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Project settings", exact: true }),
  ).toBeVisible();
  const open = page.getByRole("button", {
    name: "Delete project",
    exact: true,
  });
  await open.click();
  const dialog = page.getByRole("dialog", {
    name: "Delete project?",
    exact: true,
  });
  await expect(dialog).toContainText(
    "Private disks and saved evidence remain on the host",
  );
  const confirm = dialog.getByRole("button", {
    name: "Delete project",
    exact: true,
  });
  await expect(confirm).toBeDisabled();
  await dialog
    .getByLabel("Type the project name to confirm")
    .fill("Wrong project");
  await expect(confirm).toBeDisabled();
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toHaveCount(0);
  await expect(open).toBeFocused();
  expect(deleted).toBe(false);
  await open.click();
  await expect(
    dialog.getByLabel("Type the project name to confirm"),
  ).toHaveValue("");
  await dialog
    .getByLabel("Type the project name to confirm")
    .fill("Disposable project");
  await confirm.click();
  await expect.poll(() => deleted).toBe(true);
  await expect(
    page.getByRole("heading", { name: "Projects", exact: true }),
  ).toBeVisible();
});

test("failed project deletion stays in its confirmation dialog with recovery guidance", async ({
  page,
}) => {
  let writes = 0;
  await page.setViewportSize({ width: 320, height: 700 });
  await page.route("**/ui-api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/ui-api/projects")
      return route.fulfill({
        json: { items: [{ id: "A", name: "Customer operations" }] },
      });
    if (path === "/ui-api/projects/A/delete") {
      writes++;
      return route.fulfill({
        status: 409,
        json: { code: "STOP_WORKSPACE_FIRST" },
      });
    }
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto("/app/projects/A/settings");
  await page
    .getByRole("button", { name: "Delete project", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Delete project?",
    exact: true,
  });
  await dialog
    .getByLabel("Type the project name to confirm")
    .fill("Customer operations");
  await dialog
    .getByRole("button", { name: "Delete project", exact: true })
    .click();
  await expect(dialog.getByRole("alert")).toContainText(
    "Finish active work and close terminals first",
  );
  await expect(
    dialog.getByLabel("Type the project name to confirm"),
  ).toHaveValue("Customer operations");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  expect(writes).toBe(1);
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
});
