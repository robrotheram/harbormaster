import { expect, test } from "@playwright/test";

test("local profile remains accessible in expanded, collapsed, and mobile navigation", async ({
  page,
}) => {
  await page.route("**/ui-api/**", (route) => route.fulfill({ json: [] }));
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto("/app/");
  const profile = page.getByRole("button", {
    name: /User profile: .*Local operator/,
  });
  await expect(profile).toBeVisible();
  await profile.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("menu", { name: "User profile" })).toBeVisible();
  await expect(
    page.getByText("Sign-in is not configured.", { exact: false }),
  ).toBeVisible();
  await expect(page.getByRole("menuitem", { name: "Sign in" })).toBeDisabled();
  await page.keyboard.press("Escape");
  await expect(profile).toBeFocused();

  await page.getByRole("button", { name: "Collapse sidebar" }).click();
  await profile.click();
  await expect(page.getByRole("menu", { name: "User profile" })).toBeVisible();
  await page.keyboard.press("Escape");

  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "Open navigation" }).click();
  await profile.click();
  await expect(page.getByRole("menu", { name: "User profile" })).toBeVisible();
  await expect(page.getByRole("menuitem", { name: "Sign in" })).toBeDisabled();
});
