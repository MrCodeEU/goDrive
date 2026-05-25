import { defineConfig, devices } from "@playwright/test";
import { existsSync } from "node:fs";

const chromiumPath =
  process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE ||
  "/var/home/mrcode/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome";

export default defineConfig({
  testDir: "./e2e",
  timeout: 30_000,
  expect: {
    timeout: 10_000
  },
  fullyParallel: false,
  workers: 1,
  reporter: process.env.CI ? [["github"], ["html", { open: "never" }]] : [["list"], ["html", { open: "never" }]],
  use: {
    baseURL: "http://127.0.0.1:15173",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
    launchOptions: existsSync(chromiumPath)
      ? {
          executablePath: chromiumPath,
          args: ["--no-sandbox"]
        }
      : {
          args: ["--no-sandbox"]
        }
  },
  webServer: [
    {
      command: "../scripts/e2e-backend.sh",
      url: "http://127.0.0.1:18121/health",
      timeout: 60_000,
      reuseExistingServer: false
    },
    {
      command: "GODRIVE_BACKEND_URL=http://127.0.0.1:18121 npm run dev -- --host 127.0.0.1 --port 15173",
      url: "http://127.0.0.1:15173",
      timeout: 60_000,
      reuseExistingServer: false
    }
  ],
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] }
    }
  ]
});
