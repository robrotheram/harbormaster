import { test, expect } from "@playwright/test";

test("connection profile deletion confirms, displays errors, and refreshes the list", async ({
  page,
}) => {
  const profile = {
    id: "np_test",
    name: "Team access",
    description: "Shared network policy",
    policy: { mode: "allowlist", destinations: ["github.com:443"] },
    revision: 3,
    updated_at: "2026-11-01T10:00:00Z",
  };
  let deleted = false;
  let inUse = true;
  const mutations: unknown[] = [];
  await page.route("**/ui-api/**", async (route) => {
    const req = route.request();
    const path = new URL(req.url()).pathname;
    let status = 200;
    let value: unknown = { items: [] };
    if (path === "/ui-api/connection-profiles") {
      value = { items: deleted ? [] : [profile] };
    } else if (
      path === "/ui-api/connection-profiles/np_test/delete" &&
      req.method() === "POST"
    ) {
      mutations.push(req.postDataJSON());
      status = inUse ? 409 : 200;
      value = inUse ? { code: "CONNECTION_PROFILE_IN_USE" } : { deleted: true };
      deleted = !inUse;
    }
    await route.fulfill({
      status,
      contentType: "application/json",
      body: JSON.stringify(value),
    });
  });
  await page.goto("/app/connection-profiles");
  await expect(
    page.getByRole("button", { name: "Delete profile", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Edit profile", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog
    .getByRole("textbox", { name: "Profile name", exact: true })
    .fill("Unsaved name");
  await dialog
    .getByRole("button", { name: "Delete profile", exact: true })
    .click();
  await expect(dialog).toContainText("Team access");
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  expect(mutations).toHaveLength(0);
  await expect(
    dialog.getByRole("textbox", { name: "Profile name", exact: true }),
  ).toHaveValue("Unsaved name");
  await dialog
    .getByRole("button", { name: "Delete profile", exact: true })
    .click();
  await dialog
    .getByRole("button", { name: "Delete profile", exact: true })
    .click();
  await expect(dialog.getByRole("alert")).toContainText(
    "still used by a worker or Hermes setup",
  );
  expect(mutations).toEqual([{ revision: 3 }]);
  inUse = false;
  await dialog
    .getByRole("button", { name: "Delete profile", exact: true })
    .click();
  await expect(dialog).not.toBeVisible();
  await expect(page.locator('[data-profile="np_test"]')).toHaveCount(0);
  expect(mutations).toEqual([{ revision: 3 }, { revision: 3 }]);
});
