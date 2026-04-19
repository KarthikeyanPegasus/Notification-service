import type {
  Notification,
  NotificationFilters,
  PaginatedResponse,
  ReportSummary,
  ReportFilters,
  ScheduledFilters,
} from '@/types'

const BASE_URL = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080'

class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

async function fetchJSON<T>(path: string, options?: RequestInit): Promise<T> {
  const url = `${BASE_URL}${path}`
  const res = await fetch(url, {
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    ...options,
  })
  if (!res.ok) {
    throw new ApiError(res.status, `HTTP ${res.status}: ${res.statusText}`)
  }
  return res.json() as Promise<T>
}

function buildQuery(params: Record<string, unknown>): string {
  const q = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === '') continue
    if (Array.isArray(v)) {
      v.forEach((item) => q.append(k, String(item)))
    } else {
      q.set(k, String(v))
    }
  }
  const str = q.toString()
  return str ? `?${str}` : ''
}

export async function getNotifications(
  filters: NotificationFilters = {},
): Promise<PaginatedResponse<Notification>> {
  const { channel, status, ...rest } = filters
  const params: Record<string, unknown> = {
    ...rest,
    page_size: rest.page_size ?? 20,
    page: rest.page ?? 1,
  }
  if (channel?.length) params['channel'] = channel
  if (status?.length) params['status'] = status
  return fetchJSON<PaginatedResponse<Notification>>(`/v1/notifications${buildQuery(params)}`)
}

export async function getNotification(id: string): Promise<any> {
  return fetchJSON<any>(`/v1/notifications/${id}`)
}

export async function syncNotification(id: string): Promise<any> {
  return fetchJSON<any>(`/v1/notifications/${id}/sync`, { method: 'POST' })
}

export async function getReports(filters: ReportFilters): Promise<ReportSummary[]> {
  return fetchJSON<ReportSummary[]>(`/v1/reports/summary${buildQuery(filters as unknown as Record<string, unknown>)}`)
}

export async function getScheduled(
  filters: ScheduledFilters = {},
): Promise<PaginatedResponse<Notification>> {
  const params = {
    page_size: filters.page_size ?? 20,
    page: filters.page ?? 1,
  }
  return fetchJSON<PaginatedResponse<Notification>>(`/v1/notifications/scheduled${buildQuery(params)}`)
}

export async function cancelScheduled(id: string): Promise<void> {
  await fetchJSON<void>(`/v1/notifications/${id}/schedule`, { method: 'DELETE' })
}

export async function rescheduleNotification(id: string, scheduledAt: string): Promise<Notification> {
  return fetchJSON<Notification>(`/v1/notifications/${id}/schedule`, {
    method: 'PATCH',
    body: JSON.stringify({ scheduled_at: scheduledAt }),
  })
}

export interface VendorConfig {
  id: string
  vendor_type: string
  config_json: any
  is_active: boolean
  updated_at: string
}

export async function getVendorConfigs(): Promise<VendorConfig[]> {
  return fetchJSON<VendorConfig[]>('/v1/admin/config/vendors')
}

export async function updateVendorConfig(vendorType: string, config: any): Promise<{ message: string }> {
  return fetchJSON<{ message: string }>(`/v1/admin/config/vendors/${vendorType}`, {
    method: 'PUT',
    body: JSON.stringify({ config }),
  })
}
