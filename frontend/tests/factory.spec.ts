import { test, expect } from "@playwright/test";
test("runs a recipe, retains gallery, opens isolated origin and revokes", async ({
  page,
}) => {
  let running = false,
    revoked = false;
  const deliveries: Record<string, unknown>[] = [];
  await page.addInitScript(() => {
    window.open = (url) => {
      (window as unknown as { opened: string }).opened = String(url);
      return null;
    };
  });
  await page.route("**/ui-api/**", async (route) => {
    const req = route.request(),
      path = new URL(req.url()).pathname;
    let value: unknown = { items: [] };
    if (path === "/ui-api/projects")
      value = { items: [{ id: "A", name: "Factory" }] };
    if (path === "/ui-api/workspaces")
      value =
        req.method() === "POST"
          ? { id: "ws_retained", project_id: "A" }
          : {
              items: [
                {
                  id: "ws_coding",
                  project_id: "A",
                  name: "Gallery coding",
                  state: "READY",
                },
              ],
            };
    if (path.endsWith("/recipe-jobs")) {
      if (req.method() === "POST") running = true;
      value = {
        items: running
          ? [{ id: "job_one", template: "gogallery-v1", state: "succeeded" }]
          : [],
      };
    }
    if (path.endsWith("/artifacts"))
      value = {
        items: running
          ? [
              {
                id: "artifact_gallery",
                source_path: "jobs/job_one/gallery.tar",
                sha256: "abc",
                size_bytes: 4096,
              },
            ]
          : [],
      };
    const service = {
      id: "service_one",
      state: revoked ? "revoked" : "active",
      expires_at: 2000000000,
    };
    if (path.endsWith("/retained-previews")) {
      deliveries.push(req.postDataJSON());
      value = service;
    }
    if (path.endsWith("/services/service_one")) value = service;
    if (path.endsWith("/connect"))
      value = {
        preview_url:
          "http://preview-unique.localhost:8088/connect#ticket=one-use",
      };
    if (path.endsWith("/revoke")) {
      revoked = true;
      value = { ...service, state: "revoked" };
    }
    await route.fulfill({ json: value });
  });
  await page.goto("/app/projects/A/factory");
  await page
    .getByRole("combobox", { name: "Worker", exact: true })
    .selectOption("ws_coding");
  await page.getByLabel("Recipe", { exact: true }).selectOption("gogallery-v1");
  await page.getByRole("button", { name: "Start job", exact: true }).click();
  await page.getByRole("button", { name: "Retain gallery for 1 hour" }).click();
  await page.getByRole("button", { name: "Open isolated preview" }).click();
  await expect(
    page.getByRole("link", { name: "Open retained preview in a new tab" }),
  ).toHaveAttribute(
    "href",
    "http://preview-unique.localhost:8088/connect#ticket=one-use",
  );
  await page.getByRole("button", { name: "Revoke preview" }).click();
  await expect(
    page.getByRole("button", { name: "Open isolated preview" }),
  ).toBeDisabled();
  await expect(
    page.getByRole("button", { name: "Release environment" }),
  ).toBeEnabled();
  expect(deliveries).toEqual([
    {
      retained_workspace_id: "ws_retained",
      artifact_id: "artifact_gallery",
      ttl_seconds: 3600,
      idempotency_key: expect.any(String),
    },
  ]);
});

