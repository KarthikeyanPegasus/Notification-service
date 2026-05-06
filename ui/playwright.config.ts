import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: 'html',
  use: {
    baseURL: 'http://localhost:3000',
    trace: 'on-first-retry',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: {
    // Run with Clerk disabled so the UI uses the built-in mock auth provider.
    // Default to build+start to avoid dev server file-watcher limits in CI/containers.
    // Set PLAYWRIGHT_USE_DEV_SERVER=true to use `next dev` locally.
    command: process.env.PLAYWRIGHT_USE_DEV_SERVER === 'true'
      ? 'NEXT_PUBLIC_CLERK_ENABLED=false npm run dev -- -p 3000 -H 127.0.0.1'
      : 'NEXT_PUBLIC_CLERK_ENABLED=false npm run build && NEXT_PUBLIC_CLERK_ENABLED=false PORT=3000 HOSTNAME=127.0.0.1 node .next/standalone/server.js',
    url: 'http://localhost:3000',
    reuseExistingServer: !process.env.CI,
    cwd: '.',
  },
});
