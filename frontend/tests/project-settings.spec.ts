import { test, expect } from "@playwright/test";

test("renames a project, preserves a failed draft, and opens Hermes within settings", async ({
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
    page.getByRole("link", { name: "Hermes setup", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("tab", { name: "Hermes setup", exact: true }).click();
  await expect(page).toHaveURL(/settings\?tab=hermes/);
  await expect(page.getByRole("tab", { name: "Hermes setup" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(page.getByRole("alert")).toBeVisible();
  expect(requests).toContain("/ui-api/projects/project_one/hermes");
  await page.goto("/app/projects/project_one/hermes");
  await expect(page).toHaveURL(/settings\?tab=hermes/);
  await expect(
    page.getByRole("heading", { name: "Project settings" }),
  ).toBeVisible();
});
