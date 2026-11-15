import { test, expect } from "@playwright/test";

test("manage multiple personal accounts and independently select worker providers", async ({
  page,
}) => {
  type Saved = {
    id: string;
    name: string;
    provider: string;
    expires_at: number;
    expired: boolean;
    workers: string[];
  };
  let items: Saved[] = [];
  const imports: Record<string, unknown>[] = [];
  const expiry = Math.floor(Date.now() / 1000) + 3600;
  let serial = 0;
  await page.route("**/ui-api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const post = route.request().method() === "POST";
    if (path === "/ui-api/subscriptions") {
      if (post) {
        const input = route.request().postDataJSON();
        imports.push(input);
        items.push({
          id: `sub_${++serial}`,
          name: input.name,
          provider: input.provider,
          expires_at: expiry,
          expired: false,
          workers: [],
        });
      }
      return route.fulfill({ json: { items } });
    }
    if (path.startsWith("/ui-api/subscriptions/")) {
      const id = path.split("/")[3];
      if (path.endsWith("/delete")) items = items.filter((s) => s.id !== id);
      else {
        const input = route.request().postDataJSON();
        imports.push(input);
        items = items.map((s) =>
          s.id === id ? { ...s, name: input.name } : s,
        );
      }
      return route.fulfill({ json: { saved: true } });
    }
    if (
      path.endsWith("/subscription/load") ||
      path.endsWith("/subscription/unload")
    ) {
      const input = route.request().postDataJSON();
      items = items.map((s) =>
        s.provider === input.provider
          ? {
              ...s,
              workers:
                path.endsWith("/load") && s.id === input.subscription_id
                  ? ["ws_shared"]
                  : [],
            }
          : s,
      );
      return route.fulfill({ json: { loaded: true } });
    }
    if (path === "/ui-api/projects")
      return route.fulfill({
        json: { items: [{ id: "default", name: "Default" }] },
      });
    if (path === "/ui-api/workspaces")
      return route.fulfill({
        json: {
          items: [
            {
              id: "ws_shared",
              project_id: "default",
              name: "Shared workspace",
              shared: true,
              agent_kind: "t3",
              state: "READY",
              ready: true,
              active_runs: 0,
              background_jobs: 0,
              cpu_limit: 1,
              memory_limit_bytes: 1073741824,
              connections: { mode: "blocked", destinations: [] },
            },
          ],
        },
      });
    return route.fulfill({ json: { items: [] } });
  });
  const codex = {
    auth_mode: "chatgpt",
    tokens: {
      access_token: "synthetic-access",
      account_id: "my-account",
      refresh_token: "must-not-leave-browser",
      id_token: "also-private",
    },
  };
  const claude = {
    claudeAiOauth: {
      accessToken: "sk-ant-oat-synthetic",
      expiresAt: expiry * 1000,
      scopes: ["user:inference"],
      refreshToken: "must-not-leave-browser",
    },
  };
  await page.goto("/app/connections");
  await expect(
    page.getByRole("heading", { name: "Connections", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("No subscriptions saved", { exact: true }),
  ).toBeVisible();
  for (const [name, provider] of [
    ["Personal ChatGPT", "codex"],
    ["Work ChatGPT", "codex"],
    ["Personal Claude", "claude"],
    ["Work Claude", "claude"],
  ]) {
    await page.getByRole("button", { name: "Add subscription" }).click();
    const dialog = page.getByRole("dialog", { name: "Add subscription" });
    await dialog.getByLabel("Name", { exact: true }).fill(name);
    await dialog.getByLabel("Provider", { exact: true }).selectOption(provider);
    await dialog.getByLabel("Login file").setInputFiles({
      name: provider === "codex" ? "auth.json" : ".credentials.json",
      mimeType: "application/json",
      buffer: Buffer.from(
        JSON.stringify(provider === "codex" ? codex : claude),
      ),
    });
    await dialog.getByRole("button", { name: "Save subscription" }).click();
    await expect(
      page.getByRole("article", { name, exact: true }),
    ).toBeVisible();
  }
  expect(JSON.stringify(imports)).not.toContain("must-not-leave-browser");
  expect(JSON.stringify(imports)).not.toContain("also-private");
  expect(imports[0]).toEqual({
    name: "Personal ChatGPT",
    provider: "codex",
    auth: {
      auth_mode: "chatgpt",
      tokens: { access_token: "synthetic-access", account_id: "my-account" },
    },
  });
  expect(imports[2]).toEqual({
    name: "Personal Claude",
    provider: "claude",
    auth: {
      claudeAiOauth: {
        accessToken: "sk-ant-oat-synthetic",
        expiresAt: expiry * 1000,
        scopes: ["user:inference"],
      },
    },
  });
  await page
    .getByRole("article", { name: "Work ChatGPT", exact: true })
    .getByRole("button", { name: "Edit" })
    .click();
  const edit = page.getByRole("dialog", { name: "Edit subscription" });
  await expect(edit.getByLabel("Provider", { exact: true })).toBeDisabled();
  await edit.getByLabel("Name", { exact: true }).fill("Company ChatGPT");
  await edit.getByRole("button", { name: "Save subscription" }).click();
  await expect(
    page.getByRole("article", { name: "Company ChatGPT", exact: true }),
  ).toBeVisible();
  expect(imports[4]).not.toHaveProperty("auth");
  await page
    .getByRole("tablist", { name: "Connection categories" })
    .getByRole("tab", { name: "Repositories" })
    .click();
  await expect(
    page.getByRole("heading", { name: "Repository connections" }),
  ).toBeVisible();
  await expect(page).toHaveURL(/tab=repositories/);

  await page.goto("/app/projects/default/workspaces");
  await page.getByRole("button", { name: "Subscription", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Worker subscriptions" });
  const chatgpt = dialog.getByRole("region", { name: "ChatGPT / Codex" });
  const anthropic = dialog.getByRole("region", { name: "Claude", exact: true });
  await expect(
    chatgpt.getByRole("button", { name: "Load into worker" }),
  ).toBeDisabled();
  await chatgpt
    .getByLabel("Saved account")
    .selectOption({ label: "Personal ChatGPT" });
  await chatgpt.getByRole("button", { name: "Load into worker" }).click();
  await expect(chatgpt.getByText("Loaded: Personal ChatGPT")).toBeVisible();
  await anthropic
    .getByLabel("Saved account")
    .selectOption({ label: "Work Claude" });
  await anthropic.getByRole("button", { name: "Load into worker" }).click();
  await expect(anthropic.getByText("Loaded: Work Claude")).toBeVisible();
  await chatgpt
    .getByLabel("Saved account")
    .selectOption({ label: "Company ChatGPT" });
  await chatgpt.getByRole("button", { name: "Switch account" }).click();
  await expect(chatgpt.getByText("Loaded: Company ChatGPT")).toBeVisible();
  await expect(anthropic.getByText("Loaded: Work Claude")).toBeVisible();
  await anthropic.getByRole("button", { name: "Unload", exact: true }).click();
  await expect(anthropic.getByText("Subscription unloaded.")).toBeVisible();
  await expect(chatgpt.getByText("Loaded: Company ChatGPT")).toBeVisible();
  await dialog.getByRole("link", { name: "Manage subscriptions" }).click();
  await page
    .getByRole("article", { name: "Company ChatGPT", exact: true })
    .getByRole("button", { name: "Delete" })
    .click();
  await page
    .getByRole("dialog", { name: "Delete subscription" })
    .getByRole("button", { name: "Delete subscription" })
    .click();
  await expect(
    page.getByRole("article", { name: "Company ChatGPT", exact: true }),
  ).toHaveCount(0);
  await expect(page.getByRole("article")).toHaveCount(3);
  expect(items.every((s) => s.workers.length === 0)).toBe(true);
});

test("personal subscriptions remain available when the project list fails", async ({
  page,
}) => {
  await page.route("**/ui-api/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/ui-api/projects")
      return route.fulfill({
        status: 503,
        json: { code: "STORAGE_UNAVAILABLE" },
      });
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto("/app/connections");
  await expect(
    page.getByText("No subscriptions saved", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Add subscription" }).click();
  await expect(
    page.getByRole("dialog", { name: "Add subscription" }),
  ).toBeVisible();
});
