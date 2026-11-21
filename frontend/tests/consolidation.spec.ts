import { test, expect } from "@playwright/test";

test("Workspace keeps drafts through exact-revision review and Overview uses saved titles", async ({
  page,
}) => {
  const item = {
    id: "work_one",
    project_id: "A",
    workspace_id: "ws_one",
    specification_path: "work-requests/one.md",
    specification_digest: "a".repeat(64),
    kind: "research",
    state: "awaiting_review",
    latest_attempt_id: "attempt_new",
    created_at: "2026-11-17T10:00:00Z",
    updated_at: "2026-11-17T11:00:00Z",
  };
  const old = {
    id: "attempt_old",
    work_item_id: item.id,
    workspace_id: "ws_one",
    task_id: "task_old",
    state: "awaiting_review",
    review_state: "accepted",
    created_at: item.created_at,
    updated_at: item.created_at,
    artifacts: [],
  };
  const latest = {
    ...old,
    id: "attempt_new",
    task_id: "task_new",
    review_state: "",
    created_at: item.updated_at,
  };
  await page.route("**/ui-api/**", async (route) => {
    const path = new URL(route.request().url()).pathname.replace("/ui-api", "");
    let json: unknown = { items: [] };
    if (path === "/projects")
      json = {
        items: [{ id: "A", name: "Product", created_at: item.created_at }],
      };
    if (path === "/work-items") json = { items: [item] };
    if (path === "/work-items/work_one") json = item;
    if (path === "/work-items/work_one/attempts")
      json = { items: [old, latest] };
    if (path === "/board")
      json = {
        items: [
          {
            id: item.id,
            project_id: "A",
            revision: 1,
            stage: "review",
            title: "Workspace invitations",
            owner: "Maya",
            attention: false,
            dirty: false,
            work: item,
          },
        ],
      };
    if (path.endsWith("/instruction")) {
      return route.fulfill({
        status: 409,
        json: { code: "HISTORY_WORKSPACE_STOPPED" },
      });
    }
    return route.fulfill({ json });
  });
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/app/projects/A/work?work=work_one");
  await expect(
    page.getByRole("heading", { name: "Workspace invitations" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Workspace", exact: true }),
  ).toBeVisible();
  const area = await page.locator(".conversation-session").boundingBox();
  expect(area!.width).toBeGreaterThanOrEqual(1440 * 0.75);
  const composer = page.locator(".conversation-composer textarea");
  await composer.fill("Keep this unsent request");
  await page.getByRole("separator", { name: "Resize thread list" }).focus();
  await page.keyboard.press("ArrowRight");
  await expect(
    page.getByRole("separator", { name: "Resize thread list" }),
  ).toHaveAttribute("aria-valuenow", "256");
  await page.getByRole("button", { name: "Workspace", exact: true }).click();
  await expect(composer).toBeVisible();
  await expect(
    page.getByRole("region", { name: "Conversation and agent activity" }),
  ).toBeHidden();
  await page.getByRole("button", { name: "Conversation", exact: true }).click();
  await expect(composer).toHaveValue("Keep this unsent request");

  await page.getByRole("button", { name: "Workspace", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Workspace invitations" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Approve this revision" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Conversation", exact: true }).click();
  await expect(composer).toHaveValue("Keep this unsent request");
  await page.getByRole("link", { name: "Overview", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();
  await expect(
    page.getByRole("cell", { name: "Maya", exact: true }),
  ).toBeVisible();
  await page.getByRole("link", { name: "Review", exact: true }).click();
  await expect(page).toHaveURL(/view=workspace/);
  await page.goto("/app/projects/A/reviews?work=work_one&attempt=attempt_old");
  await expect(page).toHaveURL(/\/work\?/);
  await expect(page).toHaveURL(/attempt=attempt_old/);
  await expect(page).toHaveURL(/view=workspace/);
  await expect(
    page.getByRole("link", { name: "Reviews", exact: true }),
  ).toHaveCount(0);
  await expect(page.getByText("This revision was approved.")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Approve this revision" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("heading", { name: "Workspace invitations" }),
  ).toBeVisible();
});
