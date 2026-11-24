import { expect, test } from "@playwright/test";
import type {
  SetupInventory,
  SetupOperation,
} from "../src/features/workspaces/setup-api";

test("worker setup reviews pinned tools and skills before applying them", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const worker = {
    id: "ws_setup",
    project_id: "project_setup",
    name: "Storefront",
    state: "READY",
    ready: true,
    agent_kind: "t3",
    branch: "feature/checkout",
  };
  const inventory: SetupInventory = {
    branch: worker.branch,
    tool_config: "mise.toml",
    tool_lock: "mise.lock",
    tools_need_sync: false,
    skill_lock: false,
    tools: [],
    skills: [],
    catalog: ["node"],
    skills_target: "Codex",
  };
  const changes: SetupOperation[] = [];
  let lookupBlocked = true;
  const commit = "a".repeat(40);

  await page.route("**/ui-api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/ui-api/projects") {
      return route.fulfill({
        json: { items: [{ id: worker.project_id, name: "Storefront" }] },
      });
    }
    if (path === "/ui-api/workspaces") {
      return route.fulfill({ json: { items: [worker] } });
    }
    if (path !== `/ui-api/workspaces/${worker.id}/setup`) {
      return route.fulfill({ json: { items: [] } });
    }
    if (route.request().method() === "GET") {
      return route.fulfill({ json: inventory });
    }

    const change = route.request().postDataJSON() as SetupOperation;
    changes.push(change);
    switch (change.operation) {
      case "tool-set":
        inventory.job = {
          id: "setup_tool",
          state: "review",
          review: {
            changes: ["Use node 24.11.1 on this worker.", "Update mise.lock."],
          },
        };
        break;
      case "skill-discover":
        if (lookupBlocked) {
          lookupBlocked = false;
          inventory.job = {
            id: "setup_blocked",
            state: "failed",
            code: "SETUP_NETWORK_BLOCKED",
            request: change,
          };
          break;
        }
        inventory.job = {
          id: "setup_search",
          state: "discovered",
          discovery: { source: change.source, names: ["review-ui"], commit },
        };
        break;
      case "skill-add":
        inventory.job = {
          id: "setup_skill",
          state: "review",
          review: {
            changes: [`Install review-ui at ${change.commit}.`],
            preview: "# Review UI\nCheck keyboard navigation.",
          },
        };
        break;
      case "apply":
        if (change.plan_id === "setup_tool") {
          inventory.tools = [
            { name: "node", requested: "24.11.1", installed: "24.11.1" },
          ];
        } else {
          inventory.skill_lock = true;
          inventory.skills = [
            {
              name: "review-ui",
              state: "installed",
              source: "owner/repo",
              commit,
            },
          ];
        }
        inventory.job = { id: change.plan_id, state: "applied" };
        return route.fulfill({ json: inventory.job });
      default:
        throw new Error(`Unexpected setup request ${change.operation}`);
    }
    return route.fulfill({
      status: 202,
      json: { id: inventory.job.id, state: "preparing" },
    });
  });

  await page.goto(`/app/projects/${worker.project_id}/workspaces`);
  await page
    .getByRole("link", { name: "Open worker Storefront", exact: true })
    .click();
  const tabs = page.getByRole("navigation", { name: "Worker sections" });
  const openSetup = () =>
    tabs.getByRole("link", { name: "Environment", exact: true }).click();
  const dialog = page.getByRole("region", { name: "Worker environment" });
  await expect(
    tabs.getByRole("link", { name: "Terminal", exact: true }),
  ).toHaveAttribute("aria-current", "page");
  await openSetup();
  const toolsPanel = dialog.getByRole("button", { name: /^Software tools/ });
  const skillsPanel = dialog.getByRole("button", { name: /^Agent skills/ });
  await expect(toolsPanel).toHaveAttribute("aria-expanded", "true");
  await expect(skillsPanel).toHaveAttribute("aria-expanded", "false");
  await dialog.getByRole("button", { name: "Add tool" }).click();
  await expect(dialog.getByRole("combobox", { name: "Tool" })).toBeFocused();
  await dialog.getByRole("combobox", { name: "Tool" }).selectOption("node");
  await dialog.getByRole("textbox", { name: "Version" }).fill("24.11.1");
  await toolsPanel.focus();
  await page.keyboard.press("ArrowDown");
  await expect(skillsPanel).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(toolsPanel).toHaveAttribute("aria-expanded", "false");
  await expect(skillsPanel).toHaveAttribute("aria-expanded", "true");
  await expect(dialog.getByRole("textbox", { name: "Version" })).toBeHidden();
  await toolsPanel.click();
  await expect(dialog.getByRole("textbox", { name: "Version" })).toHaveValue(
    "24.11.1",
  );
  await dialog.getByRole("button", { name: "Review change" }).click();
  await expect(dialog.getByText("Update mise.lock.")).toBeVisible();
  expect(changes.map((change) => change.operation)).toEqual(["tool-set"]);

  await tabs.getByRole("link", { name: "Terminal", exact: true }).click();
  await openSetup();
  await page.reload();
  await expect(
    tabs.getByRole("link", { name: "Environment", exact: true }),
  ).toHaveAttribute("aria-current", "page");
  await expect(
    tabs.getByRole("link", { name: "Terminal", exact: true }),
  ).not.toHaveAttribute("aria-current", "page");
  await dialog.getByRole("button", { name: "Apply changes" }).click();
  await expect(dialog.getByText("Active 24.11.1")).toBeVisible();

  await dialog.getByRole("button", { name: /^Agent skills/ }).click();
  await dialog.getByRole("button", { name: "Add skill" }).click();
  await dialog
    .getByRole("textbox", { name: "GitHub repository" })
    .fill("owner/repo");
  await dialog.getByRole("button", { name: "Find skills" }).click();
  await expect(dialog.getByRole("alert")).toContainText(
    "connection policy blocked",
  );
  await expect(
    dialog.getByRole("button", { name: "Connection settings" }),
  ).toBeVisible();
  await tabs.getByRole("link", { name: "Terminal", exact: true }).click();
  await openSetup();
  await dialog.getByRole("button", { name: "Try again", exact: true }).click();
  await dialog.getByRole("button", { name: /^Agent skills/ }).click();
  await dialog.getByRole("button", { name: "Review skill" }).click();
  await dialog.getByText("Preview skill instructions").click();
  await expect(dialog.getByText("Check keyboard navigation.")).toBeVisible();
  expect(changes.find((change) => change.operation === "skill-add")).toEqual({
    operation: "skill-add",
    source: "owner/repo",
    name: "review-ui",
    commit,
  });
  await dialog.getByRole("button", { name: "Apply changes" }).click();
  await expect(dialog.getByText("Installed", { exact: true })).toBeVisible();
  await page.setViewportSize({ width: 896, height: 900 });
  const tabsWidth = (await tabs.boundingBox())!.width;
  expect((await dialog.boundingBox())!.width).toBeCloseTo(tabsWidth, 0);
  await tabs.getByRole("link", { name: "Settings", exact: true }).click();
  await expect(
    page.getByRole("region", { name: "Worker settings" }),
  ).toBeVisible();
  for (const name of ["Idle shutdown", "Network access", "Worker controls"]) {
    await expect(page.getByRole("region", { name, exact: true })).toHaveClass(
      "settings-panel",
    );
  }
  expect(
    (await page.getByRole("region", { name: "Worker settings" }).boundingBox())!
      .width,
  ).toBeGreaterThan(tabsWidth - 50);
  await page.setViewportSize({ width: 320, height: 840 });
  await expect(tabs.getByRole("link", { name: "Activity" })).toBeInViewport();
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth),
  ).toBeLessThanOrEqual(320);
  await page.setViewportSize({ width: 896, height: 900 });
  await openSetup();
  expect(
    await dialog.evaluate(
      ({ scrollWidth, clientWidth }) => scrollWidth <= clientWidth,
    ),
  ).toBe(true);
});
