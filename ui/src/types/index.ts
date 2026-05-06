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
  orchestration?: string
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
  total_events: number
}

export interface AdminKafkaSection {
  enabled: boolean
  mode: string
  total_lag: number
  total_events: number
  topics: KafkaTopicLag[]
}

export interface AdminMigrationSection {
  active: number
  migrations: OrchestrationMigration[]
  old_worker_total: number
  new_worker_total: number
  old_by_priority: Record<string, number>
  old_by_channel: Record<string, number>
  old_by_client: AdminWorkerClientSummary[]
  new_by_priority: Record<string, number>
  new_by_channel: Record<string, number>
  new_by_client: AdminWorkerClientSummary[]
  dry_run_result?: MigrationDryRunResult
}

export interface MigrationDryRunResult {
  old_provider: string
  new_provider: string
  old_host_port: string
  new_host_port: string
  old_namespace: string
  new_namespace: string
  old_workflow_count: number
  total_scheduled_count: number
  migrated_scheduled_count: number
  estimated_duration: string
}

export interface AutoScalerConfigForm {
  enabled?: boolean
  evaluation_interval?: string
  high_mttd_threshold?: string
  medium_mttd_threshold?: string
  low_mttd_threshold?: string
  max_lag_per_worker?: number
  cooldown_period?: string
  scale_down_factor?: number
  idle_threshold?: string
  unhealthy_threshold?: number
  global_max_workers?: number
  global_min_workers?: number
  mttd_lookback?: string
}

export interface OrchestrationMigration {
  id: string
  api_key_id?: string
  client_name: string
  old_provider: string
  new_provider: string
  old_config_json?: any
  new_config_json?: any
  status: 'in_progress' | 'transferring_scheduled' | 'waiting_old_workers' | 'completed' | 'failed'
  old_workflow_count: number
  completed_old_workflows: number
  migrated_scheduled_count: number
  total_scheduled_count: number
  error_message?: string
  started_at: string
  completed_at?: string
  notified_at?: string
  created_at: string
  updated_at: string
}

export interface AdminOverview {
  delivery: {
    window: AdminDeliveryStats
    window_label: string
    window_1h: AdminDeliveryStats
    window_24h: AdminDeliveryStats
  }
  top_clients: AdminClientRow[]
  mttd_by_priority: AdminMTTDRow[]
  mttd_by_vendor: AdminMTTDRow[]
  workers: AdminWorkersSection
  kafka: AdminKafkaSection
  cadence: AdminCadenceSection
  migrations: AdminMigrationSection
  dlq?: AdminDLQSection
}

export interface AdminDLQSection {
  total_entries: number
  pending_replay: number
  replayed: number
}

export interface DLQEntry {
  id: string
  notification_id?: string
  channel: string
  recipient: string
  reason: string
  payload?: Record<string, unknown>
  attempt_count: number
  replayed: boolean
  replayed_at?: string
  created_at: string
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

export type VendorMigrationStatus = 'in_progress' | 'completed' | 'failed' | 'rolled_back'
export type MigrationStrategy = 'instant' | 'gradual'

export interface VendorMigration {
  id: string
  api_key_id?: string | null
  channel: 'email' | 'sms' | 'push'
  from_vendor: string
  to_vendor: string
  from_config_json?: any
  to_config_json: any
  strategy: MigrationStrategy
  status: VendorMigrationStatus
  traffic_percent: number
  error_message?: string | null
  started_at: string
  completed_at?: string | null
  created_at: string
  updated_at: string
}

export interface StartVendorMigrationRequest {
  channel: 'email' | 'sms' | 'push'
  from_vendor: string
  to_vendor: string
  to_config: any
  strategy?: MigrationStrategy
}

export interface VendorRetryConfig {
  id: string
  vendor_name: string
  api_key_id?: string | null
  retry_initial_interval_ms: number
  retry_max_interval_ms: number
  retry_max_attempts: number
  retry_backoff_coefficient: number
  sla_seconds: number
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface UpsertVendorRetryConfigRequest {
  retry_initial_interval_ms?: number | null
  retry_max_interval_ms?: number | null
  retry_max_attempts?: number | null
  retry_backoff_coefficient?: number | null
  sla_seconds?: number | null
}
