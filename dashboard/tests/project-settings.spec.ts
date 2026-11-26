import { test, expect } from "@playwright/test";

test("renames a project, preserves a failed draft, and redirects older Hermes settings links", async ({
  page,
}) => {
  let name = "Original";
  let fail = true;
  const requests: string[] = [];
  await page.route("**/dashboard-api/**", (r) => r.fulfill({ json: {} }));
  await page.route("**/ui-api/**", (r) => {
    const path = new URL(r.request().url()).pathname;
    requests.push(path);
    if (path === "/ui-api/projects")
      return r.fulfill({ json: { items: [{ id: "project_one", name }] } });
    if (path.endsWith("/rename")) {
      if (fail) {
        fail = false;
        return r.fulfill({
          status: 503,
          json: { code: "STORAGE_UNAVAILABLE" },
        });
      }
      name = r.request().postDataJSON().name;
      return r.fulfill({ json: { id: "project_one", name } });
    }
    if (path.endsWith("/hermes"))
      return r.fulfill({ status: 503, json: { code: "STORAGE_UNAVAILABLE" } });
    return r.fulfill({ json: { items: [] } });
  });
  await page.goto("/app/projects/project_one/settings");
  for (const title of [
    "Project details",
    "Project members",
    "Delete project",
  ]) {
    await expect(page.getByRole("region", { name: title })).toHaveClass(
      "settings-panel",
    );
  }
  await page.getByLabel("Project name", { exact: true }).fill(" ");
  await expect(page.getByRole("button", { name: "Save name" })).toBeDisabled();
  await page
    .getByLabel("Project name", { exact: true })
    .fill("Renamed project");
  await page.getByRole("button", { name: "Save name" }).click();
  await expect(page.getByRole("alert")).toBeVisible();
  await expect(page.getByLabel("Project name", { exact: true })).toHaveValue(
    "Renamed project",
  );
  await page.getByRole("button", { name: "Save name" }).click();
  await expect(page.getByRole("status")).toHaveText("Project renamed.");
  await expect(
    page.getByRole("button", { name: "Switch project: Renamed project" }),
  ).toBeVisible();
  await page.reload();
  await expect(page.getByLabel("Project name", { exact: true })).toHaveValue(
    "Renamed project",
  );
  await expect(
    page.getByRole("tab", { name: "Hermes setup", exact: true }),
  ).toHaveCount(0);
  expect(requests).not.toContain("/ui-api/projects/project_one/hermes");
  for (const path of ["settings?tab=hermes", "hermes"]) {
    await page.goto(`/app/projects/project_one/${path}`);
    await expect(page).toHaveURL(/assistant\?tab=hermes$/);
    await expect(
      page.getByRole("tab", { name: "Hermes setup" }),
    ).toHaveAttribute("aria-selected", "true");
    await expect(
      page.getByRole("heading", { name: "Project assistant" }),
    ).toBeVisible();
    await expect(page.getByRole("alert")).toBeVisible();
  }
  expect(requests).toContain("/ui-api/projects/project_one/hermes");
});
