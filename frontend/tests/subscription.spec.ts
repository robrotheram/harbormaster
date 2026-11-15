import { test, expect } from "@playwright/test";

test("save a private subscription, load, unload, and delete without forwarding refresh tokens", async ({
  page,
}) => {
  let saved = false;
  let loaded = false;
  let imported: unknown;
  const expiry = Math.floor(Date.now() / 1000) + 3600;
  await page.route("**/ui-api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const post = route.request().method() === "POST";
    if (path === "/ui-api/subscription") {
      if (post) {
        imported = route.request().postDataJSON();
        saved = true;
      }
      return route.fulfill({
        json: {
          saved,
          account_id: saved ? "my-account" : undefined,
          expires_at: expiry,
          workers: loaded ? ["ws_shared"] : [],
        },
      });
    }
    if (path.endsWith("/subscription/load")) {
      loaded = true;
      return route.fulfill({ json: { loaded } });
    }
    if (path.endsWith("/subscription/unload")) {
      loaded = false;
      return route.fulfill({ json: { loaded } });
    }
    if (path.endsWith("/subscription/delete")) {
      saved = false;
      loaded = false;
      return route.fulfill({ json: { saved } });
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
  await page.goto("/app/projects/default/workspaces");
  await page.getByRole("button", { name: "Subscription", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Your Codex subscription" });
  await expect(
    dialog.getByRole("button", { name: "Load into worker" }),
  ).toBeDisabled();
  await dialog.getByLabel("Import Codex login").setInputFiles({
    name: "auth.json",
    mimeType: "application/json",
    buffer: Buffer.from(
      JSON.stringify({
        auth_mode: "chatgpt",
        tokens: {
          access_token: "synthetic-access",
          account_id: "my-account",
          refresh_token: "must-not-leave-browser",
          id_token: "also-private",
        },
      }),
    ),
  });
  await expect(dialog.getByText("Your subscription was saved.")).toBeVisible();
  expect(imported).toEqual({
    auth: {
      auth_mode: "chatgpt",
      tokens: { access_token: "synthetic-access", account_id: "my-account" },
    },
  });
  await dialog.getByRole("button", { name: "Load into worker" }).click();
  await expect(dialog.getByText("Loaded into this worker.")).toBeVisible();
  await dialog.getByRole("button", { name: "Unload from worker" }).click();
  await expect(dialog.getByText("Not loaded into this worker.")).toBeVisible();
  await dialog
    .getByRole("button", { name: "Delete saved subscription" })
    .click();
  await expect(dialog.getByText("No subscription saved yet.")).toBeVisible();
});
