import { defineConfig, devices } from '@playwright/test';

/**
 * Where the suite points its browser. Local runs default to the dev server on
 * :3000; CI sets this to the server it started itself. `PLAYWRIGHT_MANAGE_WEB_SERVER=0`
 * tells Playwright not to start one (CI runs `next start` and waits for it, so a
 * failed start is visible in the job log rather than swallowed by Playwright).
 */
const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:3000';
const manageWebServer = process.env.PLAYWRIGHT_MANAGE_WEB_SERVER !== '0';

export default defineConfig({
  testDir: './tests/e2e',
  // The suite writes real records to one disposable database. Running files in
  // parallel would interleave fixtures and make failures unreproducible.
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  // Bounded allowances for a loaded CI runner. These only extend waits; every
  // assertion still has to observe real state. `next dev` compiles routes on
  // demand and can exceed these, so verify locally with the same `next build`
  // + `next start` pair CI uses (see tests/e2e/README.md).
  timeout: 60_000,
  expect: { timeout: 10_000 },
  reporter: [['list'], ['html', { open: 'never', outputFolder: 'playwright-report' }]],
  use: {
    baseURL,
    // Everything is uploaded by the CI workflow with `if: always()`, so a green
    // run still leaves a trace to inspect when a later change breaks a flow.
    trace: 'on',
    screenshot: 'on',
    video: 'on',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: manageWebServer
    ? {
        command: 'npm run dev',
        url: baseURL,
        reuseExistingServer: !process.env.CI,
        timeout: 120000,
      }
    : undefined,
});
