import type { Page } from '@playwright/test'

function json(obj: unknown) {
  return {
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(obj),
  }
}

/**
 * Intercept `/v1/*` API calls and return stable fixtures so the UI
 * can be E2E-tested without a running backend.
 */
export async function mockApi(page: Page) {
  await page.route('**/v1/**', async (route) => {
    const req = route.request()
    const url = new URL(req.url())
    const path = url.pathname

    // Most list endpoints expect arrays.
    if (
      path.startsWith('/v1/reports/summary') ||
      path.startsWith('/v1/reports/ingress') ||
      path.startsWith('/v1/reports/vendors') ||
      path.startsWith('/v1/reports/billing') ||
      path.startsWith('/v1/templates') ||
      path.startsWith('/v1/admin/config/vendors') ||
      path.startsWith('/v1/me/clients') ||
      path.startsWith('/v1/governance/suppressions') ||
      path.startsWith('/v1/governance/opt-outs') ||
      path.startsWith('/v1/admin/api-keys') ||
      path.startsWith('/v1/admin/migrations')
    ) {
      return route.fulfill(json([]))
    }

    // Paginated notifications endpoints.
    if (path === '/v1/notifications' || path.startsWith('/v1/notifications/')) {
      // Detail view endpoints can accept a generic object.
      if (path !== '/v1/notifications') {
        return route.fulfill(json({}))
      }
      return route.fulfill(json({ data: [], total: 0, page: 1, page_size: 20 }))
    }

    // Scheduled notifications (paginated).
    if (path.startsWith('/v1/notifications/scheduled')) {
      return route.fulfill(json({ data: [], total: 0, page: 1, page_size: 20 }))
    }

    // DLQ endpoints.
    if (path.startsWith('/v1/admin/dlq/stats')) {
      return route.fulfill(json({ total_entries: 0, pending_replay: 0, replayed_entries: 0 }))
    }
    if (path.startsWith('/v1/admin/dlq')) {
      return route.fulfill(json({ data: [], total: 0, page: 1, page_size: 20 }))
    }

    // Admin overview.
    if (path.startsWith('/v1/admin/overview')) {
      return route.fulfill(json({}))
    }

    // Autoscaler config.
    if (path.startsWith('/v1/admin/config/autoscaler')) {
      if (req.method() === 'GET') return route.fulfill(json({}))
      return route.fulfill(json({ message: 'ok' }))
    }

    // Fallback: succeed with an empty JSON body.
    return route.fulfill(json({}))
  })
}

