import { defineConfig } from "@playwright/test";

const playerURL = "http://127.0.0.1:5173";

export default defineConfig({
  testDir: "./e2e",
  outputDir: "output/playwright/test-results",
  snapshotPathTemplate: "{testDir}/{testFilePath}-snapshots/{arg}-{projectName}{ext}",
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 2 : undefined,
  reporter: [["list"], ["html", { outputFolder: "output/playwright/report", open: "never" }]],
  expect: { timeout: 8_000, toHaveScreenshot: { animations: "disabled", caret: "hide", maxDiffPixelRatio: 0.01 } },
  use: {
    baseURL: playerURL,
    locale: "zh-CN",
    timezoneId: "Asia/Shanghai",
    colorScheme: "dark",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  projects: [
    { name: "desktop-1440", use: { viewport: { width: 1440, height: 900 } } },
    { name: "desktop-1280", use: { viewport: { width: 1280, height: 720 } } },
    { name: "mobile-portrait", use: { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true } },
    { name: "mobile-landscape", use: { viewport: { width: 844, height: 390 }, isMobile: true, hasTouch: true } },
  ],
  webServer: [
    { command: "npm run dev:web", url: playerURL, reuseExistingServer: !process.env.CI, timeout: 120_000 },
    { command: "npm run dev -w @royal-flush/admin -- --port 43174", url: "http://127.0.0.1:43174", reuseExistingServer: !process.env.CI, timeout: 120_000 },
    { command: "npm run dev:e2e-api", url: "http://127.0.0.1:5175", reuseExistingServer: !process.env.CI, timeout: 120_000 },
  ],
});
