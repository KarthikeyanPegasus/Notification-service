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
  OrchestrationMigration,
  VendorMigration,
  StartVendorMigrationRequest,
  AutoScalerConfigForm,
  DLQEntry,
} from '@/types'

// Prefer explicit API base URL when provided (browser + server).
// Fallback to same-origin paths ("/v1/*") so Next.js rewrites still work.
const API_BASE_URL = (process.env.NEXT_PUBLIC_API_URL ?? '').trim()

const AUTH_TOKEN_KEY = 'notifyhub-auth-token'

async function tryGetClerkToken(): Promise<string | null> {
  // Browser path: Clerk attaches itself to window.
  if (typeof window !== 'undefined') {
    try {
      const clerk: any = (window as any).Clerk
      if (clerk?.session?.getToken) {
        const t = await clerk.session.getToken()
        return t ? String(t) : null
      }
    } catch {
      // ignore
    }
    return null
  }
  return null
}

function adminAuthHeaders(clerkToken?: string): HeadersInit {
  const h: Record<string, string> = {}
  if (clerkToken) {
    h['Authorization'] = `Bearer ${clerkToken}`
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

export async function fetchJSON<T>(path: string, options?: RequestInit, clerkToken?: string): Promise<T> {
  const baseURL = API_BASE_URL.replace(/\/$/, '')
  const primaryURL = baseURL ? `${baseURL}${path}` : path
  const baseHeaders: Record<string, string> = {
    'Content-Type': 'application/json',
    Accept: 'application/json',
  }
  // Prefer explicit token, otherwise try Clerk. No other fallback — bearer tokens
  // must not be stored in localStorage or NEXT_PUBLIC_ env vars.
  let bearer: string | null = clerkToken ?? null
  if (!bearer) bearer = await tryGetClerkToken()
  if (bearer) baseHeaders['Authorization'] = `Bearer ${bearer}`

  // Try explicit API base first; on network failure, fallback to same-origin `/v1/*`
  // so Next.js rewrites can still proxy in containerized/dev setups.
  const candidateURLs: string[] = [primaryURL]
  if (baseURL && typeof window !== 'undefined') {
    if (baseURL.includes('localhost')) {
      candidateURLs.push(`${baseURL.replace('localhost', '127.0.0.1')}${path}`)
    }
    if (!candidateURLs.includes(path)) {
      candidateURLs.push(path)
    }
  }

  let res: Response | null = null
  for (const candidate of candidateURLs) {
    try {
      res = await fetch(candidate, { headers: { ...baseHeaders, ...(options?.headers as any) }, ...options })
      break
    } catch {
      // try next candidate URL
    }
  }

  if (!res) {
    if (!baseURL) {
      throw new Error(`Failed to fetch ${path}`)
    }
    throw new Error(`Failed to fetch ${primaryURL}`)
  }
  if (!res.ok) {
    // If the user's legacy token expired, clear it (best-effort).
    // With Clerk, navigation to sign-in should be handled by Clerk/AuthGuard.
    if (res.status === 401) {
      try {
        if (typeof window !== 'undefined') {
          clearAuthToken()
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
    let msgFromText = !msgFromJSON ? String(bodyText ?? '').trim() : ''
    // Avoid dumping full HTML documents into toast/error UI.
    if (msgFromText.startsWith('<!DOCTYPE html') || msgFromText.startsWith('<html')) {
      msgFromText = 'Unexpected HTML response (likely wrong API URL/rewrite).'
    } else if (msgFromText.length > 500) {
      msgFromText = `${msgFromText.slice(0, 500)}...`
    }
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

// ── Vendor Migrations ────────────────────────────────────────────────────────

export async function startVendorMigration(
  payload: StartVendorMigrationRequest,
  apiKeyId?: string,
): Promise<VendorMigration> {
  const q = apiKeyId ? `?api_key_id=${encodeURIComponent(apiKeyId)}` : ''
  return fetchJSON<VendorMigration>(`/v1/admin/config/vendors/migrations${q}`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function listVendorMigrations(
  opts?: { apiKeyId?: string; channel?: string; status?: string },
): Promise<VendorMigration[]> {
  const params = new URLSearchParams()
  if (opts?.apiKeyId) params.set('api_key_id', opts.apiKeyId)
  if (opts?.channel)  params.set('channel', opts.channel)
  if (opts?.status)   params.set('status', opts.status)
  const q = params.toString() ? `?${params.toString()}` : ''
  return fetchJSON<VendorMigration[]>(`/v1/admin/config/vendors/migrations${q}`)
}

export async function completeVendorMigration(id: string): Promise<{ message: string }> {
  return fetchJSON<{ message: string }>(`/v1/admin/config/vendors/migrations/${id}/complete`, {
    method: 'POST',
  })
}

export async function rollbackVendorMigration(id: string): Promise<{ message: string }> {
  return fetchJSON<{ message: string }>(`/v1/admin/config/vendors/migrations/${id}/rollback`, {
    method: 'POST',
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

// Create a sandbox client for the current dev user (auto-assigned, max 1).
export async function createMyClient(name: string): Promise<CreateApiKeyResponse> {
  return fetchJSON<CreateApiKeyResponse>('/v1/me/clients', {
    method: 'POST',
    body: JSON.stringify({ name }),
  })
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

// ── Clerk token helpers ───────────────────────────────────────────────
// These are used as fallbacks; prefer Clerk's useAuth() hook for new code.

export function getAuthToken(): string | null {
  if (typeof window === 'undefined') return null
  return window.localStorage.getItem(AUTH_TOKEN_KEY)
}

export function clearAuthToken() {
  if (typeof window === 'undefined') return
  window.localStorage.removeItem(AUTH_TOKEN_KEY)
  window.localStorage.removeItem(AUTH_USER_KEY)
}

export function getAuthUser(): { id: string; email: string; name: string; role: string } | null {
  if (typeof window === 'undefined') return null
  const raw = window.localStorage.getItem(AUTH_USER_KEY)
  if (!raw) return null
  try {
    return JSON.parse(raw)
  } catch {
    return null
  }
}

export function setAuthToken(token: string) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(AUTH_TOKEN_KEY, token)
}

export function setAuthUser(user: { id: string; email: string; name: string; role: string }) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(AUTH_USER_KEY, JSON.stringify(user))
}

// Legacy login function intentionally disabled.
// Authentication is managed by Clerk; backend /v1/auth/login is removed.
export async function login(_email: string, _password: string): Promise<{ token: string; user: { id: string; email: string; name: string; role: string } }> {
  throw new Error('Legacy login is no longer supported. Use Clerk authentication flow instead.')
}

const AUTH_USER_KEY = 'notifyhub-auth-user'

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

export async function getSuppressions(): Promise<any[]> {
  return fetchJSON<any[]>('/v1/governance/suppressions')
}

export async function deleteSuppression(id: string): Promise<void> {
  await fetchJSON<void>(`/v1/governance/suppressions/${id}`, { method: 'DELETE' })
}

export async function createSuppression(payload: {
  type: 'email' | 'sms' | 'push' | 'slack' | 'webhook'
  value: string
  reason?: string
}): Promise<any> {
  return fetchJSON<any>('/v1/governance/suppressions', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function getOptOuts(): Promise<any[]> {
  return fetchJSON<any[]>('/v1/governance/opt-outs')
}

export async function deleteOptOut(id: string): Promise<void> {
  await fetchJSON<void>(`/v1/governance/opt-outs/${id}`, { method: 'DELETE' })
}

export async function createOptOut(payload: {
  user_id: string
  channel: string
  reason?: string
  source: string
}): Promise<any> {
  return fetchJSON<any>('/v1/governance/opt-outs', {
    method: 'POST',
    body: JSON.stringify(payload),
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


// ── Dead-Letter Queue ────────────────────────────────────────────────────────

export async function getDLQEntries(page?: number, unreplayedOnly?: boolean): Promise<{ data: DLQEntry[]; total: number; page: number; page_size: number }> {
  const params = new URLSearchParams()
  if (page) params.set('page', String(page))
  if (unreplayedOnly) params.set('unreplayed', 'true')
  const q = params.toString() ? `?${params.toString()}` : ''
  return fetchJSON(`/v1/admin/dlq${q}`)
}

export async function getDLQStats(): Promise<{ total_entries: number; pending_replay: number; replayed_entries: number }> {
  return fetchJSON('/v1/admin/dlq/stats')
}

export async function getDLQEntry(id: string): Promise<DLQEntry> {
  return fetchJSON(`/v1/admin/dlq/${id}`)
}

export async function replayDLQEntry(id: string): Promise<{ dlq_id: string; notification_id?: string; status: string }> {
  return fetchJSON(`/v1/admin/dlq/${id}/replay`, { method: 'POST' })
}

export async function replayAllDLQ(): Promise<{ total: number; replayed: number; errors: number; status: string }> {
  return fetchJSON('/v1/admin/dlq/replay-all', { method: 'POST' })
}

export async function getAdminOverview(window?: string): Promise<AdminOverview> {
  const q = window ? `?window=${encodeURIComponent(window)}` : ''
  return fetchJSON<AdminOverview>(`/v1/admin/overview${q}`)
}

export async function listMigrations(activeOnly?: boolean): Promise<OrchestrationMigration[]> {
  const q = activeOnly ? '?active_only=true' : ''
  return fetchJSON<OrchestrationMigration[]>(`/v1/admin/migrations${q}`)
}

export async function cancelMigration(id: string): Promise<{ message: string }> {
  return fetchJSON<{ message: string }>(`/v1/admin/migrations/${id}/cancel`, { method: 'POST' })
}

export async function getAutoScalerConfig(): Promise<AutoScalerConfigForm> {
  return fetchJSON<AutoScalerConfigForm>('/v1/admin/config/autoscaler')
}

export async function updateAutoScalerConfig(cfg: Partial<AutoScalerConfigForm>): Promise<{ message: string; config: AutoScalerConfigForm }> {
  return fetchJSON<{ message: string; config: AutoScalerConfigForm }>('/v1/admin/config/autoscaler', {
    method: 'PUT',
    body: JSON.stringify(cfg),
  })
}

// API Docs visibility
export async function getApiDocsVisibility(): Promise<{ hidden_endpoints: string[] }> {
  try {
    return await fetchJSON<{ hidden_endpoints: string[] }>('/v1/docs/visibility')
  } catch {
    return { hidden_endpoints: [] }
  }
}

export async function setApiDocsVisibility(hiddenEndpoints: string[]): Promise<void> {
  await fetchJSON('/v1/admin/config/vendors/api_docs_visibility', {
    method: 'PUT',
    body: JSON.stringify({ config: { hidden_endpoints: hiddenEndpoints } }),
  })
}
