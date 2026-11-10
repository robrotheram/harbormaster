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
  await page.getByLabel("Factory workspace").selectOption("ws_coding");
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
