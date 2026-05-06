import { test, expect } from '@playwright/test'
import { mockApi } from './utils/mock-api'

test('dashboard loads and shows title', async ({ page }) => {
  await mockApi(page)
  await page.goto('/');
  await expect(page).toHaveTitle(/NotifyHub/)
  await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible()
})
