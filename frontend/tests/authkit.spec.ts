import { expect, test } from "@playwright/test";

const user = {
  id: "user_one",
  name: "Test Operator",
  email: "operator@example.com",
};

test("signed-out users cannot load project data and can start AuthKit sign-in", async ({
  page,
}) => {
  let privateRequests = 0;
  await page.route("**/ui-api/**", (route) => {
    privateRequests++;
    return route.fulfill({ json: [] });
  });
  await page.route("**/auth/session", (route) =>
    route.fulfill({ json: { status: "anonymous" } }),
  );
  await page.route("**/auth/login", (route) =>
    route.fulfill({
      contentType: "text/html",
      body: "<h1>Hosted authentication</h1>",
    }),
  );
  await page.goto("/app/");
  await expect(
    page.getByRole("heading", { name: "A home for your agent work." }),
  ).toBeVisible();
  expect(privateRequests).toBe(0);
  await page.getByRole("link", { name: "Open your workbench" }).click();
  await expect(
    page.getByRole("heading", { name: "Sign in to your workbench" }),
  ).toBeVisible();
  expect(privateRequests).toBe(0);
  await page.getByRole("button", { name: "Sign in with AuthKit" }).click();
  await expect(page).toHaveURL(/\/auth\/login$/);
});

test("profile uses verified session and logout ends the session", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.route("**/ui-api/**", (route) => route.fulfill({ json: [] }));
  await page.route("**/auth/session", (route) =>
    route.fulfill({ json: { status: "authenticated", user } }),
  );
  let logoutRequests = 0;
  await page.route("**/auth/logout", (route) => {
    logoutRequests++;
    expect(route.request().method()).toBe("POST");
    expect(route.request().headers()["x-workbench-request"]).toBe("1");
    return route.fulfill({
      json: {
        url: "https://api.workos.com/user_management/sessions/logout?session_id=test",
      },
    });
  });
  await page.route("https://api.workos.com/**", (route) =>
    route.fulfill({ contentType: "text/html", body: "<h1>Signed out</h1>" }),
  );
  await page.goto("/app/");
  await page
    .getByRole("button", { name: /User profile: .*Test Operator/ })
    .click();
  await expect(page.getByRole("menu").getByText(user.email)).toBeVisible();
  await page.getByRole("menuitem", { name: "Log out" }).click();
  await expect(page.getByRole("heading", { name: "Signed out" })).toBeVisible();
  expect(logoutRequests).toBe(1);
});

test("session failures fail closed and allow retry", async ({ page }) => {
  let unavailable = true;
  await page.route("**/auth/session", (route) =>
    route.fulfill(
      unavailable
        ? { status: 503, json: { code: "AUTH_UNAVAILABLE" } }
        : { json: { status: "anonymous" } },
    ),
  );
  await page.goto("/app/");
  await expect(page.getByRole("alert")).toContainText(
    "Could not check your session",
  );
  await expect(page.getByRole("button", { name: /User profile/ })).toHaveCount(
    0,
  );
  unavailable = false;
  await page.getByRole("button", { name: "Try again" }).click();
  await expect(
    page.getByRole("link", { name: "Open your workbench" }),
  ).toBeVisible();
});

test("session expiry removes private views on focus", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 800 });
  let expired = false;
  await page.route("**/ui-api/**", (route) => route.fulfill({ json: [] }));
  await page.route("**/auth/session", (route) =>
    route.fulfill({
      json: expired
        ? { status: "anonymous" }
        : { status: "authenticated", user },
    }),
  );
  await page.goto("/app/");
  await expect(
    page.getByRole("button", { name: /User profile: .*Test Operator/ }),
  ).toBeVisible();
  expired = true;
  await page.evaluate(() => window.dispatchEvent(new Event("focus")));
  await expect(
    page.getByRole("heading", { name: "A home for your agent work." }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: /User profile: .*Test Operator/ }),
  ).toHaveCount(0);
});

test("callback errors are readable and removed from the URL", async ({
  page,
}) => {
  await page.route("**/auth/session", (route) =>
    route.fulfill({ json: { status: "anonymous" } }),
  );
  await page.goto("/app/?auth_error=access_denied");
  await expect(page.getByRole("alert")).toContainText(
    "does not have operator access",
  );
  await expect(page).toHaveURL(/\/app\/$/);
});

for (const width of [375, 1280]) {
  test(`public entry supports navigation at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 850 });
    await page.route("**/auth/session", (route) =>
      route.fulfill({ json: { status: "anonymous" } }),
    );
    await page.goto("/app/");
    await expect(page).toHaveTitle("Workbench · A home for your agent work");
    await expect(
      page.getByRole("heading", { name: "A home for your agent work." }),
    ).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.getByRole("link", { name: "Sign in", exact: true }).click();
    await expect(
      page.getByRole("button", { name: "Sign in with AuthKit" }),
    ).toBeVisible();
    await expect(page).toHaveTitle("Sign in · Workbench");
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.getByRole("link", { name: "Back to home" }).click();
    await expect(
      page.getByRole("heading", { name: "A home for your agent work." }),
    ).toBeVisible();
  });
}

test("private deep links show sign-in without mounting project views", async ({
  page,
}) => {
  let privateRequests = 0;
  await page.route("**/ui-api/**", (route) => {
    privateRequests++;
    return route.fulfill({ json: [] });
  });
  await page.route("**/auth/session", (route) =>
    route.fulfill({ json: { status: "anonymous" } }),
  );
  await page.goto("/app/projects/example/work");
  await expect(
    page.getByRole("heading", { name: "Sign in to your workbench" }),
  ).toBeVisible();
  expect(privateRequests).toBe(0);
});
