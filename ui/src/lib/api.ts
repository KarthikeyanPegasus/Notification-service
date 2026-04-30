import type {
  Channel,
  Notification,
  NotificationFilters,
  PaginatedResponse,
  ReportSummary,
  ReportFilters,
  ScheduledFilters,
  ScheduledNotification,
  Template,
  VendorConfig,
  VendorRateLimit,
  IngressBreakdown,
  VendorMetric,
  VendorBilling,
  ScheduledStats,
  AdminOverview,
} from '@/types'

// In the browser, prefer same-origin `/v1/*` via Next.js rewrite proxy.
// This avoids browser -> localhost networking issues in dev and simplifies CORS.
// On the server (or in non-browser contexts), allow an explicit API base URL.
const API_BASE_URL =
  typeof window === 'undefined' ? process.env.NEXT_PUBLIC_API_URL ?? '' : ''

const AUTH_TOKEN_KEY = 'notifyhub-auth-token'

function adminAuthHeaders(): HeadersInit {
  const h: Record<string, string> = {}
  const bearer =
    (typeof window !== 'undefined' && window.localStorage.getItem(AUTH_TOKEN_KEY)) ||
    process.env.NEXT_PUBLIC_API_BEARER
  if (bearer) {
    h['Authorization'] = `Bearer ${bearer}`
  }
  return h
}

class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public details?: unknown,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

async function fetchJSON<T>(path: string, options?: RequestInit): Promise<T> {
  const url = API_BASE_URL ? `${API_BASE_URL}${path}` : path
  const baseHeaders: Record<string, string> = {
    'Content-Type': 'application/json',
    Accept: 'application/json',
  }
  const bearer =
    (typeof window !== 'undefined' && window.localStorage.getItem(AUTH_TOKEN_KEY)) ||
    process.env.NEXT_PUBLIC_API_BEARER
  if (bearer) baseHeaders['Authorization'] = `Bearer ${bearer}`
  let res: Response
  try {
    res = await fetch(url, { headers: { ...baseHeaders, ...(options?.headers as any) }, ...options })
  } catch (e) {
    // In some dev setups, `localhost` can resolve to an address the API isn't bound to.
    // Retry once with 127.0.0.1 as a best-effort fallback.
    if (API_BASE_URL.includes('localhost')) {
      const retryURL = `${API_BASE_URL.replace('localhost', '127.0.0.1')}${path}`
      try {
        res = await fetch(retryURL, { headers: { ...baseHeaders, ...(options?.headers as any) }, ...options })
      } catch {
        throw new Error(`Failed to fetch ${url}`)
      }
    } else if (!API_BASE_URL) {
      throw new Error(`Failed to fetch ${path}`)
    } else {
      throw new Error(`Failed to fetch ${url}`)
    }
  }
  if (!res.ok) {
    // If the user's token expired, clear it and bounce to login (best-effort).
    if (res.status === 401) {
      try {
        if (typeof window !== 'undefined') {
          clearAuthToken()
          // Avoid redirect loops if already on login.
          if (window.location?.pathname !== '/login') {
            window.location.href = '/login'
          }
        }
      } catch {
        // ignore
      }
    }

    let bodyText: string | null = null
    let bodyJSON: any = null
    try {
      bodyText = await res.text()
      if (bodyText) {
        try {
          bodyJSON = JSON.parse(bodyText)
        } catch {
          bodyJSON = null
        }
      }
    } catch {
      // ignore
    }

    const msgFromJSON =
      bodyJSON && typeof bodyJSON === 'object'
        ? String(bodyJSON.error ?? bodyJSON.message ?? '').trim()
        : ''
    const msgFromText = !msgFromJSON ? String(bodyText ?? '').trim() : ''
    const detail = msgFromJSON || msgFromText
    const base = `HTTP ${res.status}: ${res.statusText || 'Request failed'}`
    throw new ApiError(res.status, detail ? `${base} — ${detail}` : base, bodyJSON ?? bodyText)
  }

  // Some endpoints intentionally return empty bodies (e.g. DELETE -> 204 No Content).
  if (res.status === 204 || res.status === 205) {
    return undefined as T
  }
  const raw = await res.text()
  if (!raw) {
    return undefined as T
  }
  return JSON.parse(raw) as T
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
  if (filters.recipient) params['recipient'] = filters.recipient
  return fetchJSON<PaginatedResponse<Notification>>(`/v1/notifications${buildQuery(params)}`)
}

