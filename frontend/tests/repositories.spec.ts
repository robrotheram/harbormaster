import { test, expect } from "@playwright/test";

test("repository connections are platform-wide and imports choose a project", async ({
  page,
}) => {
  const calls: { path: string; body: Record<string, unknown> }[] = [];
  await page.route("**/ui-api/**", async (route) => {
    const req = route.request(),
      url = new URL(req.url()),
      path = url.pathname;
    if (req.method() === "POST") {
      const body = req.postDataJSON();
      calls.push({ path, body });
      if (path.endsWith("/import"))
        return route.fulfill({
          json: { id: "work_import", project_id: body.project_id },
        });
    }
    if (path === "/ui-api/projects")
      return route.fulfill({
        json: {
          items: [
            { id: "project_a", name: "Alpha" },
            { id: "project_b", name: "Beta" },
          ],
        },
      });
    if (path === "/ui-api/forge-hosts")
      return route.fulfill({
        json: {
          items: [
            {
              name: "office",
              provider: "gitlab",
              api_url: "https://gitlab.example/api/v4",
            },
          ],
        },
      });
    if (path === "/ui-api/forge-connections") {
      expect(url.searchParams.has("project_id")).toBe(false);
      return route.fulfill({
        json: {
          items: [{ id: "forge_one", host: "office", repository: "team/repo" }],
        },
      });
    }
    if (path.endsWith("/issues")) {
      expect(url.searchParams.has("project_id")).toBe(false);
      return route.fulfill({
        json: {
          items: [
            {
              number: 1,
              title: "Fix checkout",
              body: "",
              state: "open",
              labels: [],
            },
          ],
        },
      });
    }
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto("/app/repositories");
  await expect(
    page.getByRole("heading", { name: "Repository connections" }),
  ).toBeVisible();
  await expect(
    page.locator(".project-section-nav").filter({ hasText: "Repositories" }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Platform", exact: true }).click();
  await expect(
    page.getByRole("menuitem", { name: "Repositories" }),
  ).toBeVisible();
  await expect(
    page.getByRole("menuitem", { name: "Connection profiles" }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: "Browse team/repo tickets" }).click();
  const importButton = page.getByRole("button", { name: "Import to factory" });
  await expect(importButton).toBeDisabled();
  await page.getByLabel("Import into project").selectOption("project_b");
  await importButton.click();
  await expect
    .poll(() => calls)
    .toContainEqual({
      path: "/ui-api/forge-connections/forge_one/issues/1/import",
      body: { project_id: "project_b" },
    });
  await expect(
    page.getByRole("link", { name: "Open work to start it" }),
  ).toHaveAttribute("href", /projects\/project_b\/work/);
});

test("accounts can be configured and reauthenticated in the UI", async ({
  page,
}) => {
  let accounts: Record<string, unknown>[] = [];
  const submissions: Record<string, unknown>[] = [];
  await page.route("**/ui-api/**", async (route) => {
    const req = route.request(),
      path = new URL(req.url()).pathname;
    if (path === "/ui-api/forge-hosts") {
      if (req.method() === "POST") {
        const body = req.postDataJSON();
        submissions.push(body);
        if (body.token === "invalid-token")
          return route.fulfill({
            status: 502,
            json: { code: "FORGE_HTTP_401" },
          });
        const value = {
          name: body.name,
          provider: body.provider,
          api_url: "https://office.example/gitlab/api/v4",
          managed: true,
          revision: body.revision + 1,
        };
        accounts = [value];
        return route.fulfill({ json: value });
      }
      return route.fulfill({ json: { items: accounts } });
    }
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto("/app/repositories");
  await page.getByRole("button", { name: "Add account", exact: true }).click();
  await page.getByLabel("Account name", { exact: true }).fill("Office GitLab");
  await page
    .getByLabel("Provider", { exact: true })
    .selectOption("self-hosted");
  await page
    .getByLabel("GitLab address", { exact: true })
    .fill("https://office.example/gitlab");
  const token = page.getByLabel("Access token", { exact: true });
  await expect(token).toHaveAttribute("type", "password");
  await token.fill("invalid-token");
  await page.getByRole("button", { name: "Test & save account" }).click();
  await expect(page.getByRole("alert")).toContainText("rejected");
  await token.fill("synthetic-good-token");
  await page.getByRole("button", { name: "Test & save account" }).click();
  await expect(page.getByRole("status")).toContainText(
    "Office GitLab verified and saved",
  );
  await expect(page.getByLabel("Account", { exact: true })).toHaveValue(
    "Office GitLab",
  );
  await expect(page.getByLabel("Access token", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "Update token" }).click();
  await expect(page.getByLabel("Account name", { exact: true })).toBeDisabled();
  await expect(
    page.getByLabel("GitLab address", { exact: true }),
  ).toBeDisabled();
  await expect(page.getByLabel("Access token", { exact: true })).toHaveValue(
    "",
  );
  await page
    .getByLabel("Access token", { exact: true })
    .fill("synthetic-rotated-token");
  await page.getByRole("button", { name: "Test & save account" }).click();
  await expect.poll(() => submissions.at(-1)?.revision).toBe(1);
  expect(submissions[1]).toMatchObject({
    name: "Office GitLab",
    provider: "gitlab",
    api_url: "https://office.example/gitlab",
    token: "synthetic-good-token",
    revision: 0,
  });
  expect(
    await page.evaluate(() => Object.values(localStorage).join(" ")),
  ).not.toContain("synthetic-good-token");
});

test("disconnecting a repository confirms the target and preserves context after failure", async ({
  page,
}) => {
  let attempts = 0;
  let removed = false;
  await page.route("**/ui-api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/ui-api/forge-connections/forge_one/delete") {
      attempts++;
      if (attempts === 1)
        return route.fulfill({
          status: 503,
          json: { code: "STORAGE_UNAVAILABLE" },
        });
      removed = true;
      return route.fulfill({ json: { deleted: true } });
    }
    if (path === "/ui-api/forge-connections")
      return route.fulfill({
        json: {
          items: removed
            ? []
            : [{ id: "forge_one", repository: "team/repo", host: "office" }],
        },
      });
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto("/app/repositories");
  const disconnect = page.getByRole("button", {
    name: "Disconnect team/repo",
    exact: true,
  });
  await disconnect.click();
  const dialog = page.getByRole("dialog", { name: "Disconnect repository?" });
  await expect(dialog).toContainText("team/repo");
  await expect(dialog).toContainText("Imported work remains");
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  expect(attempts).toBe(0);
  await expect(disconnect).toBeFocused();
  await disconnect.click();
  await dialog
    .getByRole("button", { name: "Disconnect repository", exact: true })
    .click();
  await expect(dialog.getByRole("alert")).toContainText(
    "could not access its saved data",
  );
  await expect(dialog).toContainText("team/repo");
  await dialog
    .getByRole("button", { name: "Disconnect repository", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  await expect(disconnect).toHaveCount(0);
});

test("repository access checks lock the submitted fields and keep failed drafts", async ({
  page,
}) => {
  let writes = 0;
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/ui-api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (route.request().method() === "POST") {
      writes++;
      expect(route.request().postDataJSON()).toEqual({
        host: "office",
        repository: "team/project",
      });
      await gate;
      return route.fulfill({
        status: 503,
        json: { code: "FORGE_UNAVAILABLE" },
      });
    }
    if (path.endsWith("/forge-hosts"))
      return route.fulfill({
        json: {
          items: [
            {
              name: "office",
              provider: "gitlab",
              api_url: "https://gitlab.example/api/v4",
            },
          ],
        },
      });
    return route.fulfill({ json: { items: [] } });
  });
  await page.goto("/app/repositories");
  const account = page.getByRole("combobox", { name: "Account", exact: true });
  const repository = page.getByRole("textbox", {
    name: "Repository",
    exact: true,
  });
  await account.selectOption("office");
  await repository.fill("   ");
  await repository.press("Enter");
  expect(writes).toBe(0);
  await repository.fill("team/project");
  await repository.press("Enter");
  await expect(repository).toBeDisabled();
  await expect(account).toBeDisabled();
  release();
  await expect(page.getByRole("alert")).toBeVisible();
  await expect(repository).toHaveValue("team/project");
  await expect(repository).toBeEnabled();
  expect(writes).toBe(1);
});