test("output reader preserves Unicode, contains long text and clears worker-bound content", async ({
  page,
}) => {
  let finishRead: (() => void) | undefined;
  const content =
    "Résumé — 東京\n<script>window.outputExecuted = true</script>\n" +
    "long-path/".repeat(100);
  await page.route("**/ui-api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/ui-api/projects")
      return route.fulfill({ json: { items: [{ id: "A", name: "Outputs" }] } });
    if (path === "/ui-api/workspaces")
      return route.fulfill({
        json: {
          items: [
            {
              id: "ws_one",
              project_id: "A",
              name: "First worker",
              state: "READY",
            },
            {
              id: "ws_two",
              project_id: "A",
              name: "Second worker",
              state: "STOPPED",
            },
          ],
        },
      });
    if (path === "/ui-api/workspaces/ws_one/artifacts")
      return route.fulfill({
        json: {
          items: [
            {
              id: "artifact_text",
              source_path: "report.md",
              sha256: "abc",
              size_bytes: 1000,
            },
          ],
        },
      });
    if (path.endsWith("/artifacts/artifact_text/content")) {
      await new Promise<void>((resolve) => {
        finishRead = resolve;
      });
      return route.fulfill({
        json: { content: Buffer.from(content).toString("base64") },
      });
    }
    return route.fulfill({ json: { items: [] } });
  });
  await page.setViewportSize({ width: 320, height: 780 });
  await page.goto("/app/projects/A/factory");
  await page
    .getByRole("combobox", { name: "Worker", exact: true })
    .selectOption("ws_one");
  await page.getByRole("button", { name: "Read output" }).click();
  await expect.poll(() => !!finishRead).toBe(true);
  await expect(
    page.getByRole("combobox", { name: "Worker", exact: true }),
  ).toBeDisabled();
  await expect(page.getByLabel("Recipe", { exact: true })).toBeDisabled();
  finishRead!();
  const dialog = page.getByRole("dialog", { name: "Saved output" });
  await expect(dialog).toContainText("Résumé — 東京");
  expect(
    await page.evaluate(
      () => (window as unknown as Record<string, unknown>).outputExecuted,
    ),
  ).toBeUndefined();
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth),
  ).toBeLessThanOrEqual(320);
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
  await page
    .getByRole("combobox", { name: "Worker", exact: true })
    .selectOption("ws_two");
  await expect(page.getByText("report.md", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

test("unavailable preview observations disable lifecycle actions instead of trusting cached state", async ({
  page,
}) => {
  let statusUnavailable = true;
  await page.route("**/ui-api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/ui-api/projects")
      return route.fulfill({ json: { items: [{ id: "A", name: "Outputs" }] } });
    if (path === "/ui-api/workspaces")
      return route.fulfill({
        json: {
          items: [
            {
              id: "ws_retained",
              project_id: "A",
              name: "Saved gallery",
              state: "READY",
              retained_preview: {
                service_id: "service_one",
                expires_at: 2000000000,
              },
            },
          ],
        },
      });
    if (path.endsWith("/services/service_one"))
      return statusUnavailable
        ? route.fulfill({ status: 503, json: { code: "RUNTIME_UNAVAILABLE" } })
        : route.fulfill({
            json: {
              id: "service_one",
              state: "active",
              expires_at: 2000000000,
            },
          });
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto("/app/projects/A/factory");
  await page.getByRole("button", { name: /Saved gallery/ }).click();
  await expect(
    page.getByRole("button", { name: "Release environment" }),
  ).toBeDisabled();
  await expect(page.getByRole("alert")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Open isolated preview" }),
  ).toBeDisabled();
  await expect(
    page.getByRole("button", { name: "Revoke preview" }),
  ).toBeDisabled();
  statusUnavailable = false;
  await expect(
    page.getByRole("button", { name: "Open isolated preview" }),
  ).toBeEnabled({ timeout: 10000 });
  await expect(
    page.getByRole("button", { name: "Release environment" }),
  ).toBeDisabled();
});

test("failed preview opening closes its blank tab and explains recovery", async ({
  page,
}) => {
  await page.addInitScript(() => {
    window.open = (() => ({
      opener: null,
      close: () => {
        (window as unknown as Record<string, unknown>).blankTabClosed = true;
      },
    })) as typeof window.open;
  });
  await page.route("**/ui-api/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/ui-api/projects")
      return route.fulfill({ json: { items: [{ id: "A", name: "Outputs" }] } });
    if (path === "/ui-api/workspaces")
      return route.fulfill({
        json: {
          items: [
            {
              id: "ws_retained",
              project_id: "A",
              name: "Saved gallery",
              state: "READY",
              retained_preview: {
                service_id: "service_one",
                expires_at: 2000000000,
              },
            },
          ],
        },
      });
    if (path.endsWith("/services/service_one"))
      return route.fulfill({
        json: { id: "service_one", state: "active", expires_at: 2000000000 },
      });
    if (path.endsWith("/connect"))
      return route.fulfill({
        status: 503,
        json: { code: "RETAINED_PREVIEW_UNAVAILABLE" },
      });
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto("/app/projects/A/factory");
  await page.getByRole("button", { name: /Saved gallery/ }).click();
  await page.getByRole("button", { name: "Open isolated preview" }).click();
  await expect(page.getByRole("alert")).toContainText(
    "Check the source and preview workers",
  );
  expect(
    await page.evaluate(
      () => (window as unknown as Record<string, unknown>).blankTabClosed,
    ),
  ).toBe(true);
  await expect(
    page.getByRole("link", { name: "Open retained preview in a new tab" }),
  ).toHaveCount(0);
});
