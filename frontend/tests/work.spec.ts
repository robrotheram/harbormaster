import { test, expect } from "@playwright/test";
test("runs work and reviews retained evidence", async ({ page }) => {
  const item = {
    id: "work-one",
    project_id: "A",
    workspace_id: "ws-one",
    specification_path: "spec.md",
    specification_digest: "sha256:test",
    kind: "research",
    state: "draft",
    created_at: "2026-11-10T12:00:00Z",
    updated_at: "2026-11-10T12:00:00Z",
    latest_attempt_id: "",
  };
  const posts: unknown[] = [];
  await page.route("**/ui-api/**", async (route) => {
    const req = route.request(),
      path = new URL(req.url()).pathname;
    let value: unknown = { items: [] };
    if (path === "/ui-api/projects")
      value = {
        items: [{ id: "A", name: "Research", created_at: item.created_at }],
      };
    if (path === "/ui-api/work-items") value = { items: [item] };
    if (path === "/ui-api/work-items/work-one") value = item;
    if (path.endsWith("/attempts"))
      value = {
        items: item.latest_attempt_id
          ? [
              {
                id: "attempt-one",
                work_item_id: item.id,
                workspace_id: item.workspace_id,
                state: "completed",
                created_at: item.created_at,
                updated_at: item.updated_at,
                artifacts: [
                  {
                    name: "Report",
                    content_url: "/v1/artifacts/report?project_id=A",
                  },
                ],
              },
            ]
          : [],
      };
    if (req.method() === "POST") {
      posts.push(req.postDataJSON());
      if (path.endsWith("/run")) {
        item.state = "awaiting_review";
        item.latest_attempt_id = "attempt-one";
      }
      if (path.endsWith("/review")) item.state = "accepted";
      value = item;
    }
    await route.fulfill({ json: value });
  });
  await page.goto("/app/projects/A/work");
  await page.getByRole("button", { name: "spec.md", exact: true }).click();
  await page.getByRole("button", { name: "Run work", exact: true }).click();
  await expect(
    page.getByRole("link", { name: "Report", exact: true }),
  ).toHaveAttribute("href", "/v1/artifacts/report?project_id=A");
  await page
    .getByRole("button", { name: "Accept result", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Accept result", exact: true }),
  ).toHaveCount(0);
  expect(posts).toEqual([
    { project_id: "A", idempotency_key: expect.any(String) },
    { project_id: "A", accepted: true },
  ]);
});
