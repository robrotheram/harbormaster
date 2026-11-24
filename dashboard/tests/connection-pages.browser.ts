import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

const hosts = [
  "preview-review.localhost",
  "ws-0123456789abcdef0123456789abcdef.localhost",
];
const url = (host: string, path = "/connect") => `http://${host}:5191${path}`;

test("missing links recover without a POST and fit both themes at desktop and phone widths", async ({
  page,
}) => {
  const posts: string[] = [],
    errors: string[] = [];
  page.on("request", (request) => {
    if (request.method() === "POST") posts.push(request.url());
  });
  page.on("pageerror", (error) => errors.push(error.message));
  for (const host of hosts) {
    for (const width of [1440, 320]) {
      await page.setViewportSize({ width, height: 800 });
      for (const colorScheme of ["light", "dark"] as const) {
        await page.emulateMedia({ colorScheme });
        await page.goto(url(host));
        await expect(page.getByRole("heading", { level: 1 })).toHaveText(
          /Open this (preview|worker) from Workbench/,
        );
        await expect(page.getByRole("heading", { level: 1 })).toBeFocused();
        const link = page.getByRole("link", { name: "Open Workbench" });
        await expect(link).toHaveAttribute(
          "href",
          "http://localhost:5191/app/",
        );
        await expect(link).toHaveAttribute("rel", "noopener noreferrer");
        expect((await link.boundingBox())!.height).toBeGreaterThanOrEqual(44);
        await page.getByText("Support details", { exact: true }).click();
        await expect(page.locator("#support-code")).toHaveText(
          "OPEN_FROM_WORKBENCH",
        );
        const audit = await new AxeBuilder({ page })
          .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
          .analyze();
        expect(audit.violations).toEqual([]);
        await page.evaluate(() => {
          document.documentElement.style.fontSize = "200%";
        });
        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth,
          ),
        ).toBe(true);
        await expect(link).toBeVisible();
      }
    }
  }
  expect(posts).toEqual([]);
  expect(errors).toEqual([]);
});

test("expired links clear the ticket and present a stable recovery code", async ({
  page,
}) => {
  for (const host of hosts) {
    await page.goto(url(host, "/connect#ticket=expired-review-ticket"));
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(
      "This connection link has expired",
    );
    await expect(page).toHaveURL(url(host));
    await expect(page.locator("body")).not.toContainText(
      "expired-review-ticket",
    );
    await page.getByText("Support details", { exact: true }).click();
    await expect(page.locator("#support-code")).toHaveText(
      "CONNECTION_EXPIRED",
    );
    await expect(page.locator("#http-status")).toHaveText("403");
  }
});

test("network and service failures explain recovery without replaying a one-use ticket", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  for (const mode of ["offline", "service", "unknown", "malformed"] as const) {
    let posts = 0;
    await page.route("**/connect", (route) => {
      if (route.request().method() !== "POST") return route.continue();
      posts++;
      if (mode === "offline") return route.abort("failed");
      return route.fulfill({
        status: 503,
        json:
          mode === "malformed"
            ? null
            : mode === "service"
              ? { code: "PREVIEW_UNAVAILABLE" }
              : {
                  code: "private-upstream-detail",
                  message: "secret-debug-body",
                },
      });
    });
    await page.goto(url(hosts[0], "/connect#ticket=test"));
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(
      mode === "offline"
        ? "Connection interrupted"
        : mode === "service"
          ? "Preview unavailable"
          : "Connection unavailable",
    );
    await expect(page.locator("body")).not.toContainText(
      /secret-debug-body|private-upstream-detail/,
    );
    expect(posts).toBe(1);
    await page.unroute("**/connect");
  }
  expect(errors).toEqual([]);
});

test("a stalled connection has a bounded wait and actionable timeout", async ({
  page,
}) => {
  await page.clock.install();
  await page.route("**/connect", async (route) => {
    if (route.request().method() !== "POST") return route.continue();
    await new Promise<void>((resolve) => {
      page.once("close", () => resolve());
    });
  });
  await page.goto(url(hosts[0], "/connect#ticket=slow"));
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(
    "Opening preview",
  );
  await page.clock.fastForward(20001);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(
    "The connection is taking too long",
  );
  await page.getByText("Support details", { exact: true }).click();
  await expect(page.locator("#support-code")).toHaveText("CONNECTION_TIMEOUT");
});

test("direct unauthenticated navigation gets a recovery page and fetches retain JSON", async ({
  page,
  request,
}) => {
  for (const host of hosts) {
    const response = await page.goto(url(host, "/"));
    expect(response!.status()).toBe(401);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(
      /Open this (preview|worker) from Workbench/,
    );
    const api = await request.get(url(host, "/"));
    expect(api.status()).toBe(401);
    expect(api.headers()["content-type"]).toContain("application/json");
  }
});

test("embedded recovery preserves the editor focus and uses the parent preview controls", async ({
  page,
}) => {
  let release!: () => void;
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/connect", async (route) => {
    if (route.request().method() !== "POST") return route.continue();
    await held;
    await route.fulfill({ status: 403, json: { code: "CONNECTION_EXPIRED" } });
  });
  await page.goto("http://127.0.0.1:5191/_fixture/ready");
  await page.setContent(
    `<textarea aria-label="Unsent draft"></textarea><iframe title="Application preview" sandbox="allow-scripts allow-same-origin allow-forms allow-downloads allow-modals allow-pointer-lock" src="${url(hosts[0], "/connect#ticket=embedded")}"></iframe>`,
  );
  const draft = page.getByRole("textbox", { name: "Unsent draft" });
  await draft.fill("Keep this message while the preview reconnects");
  release();
  const frame = page.frameLocator("iframe");
  await expect(frame.getByRole("heading", { level: 1 })).toHaveText(
    "This connection link has expired",
  );
  await expect(frame.getByRole("link", { name: "Open Workbench" })).toHaveCount(
    0,
  );
  await expect(
    frame.getByText(/Use Reload preview above this panel/),
  ).toBeVisible();
  await expect(draft).toBeFocused();
  await expect(draft).toHaveValue(
    "Keep this message while the preview reconnects",
  );
});

test("valid worker connection preserves the single-use exchange and redirect", async ({
  page,
  request,
}) => {
  const issued = await request.post(
    "http://127.0.0.1:5191/ui-api/workspaces/ws_0123456789abcdef0123456789abcdef/start",
    {
      headers: { "X-Workbench-Request": "1", Origin: "http://127.0.0.1:5191" },
      data: { idempotency_key: "browser-fixture-only" },
    },
  );
  expect(issued.status()).toBe(200);
  const ticket = (await issued.json()).console_ticket;
  expect(ticket).toBeTruthy();
  await page.goto(url(hosts[1], "/connect#ticket=expired-review-ticket"));
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(
    "This connection link has expired",
  );
  // A fresh link pasted into this same document must start a new exchange.
  await page.goto(
    url(hosts[1], `/connect#ticket=${encodeURIComponent(ticket)}`),
  );
  await expect(page).toHaveURL(url(hosts[1], "/"));
  await expect(page.locator("body")).toContainText('"fixture":true');
  await page.goto(
    url(hosts[1], `/connect#ticket=${encodeURIComponent(ticket)}`),
  );
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(
    "This connection link has expired",
  );
});
