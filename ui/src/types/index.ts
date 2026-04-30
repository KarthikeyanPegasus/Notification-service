export type Channel = 'email' | 'sms' | 'push' | 'websocket' | 'webhook' | 'slack'
// API returns lowercase notification statuses (mirrors backend domain.NotificationStatus).
export type Status =
  | 'pending'
  | 'queued'
  | 'sent'
  | 'delivered'
  | 'failed'
  | 'cancelled'
  | 'bounced'
  | 'suppressed'
export type Priority = 'low' | 'normal' | 'high' | 'critical'

export interface Notification {
  id: string
  user_id: string
  api_key_id?: string
  client_name?: string
  channel: Channel
  status: Status
  priority: Priority
  recipient?: string
  subject?: string
  body: string
  provider?: string
  idempotency_key?: string
  scheduled_at?: string
  sent_at?: string
  source?: string
  created_at: string
  updated_at: string
  attempts?: Attempt[]
  events?: Event[]
}

export interface Attempt {
  id: string
  notification_id: string
  provider: string
  status: string
  // Normalized UI fields (older API shape)
  error?: string
  provider_message_id?: string

  // Backend attempt fields (current API shape)
  error_message?: string | null
  error_code?: string | null
  provider_msg_id?: string | null
  latency_ms: number
  created_at: string
}

export interface Event {
  id: string
  notification_id: string
  event_type: string
  metadata?: Record<string, unknown>
  created_at: string
}

export interface ReportSummary {
  channel: string
  total: number
  sent: number
  delivered: number
  failed: number
  bounced: number
  success_rate: number
  p50_latency_ms: number
  p95_latency_ms: number
  date: string
}

export interface IngressBreakdown {
  source: string
  count: number
}

export interface VendorMetric {
  provider: string
  total: number
  success_rate: number
  avg_latency_ms: number
  latest_error: string
  last_status?: string
  cost_per_msg: number
  estimated_cost: number
  bounce_count: number
  bounce_rate: number
  complaint_count: number
  complaint_rate: number
  all_time_total: number
  // UI derived fields
  status: 'healthy' | 'degraded' | 'down'
  payment_status: 'paid' | 'unpaid' | 'low_balance'
}

export interface ScheduledStats {
  total_scheduled: number
  pending: number
  delivered: number
  avg_delivery_latency_ms: number | null
}

export interface VendorBilling {
  provider: string
  total_cost: number
  balance?: number
  currency: string
  period: string
  is_actual: boolean
  source: string
  total_count: number
  cost_per_msg: number
  error?: string
}

export interface PaginatedResponse<T> {
  data: T[]
  total: number
  page: number
  page_size: number
}

// ── Admin Dashboard ───────────────────────────────────────────────────────────

export interface AdminDeliveryStats {
  total: number
  sent: number
  failed: number
  failure_rate: number
}

export interface AdminClientRow {
  client_id: string
  client_name: string
  total: number
  sent: number
  failed: number
  failure_rate: number
}

export interface AdminMTTDRow {
  group: string
  avg_ms: number
  p95_ms: number
  count: number
}

export interface AdminWorkerClientSummary {
  client_id: string
  client_name: string
  total: number
  by_priority: Record<string, number>
  by_channel: Record<string, number>
}

export interface AdminWorkersSection {
  total: number
  by_priority: Record<string, number>
  by_channel: Record<string, number>
  by_client: AdminWorkerClientSummary[]
}

export interface CadenceThroughputRow {
  channel: string
  priority: string
  count: number
}

export interface CadenceRetryRow {
  channel: string
  total_notifications: number
  total_attempts: number
  retried_count: number
  retry_rate: number
  avg_attempts: number
}

export interface CadenceScheduleRow {
  channel: string
  priority: string
  avg_ms: number
  p95_ms: number
  count: number
}

export interface AdminCadenceSection {
  engine: string
  throughput: CadenceThroughputRow[]
  retries: CadenceRetryRow[]
  schedule_to_start: CadenceScheduleRow[]
}

export interface KafkaTopicLag {
  topic: string
  channel: string
  priority: string
  lag: number
}

export interface AdminKafkaSection {
  total_lag: number
  topics: KafkaTopicLag[]
}

export interface AdminOverview {
  delivery: {
    window_1h: AdminDeliveryStats
    window_24h: AdminDeliveryStats
  }
  top_clients: AdminClientRow[]
  mttd_by_priority: AdminMTTDRow[]
  mttd_by_vendor: AdminMTTDRow[]
  workers: AdminWorkersSection
  kafka: AdminKafkaSection
  cadence: AdminCadenceSection
}

export interface NotificationFilters {
  search?: string
  recipient?: string
  channel?: Channel[]
  status?: Status[]
  date_from?: string
  date_to?: string
  page?: number
  page_size?: number
}

export interface ReportFilters {
  date_from: string
  date_to: string
  channel?: Channel
}

export interface ScheduledFilters {
  page?: number
  page_size?: number
}

export interface KPISummary {
  total_sent_24h: number
  success_rate: number
  failed_24h: number
  pending: number
}

export interface ChannelHealth {
  channel: Channel
  status: 'healthy' | 'degraded' | 'down'
  success_rate: number
  total_24h: number
  uptime: number
  payment_status: 'paid' | 'unpaid' | 'low_balance'
  error_status?: string | null
  bounce_rate?: number
}

export interface ScheduledNotification {
  id: string
  notification_id: string
  channel: Channel
  template_id?: string
  template_vars?: Record<string, string>
  scheduled_at: string
  original_at: string
  status: Status
  reschedule_count: number
  api_key_id?: string
  created_at: string
  updated_at: string
}

export interface Template {
  id: string
  name: string
  channel: Channel
  subject?: string | null
  body: string
  version: number
  is_active?: boolean
  created_at: string
  updated_at?: string
}

export interface VendorConfig {
  id: string
  vendor_type: string
  config_json: any
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface VendorRateLimit {
  id: string
  vendor_name: string
  api_key_id?: string | null
  rps?: number | null
  per_minute?: number | null
  per_10_min?: number | null
  per_hour?: number | null
  per_day?: number | null
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface UpsertVendorRateLimitRequest {
  rps?: number | null
  per_minute?: number | null
  per_10_min?: number | null
  per_hour?: number | null
  per_day?: number | null
}
