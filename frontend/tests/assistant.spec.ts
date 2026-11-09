import { test, expect } from "@playwright/test";
test("starting the project assistant uses one backend workflow", async ({
  page,
}) => {
  const project = {
    id: "p_a",
    name: "Research",
    created_at: "2026-11-01T10:00:00Z",
  };
  const workspace = {
    id: "ws_" + "1".repeat(32),
    project_id: project.id,
    name: "Hermes",
    agent_kind: "hermes",
    profile_id: "general",
    state: "READY",
    ready: true,
    active_runs: 0,
    background_jobs: 0,
    runtime_type: "firecracker",
    isolation_status: "jailed-vm",
    network_status: "no-nic-vsock",
    cpu_limit: 1,
    memory_limit_bytes: 1073741824,
    created_at: project.created_at,
    updated_at: project.created_at,
  };
  const mutations: { path: string; body: unknown }[] = [];
  await page.route("**/ui-api/**", async (route) => {
    const req = route.request(),
      path = new URL(req.url()).pathname;
    if (req.method() === "POST")
      mutations.push({ path, body: req.postDataJSON() });
    const responses: Record<string, unknown> = {
      "/ui-api/projects": { items: [project] },
      "/ui-api/connection-profiles": { items: [] },
      "/ui-api/profiles": { items: [] },
      "/ui-api/workspaces": { items: [workspace] },
      "/ui-api/projects/p_a/hermes": {
        config: {
          project_id: project.id,
          provider: "openai-codex",
          model: "approved-model",
          mode: "workspace",
          connection_profile_id: "",
          matrix: null,
          revision: 1,
          updated_at: null,
        },
        saved: true,
        runtime_available: true,
        runtime_status: "available",
        matrix_available: true,
        matrix_state: "disconnected",
      },
      "/ui-api/projects/p_a/hermes/assistant": workspace,
    };
    await route.fulfill({
      status: path in responses ? 200 : 404,
      contentType: "application/json",
      body: JSON.stringify(
        responses[path] || { code: "NOT_FOUND_OR_FORBIDDEN" },
      ),
    });
  });
  await page.goto("/app/projects/p_a/hermes");
  await page
    .getByRole("button", { name: "Start Hermes workspace", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Workspaces", exact: true }),
  ).toBeVisible();
  expect(mutations).toHaveLength(1);
  expect(mutations[0]).toMatchObject({
    path: "/ui-api/projects/p_a/hermes/assistant",
    body: { idempotency_key: expect.any(String) },
  });
});