export async function getNotificationsScoped(
  filters: NotificationFilters = {},
  apiKeyId?: string,
): Promise<PaginatedResponse<Notification>> {
  const params: Record<string, unknown> = {
    ...filters,
    page_size: (filters as any).page_size ?? 20,
    page: (filters as any).page ?? 1,
    api_key_id: apiKeyId,
  }
  const channel = (filters as any).channel
  const status = (filters as any).status
  if (channel?.length) params['channel'] = channel
  if (status?.length) params['status'] = status
  if (filters.recipient) params['recipient'] = filters.recipient
  return fetchJSON<PaginatedResponse<Notification>>(`/v1/notifications${buildQuery(params)}`)
}

export async function createNotification(req: {
  channels: Channel[]
  recipient: string
  type?: string
  subject?: string
  body?: string
  scheduled_at?: string
  forced_vendor?: string
  idempotency_key: string
  client_id?: string
}): Promise<Notification> {
  return fetchJSON<Notification>('/v1/notifications', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

export async function getNotification(id: string): Promise<any> {
  return fetchJSON<any>(`/v1/notifications/${id}`)
}

export async function syncNotification(id: string): Promise<any> {
  return fetchJSON<any>(`/v1/notifications/${id}/sync`, { method: 'POST' })
}

export async function retriggerNotification(
  id: string,
): Promise<{ notification_id: string; status: string }> {
  return fetchJSON<{ notification_id: string; status: string }>(
    `/v1/notifications/${id}/retrigger`,
    { method: 'POST' },
  )
}

export async function getReports(filters: ReportFilters): Promise<ReportSummary[]> {
  return fetchJSON<ReportSummary[]>(`/v1/reports/summary${buildQuery(filters as unknown as Record<string, unknown>)}`)
}

export async function getReportsScoped(filters: ReportFilters, apiKeyId?: string): Promise<ReportSummary[]> {
  const params: Record<string, unknown> = { ...(filters as any) }
  if (apiKeyId) params['api_key_id'] = apiKeyId
  return fetchJSON<ReportSummary[]>(`/v1/reports/summary${buildQuery(params)}`)
}

export async function getScheduled(
  filters: ScheduledFilters = {},
): Promise<PaginatedResponse<ScheduledNotification>> {
  const params = {
    page_size: filters.page_size ?? 20,
    page: filters.page ?? 1,
  }
  return fetchJSON<PaginatedResponse<ScheduledNotification>>(`/v1/notifications/scheduled${buildQuery(params)}`)
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

export async function getVendorConfigs(apiKeyId?: string): Promise<VendorConfig[]> {
  const q = apiKeyId ? `?api_key_id=${encodeURIComponent(apiKeyId)}` : ''
  return fetchJSON<VendorConfig[]>(`/v1/admin/config/vendors${q}`, {
    headers: { ...adminAuthHeaders() },
  })
}

export const getVendorConfigsScoped = getVendorConfigs

export async function updateVendorConfig(vendorType: string, config: any): Promise<{ message: string }> {
  return fetchJSON<{ message: string }>(`/v1/admin/config/vendors/${vendorType}`, {
    method: 'PUT',
    body: JSON.stringify({ config }),
  })
}

export async function updateVendorConfigScoped(vendorType: string, config: any, apiKeyId?: string): Promise<{ message: string }> {
  const q = apiKeyId ? `?api_key_id=${encodeURIComponent(apiKeyId)}` : ''
  return fetchJSON<{ message: string }>(`/v1/admin/config/vendors/${vendorType}${q}`, {
    method: 'PUT',
    body: JSON.stringify({ config }),
  })
}

export async function deleteVendorConfigScoped(vendorType: string, apiKeyId?: string): Promise<{ message: string }> {
  const q = apiKeyId ? `?api_key_id=${encodeURIComponent(apiKeyId)}` : ''
  return fetchJSON<{ message: string }>(`/v1/admin/config/vendors/${vendorType}${q}`, {
    method: 'DELETE',
  })
}

export async function getVendorRateLimitsScoped(apiKeyId?: string): Promise<VendorRateLimit[]> {
  const q = apiKeyId ? `?api_key_id=${encodeURIComponent(apiKeyId)}` : ''
  return fetchJSON<VendorRateLimit[]>(`/v1/admin/config/rate-limits${q}`, {
    headers: { ...adminAuthHeaders() },
  })
}

export async function upsertVendorRateLimitScoped(
  vendorName: string,
  payload: {
    rps?: number | null
    per_minute?: number | null
    per_10_min?: number | null
    per_hour?: number | null
    per_day?: number | null
  },
  apiKeyId?: string,
): Promise<{ message: string }> {
  const q = apiKeyId ? `?api_key_id=${encodeURIComponent(apiKeyId)}` : ''
  return fetchJSON<{ message: string }>(`/v1/admin/config/rate-limits/${encodeURIComponent(vendorName)}${q}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export async function deleteVendorRateLimitScoped(vendorName: string, apiKeyId?: string): Promise<{ message: string }> {
  const q = apiKeyId ? `?api_key_id=${encodeURIComponent(apiKeyId)}` : ''
  return fetchJSON<{ message: string }>(`/v1/admin/config/rate-limits/${encodeURIComponent(vendorName)}${q}`, {
    method: 'DELETE',
  })
}

// Templates

export async function getTemplates(channel?: string): Promise<Template[]> {
  const query = channel ? `?channel=${channel}` : ''
  return fetchJSON<Template[]>(`/v1/templates${query}`)
}

export async function getTemplate(id: string): Promise<Template> {
  return fetchJSON<Template>(`/v1/templates/${id}`)
}

export async function createTemplate(template: Omit<Template, 'id' | 'version' | 'created_at' | 'updated_at'>): Promise<Template> {
  return fetchJSON<Template>('/v1/templates', {
    method: 'POST',
    body: JSON.stringify(template),
  })
}

export async function updateTemplate(id: string, template: Omit<Template, 'id' | 'version' | 'created_at' | 'updated_at'>): Promise<Template> {
  return fetchJSON<Template>(`/v1/templates/${id}`, {
    method: 'PUT',
    body: JSON.stringify(template),
  })
}

export async function deleteTemplate(id: string): Promise<void> {
  await fetchJSON<void>(`/v1/templates/${id}`, { method: 'DELETE' })
}

// API clients (admin API keys)

export interface ApiClientKey {
  id: string
  name: string
  prefix: string
  created_at: string
  revoked_at?: string | null
}

// List API key clients visible to the current user (admin: all; dev/support: assigned).
export async function listMyClients(): Promise<ApiClientKey[]> {
  return fetchJSON<ApiClientKey[]>('/v1/me/clients')
}

export async function sendVendorTest(
  vendorType: string,
  payload: { channel: 'sms' | 'email' | 'slack'; recipient: string; body: string; subject?: string; slack_channel?: string },
  apiKeyId?: string,
): Promise<any> {
  const q = apiKeyId ? `?api_key_id=${encodeURIComponent(apiKeyId)}` : ''
  return fetchJSON<any>(`/v1/admin/test-delivery/${encodeURIComponent(vendorType)}${q}`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export interface CreateApiKeyResponse {
  key: ApiClientKey
  api_key: string
}

export async function listApiKeys(): Promise<ApiClientKey[]> {
  return fetchJSON<ApiClientKey[]>('/v1/admin/api-keys', {
    headers: { ...adminAuthHeaders() },
  })
}

export interface LoginResponse {
  token: string
  user: { id: string; email: string; name: string; role: string }
}

export async function login(email: string, password: string): Promise<LoginResponse> {
  return fetchJSON<LoginResponse>('/v1/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  })
}

export function setAuthToken(token: string) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(AUTH_TOKEN_KEY, token)
}

const AUTH_USER_KEY = 'notifyhub-auth-user'

export function setAuthUser(user: LoginResponse['user']) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(AUTH_USER_KEY, JSON.stringify(user))
}

export function getAuthUser(): LoginResponse['user'] | null {
  if (typeof window === 'undefined') return null
  const raw = window.localStorage.getItem(AUTH_USER_KEY)
  if (!raw) return null
  try {
    return JSON.parse(raw)
  } catch {
    return null
  }
}

export function clearAuthToken() {
  if (typeof window === 'undefined') return
  window.localStorage.removeItem(AUTH_TOKEN_KEY)
  window.localStorage.removeItem(AUTH_USER_KEY)
}

export function getAuthToken(): string | null {
  if (typeof window === 'undefined') return null
  return window.localStorage.getItem(AUTH_TOKEN_KEY)
}

export async function createApiKey(name: string): Promise<CreateApiKeyResponse> {
  return fetchJSON<CreateApiKeyResponse>('/v1/admin/api-keys', {
    method: 'POST',
    headers: { ...adminAuthHeaders() },
    body: JSON.stringify({ name }),
  })
}

export async function revokeApiKey(id: string): Promise<void> {
  await fetchJSON<void>(`/v1/admin/api-keys/${id}`, {
    method: 'DELETE',
    headers: { ...adminAuthHeaders() },
  })
}

export async function getIngressBreakdown(filters?: ReportFilters): Promise<IngressBreakdown[]> {
  const queryParams = new URLSearchParams()
  if (filters?.date_from) queryParams.set('date_from', filters.date_from)
  if (filters?.date_to) queryParams.set('date_to', filters.date_to)

  return fetchJSON<IngressBreakdown[]>(`/v1/reports/ingress?${queryParams.toString()}`)
}

export async function getIngressBreakdownScoped(filters?: ReportFilters, apiKeyId?: string): Promise<IngressBreakdown[]> {
  const queryParams = new URLSearchParams()
  if (filters?.date_from) queryParams.set('date_from', filters.date_from)
  if (filters?.date_to) queryParams.set('date_to', filters.date_to)
  if (apiKeyId) queryParams.set('api_key_id', apiKeyId)
  return fetchJSON<IngressBreakdown[]>(`/v1/reports/ingress?${queryParams.toString()}`)
}

export interface BreakdownRow {
  key: string
  count: number
}

export async function getSMSCountryBreakdown(filters?: ReportFilters, apiKeyId?: string): Promise<BreakdownRow[]> {
  const params: any = { ...filters }
  if (apiKeyId) params['api_key_id'] = apiKeyId
  return fetchJSON<BreakdownRow[]>(`/v1/reports/sms-countries${buildQuery(params)}`)
}

export async function getEmailDomainBreakdown(filters?: ReportFilters, apiKeyId?: string): Promise<BreakdownRow[]> {
  const params: any = { ...filters }
  if (apiKeyId) params['api_key_id'] = apiKeyId
  return fetchJSON<BreakdownRow[]>(`/v1/reports/email-domains${buildQuery(params)}`)
}

export async function addSuppression(type: 'email' | 'sms' | 'push' | 'slack' | 'webhook', value: string, reason: string): Promise<any> {
  return fetchJSON<any>('/v1/governance/suppressions', {
    method: 'POST',
    body: JSON.stringify({ type, value, reason })
  })
}

export async function getVendorHealth(apiKeyId?: string): Promise<VendorMetric[]> {
  const q = apiKeyId ? `?api_key_id=${encodeURIComponent(apiKeyId)}` : ''
  return fetchJSON<VendorMetric[]>(`/v1/reports/vendors${q}`)
}

export async function getVendorBilling(apiKeyId?: string): Promise<VendorBilling[]> {
  const q = apiKeyId ? `?api_key_id=${encodeURIComponent(apiKeyId)}` : ''
  const res = await fetchJSON<any>(`/v1/reports/billing${q}`)
  if (Array.isArray(res)) return res as VendorBilling[]
  if (res && typeof res === 'object' && Array.isArray((res as any).data)) return (res as any).data as VendorBilling[]
  return []
}

export async function getScheduledStats(filters: ReportFilters, apiKeyId?: string): Promise<ScheduledStats> {
  const params: Record<string, unknown> = { ...(filters as any) }
  if (apiKeyId) params['api_key_id'] = apiKeyId
  return fetchJSON<ScheduledStats>(`/v1/reports/scheduled-stats${buildQuery(params)}`)
}


export async function getAdminOverview(): Promise<AdminOverview> {
  return fetchJSON<AdminOverview>('/v1/admin/overview')
}
