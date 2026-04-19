import { test, expect } from '@playwright/test';

test('dashboard loads and shows title', async ({ page }) => {
  await page.goto('/');
  await expect(page).toHaveTitle(/NotifyHub/);
  await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
});
