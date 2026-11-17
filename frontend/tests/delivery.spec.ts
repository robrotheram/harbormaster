import { test, expect } from "@playwright/test";

test("board and review use real work and bind decisions to the inspected attempt", async ({
  page,
}) => {
  const project = {
    id: "project_delivery",
    name: "Delivery",
    created_at: "2026-11-01T00:00:00Z",
  };
  const item = {
    id: "work_delivery",
    project_id: project.id,
    workspace_id: "ws_one",
    kind: "software",
    specification_path: "spec.md",
    state: "awaiting_review",
    latest_attempt_id: "attempt_one",
    created_at: project.created_at,
    updated_at: project.created_at,
  };
  let decision: unknown;
  await page.route("**/ui-api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/review")) {
      decision = route.request().postDataJSON();
      item.state = "completed";
      return route.fulfill({ json: item });
    }
    if (path === "/ui-api/projects")
      return route.fulfill({ json: { items: [project] } });
    if (path === "/ui-api/board")
      return route.fulfill({
        json: {
          items: [
            {
              id: item.id,
              project_id: project.id,
              stage: "review",
              revision: 0,
              dirty: false,
              attention: false,
              work: item,
            },
          ],
        },
      });
    if (path === "/ui-api/work-items")
      return route.fulfill({ json: { items: [item] } });
    if (path.endsWith("/instruction"))
      return route.fulfill({
        json: {
          project_id: project.id,
          work_item_id: item.id,
          instruction: "Fix checkout validation",
        },
      });
    if (path.endsWith("/attempts"))
      return route.fulfill({
        json: {
          items: [
            {
              id: "attempt_one",
              work_item_id: item.id,
              state: "awaiting_review",
              artifacts: [],
              created_at: project.created_at,
              updated_at: project.created_at,
            },
          ],
        },
      });
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto(`/app/projects/${project.id}/board`);
  await expect(
    page.getByRole("heading", { name: "Overview", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Fix checkout validation", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Fix checkout validation", exact: true })
    .click();
  await page.getByRole("link", { name: "Review output", exact: true }).click();
  await page.getByRole("button", { name: /Fix checkout validation/ }).click();
  const accept = page.getByRole("button", { name: "Approve" });
  await expect(accept).toBeEnabled();
  await accept.click();
  await expect
    .poll(() => decision)
    .toEqual({
      project_id: project.id,
      accepted: true,
      expected_attempt_id: "attempt_one",
    });
  await expect(
    page.getByText("Nothing waiting for review.", { exact: false }),
  ).toBeVisible();
});
