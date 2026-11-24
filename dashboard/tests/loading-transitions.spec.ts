import { test, expect } from "@playwright/test";

test("slow page navigation reveals a static skeleton with reduced motion", async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.route("**/ui-api/**", (route) =>
    route.fulfill({ json: { items: [] } }),
  );
  await page.route("**/ui-api/projects", (route) =>
    route.fulfill({ json: { items: [{ id: "factory", name: "Factory" }] } }),
  );

  const chunk = Promise.withResolvers<void>();
  const profiles = Promise.withResolvers<void>();
  await page.route("**/app/assets/connection-profiles-*.js", async (route) => {
    await chunk.promise;
    await route.continue();
  });
  await page.route("**/ui-api/connection-profiles", async (route) => {
    await profiles.promise;
    await route.fulfill({ json: { items: [] } });
  });

  await page.goto("/app/");
  await expect(
    page.getByRole("heading", { name: "Projects", exact: true }),
  ).toBeVisible();

  // Capture the first computed visibility, even if the assertion runs after
  // the indicator's delay has elapsed.
  await page.evaluate(() => {
    const observer = new MutationObserver(() => {
      for (const skeleton of document.querySelectorAll(
        "main [data-slot=skeleton]",
      )) {
        if (!skeleton.hasAttribute("data-initial-visibility"))
          skeleton.setAttribute(
            "data-initial-visibility",
            getComputedStyle(skeleton).visibility,
          );
      }
    });
    observer.observe(document.body, {
      childList: true,
      subtree: true,
    });
  });

  await page.getByRole("button", { name: "Platform" }).click();
  await page.getByRole("menuitem", { name: "Network profiles" }).click();
  const fallback = page.locator("main [data-slot=skeleton][role=status]");
  try {
    await expect(fallback).toHaveAttribute("data-initial-visibility", "hidden");
    await expect(fallback).toBeVisible();
    await expect(fallback).toHaveCSS("animation-iteration-count", "1");
  } finally {
    chunk.resolve();
  }

  const skeleton = page.locator("main [data-slot=skeleton]:not([role=status])");
  try {
    await expect(skeleton).toHaveAttribute("data-initial-visibility", "hidden");
    await expect(skeleton).toBeVisible();
    await expect(skeleton).toHaveCSS("animation-iteration-count", "1");
  } finally {
    profiles.resolve();
  }

  await expect(page.getByText(/No network profiles yet/)).toBeVisible();
  await expect(skeleton).toHaveCount(0);
  await page.goBack();
  await expect(
    page.getByRole("heading", { name: "Projects", exact: true }),
  ).toBeVisible();
  await expect(page.locator("main [data-slot=skeleton]")).toHaveCount(0);
});

test("refresh stays still for quick responses and spins during a slow request", async ({
  page,
}) => {
  let pending: Promise<void> | undefined;
  await page.route("**/ui-api/**", (route) =>
    route.fulfill({ json: { items: [] } }),
  );
  await page.route("**/dashboard-api/**", async (route) => {
    await pending;
    await route.fulfill({ json: { services: { items: [] } } });
  });

  await page.goto("/app/platform-services");
  const refresh = page.getByRole("button", { name: "Refresh", exact: true });
  await expect(refresh).toBeEnabled();
  const icon = refresh.locator("svg");
  await icon.evaluate((node) => {
    node.addEventListener("animationstart", () =>
      node.setAttribute("data-spin-started", ""),
    );
  });

  await refresh.click();
  await expect(refresh).toBeEnabled();
  await expect(icon).not.toHaveAttribute("data-spin-started");

  const slow = Promise.withResolvers<void>();
  pending = slow.promise;
  try {
    await refresh.click();
    await expect(refresh).toBeDisabled();
    await expect(icon).toHaveAttribute("data-spin-started", "");
  } finally {
    slow.resolve();
  }
  await expect(refresh).toBeEnabled();
  await expect(icon).toHaveCSS("transform", "none");
});
