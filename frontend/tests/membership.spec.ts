import { test, expect } from "@playwright/test";

test("project administrators invite and revoke with exact revision payloads", async ({
  page,
}) => {
  const writes: Record<string, unknown>[] = [];
  const members = [
    {
      project_id: "A",
      email: "alice@example.com",
      user_id: "user_alice",
      role: "admin",
      status: "active",
      revision: 1,
    },
  ];
  await page.route("**/auth/session", (route) =>
    route.fulfill({
      json: {
        status: "authenticated",
        user: { id: "alice", email: "alice@example.com", name: "Alice" },
      },
    }),
  );
  await page.route("**/ui-api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/ui-api/projects")
      return route.fulfill({
        json: {
          items: [
            { id: "A", name: "Product", created_at: "2026-11-17T10:00:00Z" },
          ],
        },
      });
    if (path.endsWith("/members")) {
      if (route.request().method() === "POST") {
        const body = route.request().postDataJSON();
        writes.push(body);
        if (body.action === "invite")
          members.push({
            project_id: "A",
            email: body.email,
            user_id: "",
            role: body.role,
            status: "invited",
            revision: 1,
          });
        else members.find((m) => m.email === body.email)!.status = "revoked";
        return route.fulfill({ json: {} });
      }
      return route.fulfill({
        json: { items: members, can_manage: true, user_id: "user_alice" },
      });
    }
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto("/app/projects/A/settings");
  await page
    .getByLabel("Email address", { exact: true })
    .fill("bob@example.com");
  await page
    .getByRole("button", { name: "Invite member", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Revoke access for bob@example.com" }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Revoke access for bob@example.com" })
    .click();
  await expect(
    page.getByRole("button", { name: "Revoke access for bob@example.com" }),
  ).toHaveCount(0);
  expect(writes).toEqual([
    { email: "bob@example.com", role: "member", revision: 0, action: "invite" },
    { email: "bob@example.com", role: "member", revision: 1, action: "revoke" },
  ]);
});

test("an invited user explicitly accepts before the project appears", async ({
  page,
}) => {
  let accepted = false;
  await page.route("**/auth/session", (route) =>
    route.fulfill({
      json: {
        status: "authenticated",
        user: { id: "bob", email: "bob@example.com", name: "Bob" },
      },
    }),
  );
  await page.route("**/ui-api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/ui-api/projects")
      return route.fulfill({
        json: {
          items: accepted
            ? [{ id: "A", name: "Product", created_at: "2026-11-17T10:00:00Z" }]
            : [],
        },
      });
    if (path === "/ui-api/project-invitations")
      return route.fulfill({
        json: {
          items: accepted
            ? []
            : [
                {
                  project_id: "A",
                  email: "bob@example.com",
                  role: "member",
                  status: "invited",
                  revision: 3,
                },
              ],
        },
      });
    if (path.endsWith("/accept-invitation")) {
      expect(route.request().postDataJSON()).toEqual({ revision: 3 });
      accepted = true;
      return route.fulfill({ json: { accepted: true } });
    }
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto("/app/");
  await expect(
    page.getByRole("button", { name: "Accept invitation", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Accept invitation", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Accept invitation", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("heading", { name: "Product", exact: true }),
  ).toBeVisible();
});

test("an assigned reviewer requests revision without dispatching the controller's agent", async ({
  page,
}) => {
  const work = {
    id: "work_one",
    project_id: "A",
    workspace_id: "ws_one",
    specification_path: "request.md",
    kind: "research",
    state: "awaiting_review",
    latest_attempt_id: "attempt_one",
    controller_id: "user_controller",
    reviewer_id: "user_reviewer",
    created_at: "2026-11-17T10:00:00Z",
    updated_at: "2026-11-17T10:00:00Z",
  };
  const attempt = {
    id: "attempt_one",
    work_item_id: work.id,
    workspace_id: work.workspace_id,
    state: "awaiting_review",
    review_state: "",
    created_at: work.created_at,
    updated_at: work.updated_at,
    artifacts: [],
  };
  const writes: { path: string; body: unknown }[] = [];
  await page.route("**/auth/session", (route) =>
    route.fulfill({
      json: {
        status: "authenticated",
        user: {
          id: "reviewer",
          email: "reviewer@example.com",
          name: "Reviewer",
        },
      },
    }),
  );
  await page.route("**/ui-api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (route.request().method() === "POST") {
      writes.push({ path, body: route.request().postDataJSON() });
      if (path.endsWith("/review")) {
        work.state = "changes_requested";
        attempt.review_state = "changes_requested";
        return route.fulfill({ json: work });
      }
      return route.fulfill({ json: {} });
    }
    if (path === "/ui-api/projects")
      return route.fulfill({
        json: {
          items: [{ id: "A", name: "Product", created_at: work.created_at }],
        },
      });
    if (path === "/ui-api/work-items")
      return route.fulfill({ json: { items: [work] } });
    if (path.endsWith("/attempts"))
      return route.fulfill({ json: { items: [attempt] } });
    if (path.endsWith("/members"))
      return route.fulfill({
        json: {
          items: [
            {
              user_id: "user_reviewer",
              email: "reviewer@example.com",
              role: "member",
              status: "active",
            },
          ],
          user_id: "user_reviewer",
          can_manage: false,
        },
      });
    if (path === "/ui-api/board")
      return route.fulfill({
        json: {
          items: [
            {
              id: work.id,
              project_id: "A",
              title: "Review this report",
              work,
              stage: "review",
            },
          ],
        },
      });
    if (path.endsWith("/instruction"))
      return route.fulfill({
        status: 409,
        json: { code: "HISTORY_WORKSPACE_STOPPED" },
      });
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto("/app/projects/A/reviews?work=work_one&attempt=attempt_one");
  await expect(
    page.getByRole("button", { name: "Approve this revision" }),
  ).toBeEnabled();
  await page
    .getByRole("button", { name: "Request changes", exact: true })
    .click();
  await expect(
    page.getByText(
      "Changes requested. The controller can prepare the next revision.",
    ),
  ).toBeVisible();
  expect(writes).toEqual([
    {
      path: "/ui-api/work-items/work_one/review",
      body: {
        project_id: "A",
        accepted: false,
        expected_attempt_id: "attempt_one",
      },
    },
  ]);
});
