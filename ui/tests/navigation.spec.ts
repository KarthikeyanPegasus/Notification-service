import { test, expect } from '@playwright/test'
import { mockApi } from './utils/mock-api'

test.beforeEach(async ({ page }) => {
  await mockApi(page)
})

test('can navigate to key pages from sidebar', async ({ page }) => {
  await page.goto('/dashboard')
  await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible()

  await page.getByRole('link', { name: 'Notifications' }).click()
  await expect(page.getByRole('heading', { name: 'Notifications' })).toBeVisible()

  await page.getByRole('link', { name: 'Templates' }).click()
  await expect(page.getByRole('heading', { name: 'Templates' })).toBeVisible()

  await page.getByRole('link', { name: 'Suppressions' }).click()
  await expect(page.getByRole('heading', { name: 'Suppressions' })).toBeVisible()

  await page.getByRole('link', { name: 'Opt-outs' }).click()
  await expect(page.getByRole('heading', { name: 'Opt-outs' })).toBeVisible()

  await page.getByRole('link', { name: 'Settings' }).click()
  await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible()
})

