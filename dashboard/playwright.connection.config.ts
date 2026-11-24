import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  testMatch: "connection-pages.browser.ts",
  timeout: 60000,
  workers: 1,
  use: { headless: true, actionTimeout: 10000, trace: "retain-on-failure" },
  outputDir: "artifacts/connection-tests",
  reporter: [["list"]],
  webServer: {
    command:
      "WORKBENCH_CONNECTION_TEST_ADDR=127.0.0.1:5191 go test ../internal/dashboard -run '^TestConnectionBrowserFixture$' -count=1 -timeout=15m -v",
    url: "http://127.0.0.1:5191/_fixture/ready",
    reuseExistingServer: false,
    timeout: 30000,
  },
});
