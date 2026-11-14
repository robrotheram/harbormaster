import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "./tests",
  testMatch: "**/*.spec.*",
  timeout: 120000,
  fullyParallel: false,
  workers: 1,
  use: {
    baseURL: "http://127.0.0.1:4173",
    headless: true,
    actionTimeout: 15000,
    trace: "retain-on-failure",
  },
  outputDir: "artifacts/test-results",
  reporter: [["list"]],
  webServer: {
    command: "npm run build && node tests/host.mjs",
    url: "http://127.0.0.1:4173/app/",
    reuseExistingServer: false,
    timeout: 30000,
  },
});
