export type Channel = 'email' | 'sms' | 'push' | 'websocket' | 'webhook' | 'slack'
export type Status = 'PENDING' | 'QUEUED' | 'SENT' | 'DELIVERED' | 'FAILED' | 'CANCELLED'
export type Priority = 'low' | 'normal' | 'high' | 'critical'

export interface Notification {
  id: string
  user_id: string
  channel: Channel
  status: Status
  priority: Priority
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
  error?: string
  provider_message_id?: string
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
  success_rate: number
  p50_latency_ms: number
  p95_latency_ms: number
  date: string
}

export interface IngressBreakdown {
  source: string
  count: number
}

export interface PaginatedResponse<T> {
  data: T[]
  total: number
  page: number
  page_size: number
}

export interface NotificationFilters {
  search?: string
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
}

export interface ScheduledNotification {
  id: string
  notification_id: string
  user_id: string
  channel: Channel
  template_id?: string
  template_vars?: Record<string, string>
  scheduled_at: string
  original_at: string
  status: Status
  reschedule_count: number
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
