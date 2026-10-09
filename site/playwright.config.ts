import { defineConfig } from "@playwright/test";

// The end-to-end tests run against the production build, served from /legatus/ exactly as on GitHub Pages.
// Set LEGATUS_CHROMIUM to use a Chromium that is already installed instead of Playwright's own download.
const port = 4173;

export default defineConfig({
  testDir: "e2e",
  timeout: 45_000,
  expect: { timeout: 8_000 },
  fullyParallel: true,
  reporter: [["list"]],
  use: {
    baseURL: `http://127.0.0.1:${port}/legatus/`,
    launchOptions: {
      // Software WebGL, so the optional wallpaper shader can be exercised on machines without a GPU.
      args: ["--enable-unsafe-swiftshader"],
      ...(process.env.LEGATUS_CHROMIUM ? { executablePath: process.env.LEGATUS_CHROMIUM } : {}),
    },
    trace: "retain-on-failure",
  },
  webServer: {
    command: `npx vite preview --host 127.0.0.1 --port ${port} --strictPort`,
    url: `http://127.0.0.1:${port}/legatus/`,
    reuseExistingServer: !process.env.CI,
    timeout: 60_000,
  },
});
