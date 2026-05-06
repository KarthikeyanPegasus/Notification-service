'use client'

import { useMemo, useState, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { PageHeader } from '@/components/shared/page-header'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { KPISkeleton, CardSkeleton } from '@/components/shared/loading-skeleton'
import { ErrorState } from '@/components/shared/error-state'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Cell,
} from 'recharts'
import {
  AlertTriangle,
  CheckCircle2,
  XCircle,
  Layers,
  Gauge,
  Users,
  Activity,
  Clock,
  TrendingUp,
  Server,
  Radio,
  BarChart2,
  RefreshCw,
  Repeat,
  Workflow,
  ArrowRight,
  Loader2,
  Info,
  Save,
} from 'lucide-react'
import { getAdminOverview } from '@/lib/api'
import { cn } from '@/lib/utils'
import type { AdminOverview, AdminWorkerClientSummary, KafkaTopicLag, OrchestrationMigration } from '@/types'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'

// ── Helpers ───────────────────────────────────────────────────────────────────

function calcMigrationProgress(mig: OrchestrationMigration): number {
  if (mig.status === 'completed') return 1
  if (mig.old_workflow_count === 0 && mig.total_scheduled_count === 0) return 0.5
  const wfProgress = mig.old_workflow_count > 0 ? mig.completed_old_workflows / mig.old_workflow_count : 1
  const schedProgress = mig.total_scheduled_count > 0 ? mig.migrated_scheduled_count / mig.total_scheduled_count : 1
  return 0.6 * wfProgress + 0.4 * schedProgress
}

function fmt(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`
  return String(n)
}

function fmtMs(ms: number): string {
  if (ms >= 1000) return `${(ms / 1000).toFixed(2)}s`
  return `${Math.round(ms)}ms`
}

function fmtPct(r: number): string {
  return `${(r * 100).toFixed(1)}%`
}

const PRIORITY_ORDER = ['high', 'medium', 'low']
const PRIORITY_COLOR: Record<string, string> = {
  high: '#ef4444',
  medium: '#f59e0b',
  low: '#22c55e',
}
const CHANNEL_COLOR: Record<string, string> = {
  email: '#6366f1',
  sms: '#0ea5e9',
  push: '#f97316',
  webhook: '#8b5cf6',
  slack: '#4ade80',
  websocket: '#fb7185',
}

const WINDOW_OPTIONS: { value: string; label: string }[] = [
  { value: '1h', label: 'Last 1h' },
  { value: '3h', label: 'Last 3h' },
  { value: '6h', label: 'Last 6h' },
  { value: '12h', label: 'Last 12h' },
  { value: '1d', label: 'Last 1d' },
  { value: '1w', label: 'Last 1 week' },
  { value: '1mo', label: 'Last 1 month' },
  { value: '3mo', label: 'Last 3 months' },
]

// ── Sub-components ────────────────────────────────────────────────────────────

function StatCard({
  label,
  value,
  sub,
  icon: Icon,
  alert,
}: {
  label: string
  value: string
  sub?: string
  icon: React.ElementType
  alert?: 'ok' | 'warn' | 'crit'
}) {
  const ring =
    alert === 'crit'
      ? 'border-red-500/50 bg-red-50 dark:bg-red-950/20'
      : alert === 'warn'
        ? 'border-amber-400/50 bg-amber-50 dark:bg-amber-950/20'
        : ''

  return (
    <Card className={cn('transition-colors', ring)}>
      <CardContent className="pt-5">
        <div className="flex items-start justify-between gap-2">
          <div className="min-w-0">
            <p className="text-xs text-muted-foreground truncate">{label}</p>
            <p className="mt-1 text-2xl font-semibold tabular-nums">{value}</p>
            {sub && <p className="mt-0.5 text-xs text-muted-foreground">{sub}</p>}
          </div>
          <Icon
            className={cn(
              'h-5 w-5 shrink-0 mt-0.5',
              alert === 'crit'
                ? 'text-red-500'
                : alert === 'warn'
                  ? 'text-amber-500'
                  : 'text-muted-foreground',
            )}
          />
        </div>
      </CardContent>
    </Card>
  )
}

function PriorityBadge({ priority }: { priority: string }) {
  const cls =
    priority === 'high'
      ? 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'
      : priority === 'medium'
        ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300'
        : 'bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-300'
  return <span className={cn('rounded-full px-2 py-0.5 text-xs font-medium capitalize', cls)}>{priority}</span>
}

function SLABar({ avgMs, p95Ms, priority }: { avgMs: number; p95Ms: number; priority: string }) {
  const slaMs = priority === 'high' ? 5000 : priority === 'medium' ? 15000 : 30000
  const pct = Math.min(100, (p95Ms / slaMs) * 100)
  const color = pct > 90 ? 'bg-red-500' : pct > 60 ? 'bg-amber-400' : 'bg-emerald-500'
  return (
    <div className="space-y-1">
      <div className="flex justify-between text-xs text-muted-foreground">
        <span>avg {fmtMs(avgMs)}</span>
        <span>p95 {fmtMs(p95Ms)} / SLA {fmtMs(slaMs)}</span>
      </div>
      <div className="h-1.5 w-full rounded-full bg-muted overflow-hidden">
        <div className={cn('h-full rounded-full transition-all', color)} style={{ width: `${pct}%` }} />
      </div>
    </div>
  )
}

// ── Main page ─────────────────────────────────────────────────────────────────

export default function AdminDashboardPage() {
  const [windowKey, setWindowKey] = useState<string>('1h')
  const { data, isLoading, isError, refetch, dataUpdatedAt } = useQuery<AdminOverview>({
    queryKey: ['admin-overview', windowKey],
    queryFn: () => getAdminOverview(windowKey),
    refetchInterval: 3_000,
    staleTime: 1_000,
    retry: 1,
  })

  // Track whether we've received at least one successful fetch for the "Live" badge.
  const [hasData, setHasData] = useState(false)
  useMemo(() => {
    if (data && !hasData) setHasData(true)
  }, [data, hasData])

  const lastUpdated = useMemo(() => {
    if (!dataUpdatedAt) return null
    return new Date(dataUpdatedAt).toLocaleTimeString()
  }, [dataUpdatedAt])

  // ── Kafka: sort topics high → medium → low
  const sortedTopics: KafkaTopicLag[] = useMemo(() => {
    if (!data?.kafka.topics) return []
    return [...data.kafka.topics].sort((a, b) => {
      const pa = PRIORITY_ORDER.indexOf(a.priority)
      const pb = PRIORITY_ORDER.indexOf(b.priority)
      if (pa !== pb) return pa - pb
      return b.lag - a.lag
    })
  }, [data])

  const kafkaTotalEvents = data?.kafka.total_events ?? 0

  // ── Workers: bar chart data and per-client
  const workerByPriority = useMemo(() => {
    if (!data?.workers.by_priority) return []
    return PRIORITY_ORDER.map((p) => ({
      name: p,
      workers: data.workers.by_priority[p] ?? 0,
    }))
  }, [data])

  const workerByChannel = useMemo(() => {
    if (!data?.workers.by_channel) return []
    return Object.entries(data.workers.by_channel)
      .map(([name, workers]) => ({ name, workers }))
      .sort((a, b) => b.workers - a.workers)
  }, [data])

  const workerByClient = useMemo<AdminWorkerClientSummary[]>(() => {
    if (!data?.workers.by_client) return []
    return [...data.workers.by_client].sort((a, b) => b.total - a.total)
  }, [data])

  // ── Cadence
  const cadenceThroughput = useMemo(() => {
    if (!data?.cadence.throughput) return []
    return [...data.cadence.throughput].sort((a, b) => {
      const pa = PRIORITY_ORDER.indexOf(a.priority)
      const pb = PRIORITY_ORDER.indexOf(b.priority)
      return pa !== pb ? pa - pb : b.count - a.count
    })
  }, [data])

  const cadenceRetries = useMemo(() => {
    if (!data?.cadence.retries) return []
    return [...data.cadence.retries].sort((a, b) => b.retry_rate - a.retry_rate)
  }, [data])

  const cadenceSchedule = useMemo(() => {
    if (!data?.cadence.schedule_to_start) return []
    return [...data.cadence.schedule_to_start].sort((a, b) => {
      const pa = PRIORITY_ORDER.indexOf(a.priority)
      const pb = PRIORITY_ORDER.indexOf(b.priority)
      return pa !== pb ? pa - pb : b.p95_ms - a.p95_ms
    })
  }, [data])

  // ── MTTD: bar chart
  const mttdPriorityChart = useMemo(() => {
    if (!data?.mttd_by_priority) return []
    return [...data.mttd_by_priority].sort((a, b) => {
      const pa = PRIORITY_ORDER.indexOf(a.group)
      const pb = PRIORITY_ORDER.indexOf(b.group)
      return pa - pb
    })
  }, [data])

  // ── Top clients bar chart
  const topClientsChart = useMemo(() => {
    if (!data?.top_clients) return []
    return data.top_clients.slice(0, 8).map((c) => ({
      name: c.client_name.length > 12 ? c.client_name.slice(0, 12) + '…' : c.client_name,
      fullName: c.client_name,
      total: c.total,
      failed: c.failed,
    }))
  }, [data])

  if (isLoading) {
    return (
      <div className="space-y-8">
        <PageHeader title="Admin Dashboard" description="System-wide operational overview" />
        <KPISkeleton />
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <CardSkeleton />
          <CardSkeleton />
          <CardSkeleton />
          <CardSkeleton />
        </div>
      </div>
    )
  }

  if (isError || !data) {
    return (
      <div className="space-y-8">
        <PageHeader title="Admin Dashboard" description="System-wide operational overview" />
        <ErrorState description="Failed to load admin overview." onRetry={refetch} />
      </div>
    )
  }

  const { delivery, workers, kafka, top_clients, mttd_by_priority, mttd_by_vendor, cadence, migrations } = data
  const kafkaAlert =
    kafka.enabled && kafka.total_lag > 2000 ? 'crit' : kafka.enabled && kafka.total_lag > 200 ? 'warn' : 'ok'
  const windowLabel = delivery.window_label || 'Selected window'
  const failureAlert = delivery.window.failure_rate > 0.05 ? 'crit' : delivery.window.failure_rate > 0.02 ? 'warn' : 'ok'
  const workerAlert = workers.total < 3 ? 'crit' : workers.total < 10 ? 'warn' : 'ok'

  return (
    <div className="space-y-8 animate-in fade-in duration-300">
      <div className="flex items-start justify-between gap-3 flex-wrap">
        <div className="flex items-center gap-3 flex-wrap">
          <PageHeader
            title="Admin Dashboard"
            description={
              lastUpdated
                ? `System-wide operational overview · ${windowLabel} · updated ${lastUpdated}`
                : `System-wide operational overview · ${windowLabel}`
            }
          />
          {data && (
            <span className="inline-flex items-center gap-1.5 rounded-full bg-emerald-100 dark:bg-emerald-900/30 px-2.5 py-0.5 text-xs font-medium text-emerald-700 dark:text-emerald-300 shrink-0 self-start mt-5">
              <span className="relative flex h-2 w-2">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75" />
                <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500" />
              </span>
              Live
            </span>
          )}
        </div>
        <div className="min-w-[180px]">
          <Select value={windowKey} onValueChange={setWindowKey}>
            <SelectTrigger className="h-9 text-xs bg-card/50">
              <SelectValue placeholder="Window" />
            </SelectTrigger>
            <SelectContent>
              {WINDOW_OPTIONS.map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      {/* ── Row 1: Key stats ──────────────────────────────────────────────── */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
        <StatCard
          label={`Total (${WINDOW_OPTIONS.find((w) => w.value === windowKey)?.label ?? 'window'})`}
          value={fmt(delivery.window.total)}
          sub={`${fmt(delivery.window.sent)} sent`}
          icon={Activity}
        />
        <StatCard
          label={`Failure Rate (${WINDOW_OPTIONS.find((w) => w.value === windowKey)?.label ?? 'window'})`}
          value={fmtPct(delivery.window.failure_rate)}
          sub={`${fmt(delivery.window.failed)} failed`}
          icon={XCircle}
          alert={failureAlert}
        />
        <StatCard
          label="Active Workers"
          value={String(workers.total)}
          sub={`${workerByClient.length} clients · ${Object.keys(workers.by_channel).length} channels`}
          icon={Server}
          alert={workerAlert}
        />
        <StatCard
          label="Kafka Queue Backlog"
          value={kafka.enabled ? fmt(kafka.total_lag) : '—'}
          sub={kafka.enabled ? 'msgs behind' : `disabled (mode: ${kafka.mode || 'unknown'})`}
          icon={Radio}
          alert={kafka.enabled ? kafkaAlert : 'ok'}
        />
      </div>

      {/* ── Row 2: Scaling signals ────────────────────────────────────────── */}
      {(kafkaAlert !== 'ok' || failureAlert !== 'ok' || workerAlert !== 'ok') && (
        <Card className="border-amber-400/50 bg-amber-50 dark:bg-amber-950/20">
          <CardContent className="pt-5">
            <div className="flex items-start gap-3">
              <AlertTriangle className="h-5 w-5 text-amber-500 shrink-0 mt-0.5" />
              <div className="space-y-1">
                <p className="font-medium text-sm">Scaling signals detected</p>
                <ul className="text-sm text-muted-foreground space-y-0.5">
                  {kafkaAlert === 'crit' && (
                    <li>• Kafka lag is critical ({fmt(kafka.total_lag)} msgs) — consider scaling worker pods: <code className="bg-muted rounded px-1">kubectl scale deploy/notification-worker --replicas=+2</code></li>
                  )}
                  {kafkaAlert === 'warn' && (
                    <li>• Kafka lag is elevated ({fmt(kafka.total_lag)} msgs) — monitor and scale if it continues to grow</li>
                  )}
                  {failureAlert === 'crit' && (
                    <li>• Failure rate {fmtPct(delivery.window.failure_rate)} exceeds 5% — check vendor connectivity and rate limits in Settings</li>
                  )}
                  {failureAlert === 'warn' && (
                    <li>• Failure rate {fmtPct(delivery.window.failure_rate)} is elevated (2–5%) — review vendor health</li>
                  )}
                  {workerAlert !== 'ok' && (
                    <li>• Only {workers.total} {cadence?.engine === 'standalone' ? 'dispatcher' : 'Temporal'} workers active — {cadence?.engine === 'standalone' ? 'dispatcher goroutines' : 'WorkerManager'} may need restart or more API keys need to be provisioned</li>
                  )}
                </ul>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* ── Row 3: Kafka queue depth ──────────────────────────────────────── */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Radio className="h-4 w-4 text-muted-foreground" />
              Kafka Topics
            </CardTitle>
            <CardDescription>
              {kafka.enabled
                ? `Consumer lag and total events per channel & priority. RED = >100 messages.${kafkaTotalEvents > 0 ? ` ${kafkaTotalEvents.toLocaleString()} events total.` : ''}`
                : `Kafka is disabled (mode: ${kafka.mode || 'unknown'}). Enable pubsub.mode=kafka to see lag.`}
            </CardDescription>
          </CardHeader>
          <CardContent>
            {!kafka.enabled ? (
              <p className="text-sm text-muted-foreground py-6 text-center">
                Kafka is disabled. Set <code className="bg-muted rounded px-1">NS_PUBSUB_MODE=kafka</code> and start a broker to enable these widgets.
              </p>
            ) : sortedTopics.length === 0 ? (
              <p className="text-sm text-muted-foreground py-6 text-center">
                No Kafka data available yet.
              </p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Channel</TableHead>
                    <TableHead>Priority</TableHead>
                    <TableHead className="text-right">Lag (msgs)</TableHead>
                    <TableHead className="w-28">Status</TableHead>
                    <TableHead className="text-right">Total Events</TableHead>
                    <TableHead className="text-right w-20">% Share</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sortedTopics.map((t) => {
                    const isHigh = t.lag > 100
                    const isWarn = !isHigh && t.lag > 20
                    const pct = kafkaTotalEvents > 0 ? (t.total_events / kafkaTotalEvents) * 100 : 0
                    return (
                      <TableRow key={t.topic}>
                        <TableCell className="capitalize font-medium">{t.channel}</TableCell>
                        <TableCell><PriorityBadge priority={t.priority} /></TableCell>
                        <TableCell className={cn('text-right tabular-nums font-mono', isHigh ? 'text-red-500 font-bold' : isWarn ? 'text-amber-500' : '')}>
                          {t.lag.toLocaleString()}
                        </TableCell>
                        <TableCell>
                          {isHigh ? (
                            <span className="flex items-center gap-1 text-red-500 text-xs"><AlertTriangle className="h-3 w-3" /> Scale now</span>
                          ) : isWarn ? (
                            <span className="flex items-center gap-1 text-amber-500 text-xs"><AlertTriangle className="h-3 w-3" /> Watch</span>
                          ) : (
                            <span className="flex items-center gap-1 text-emerald-500 text-xs"><CheckCircle2 className="h-3 w-3" /> OK</span>
                          )}
                        </TableCell>
                        <TableCell className="text-right tabular-nums font-mono">
                          {t.total_events.toLocaleString()}
                        </TableCell>
                        <TableCell className="text-right tabular-nums text-muted-foreground text-xs">
                          {pct > 0 ? `${pct.toFixed(1)}%` : '—'}
                        </TableCell>
                      </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        {/* Delivery 24h summary */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <BarChart2 className="h-4 w-4 text-muted-foreground" />
              Delivery Summary
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-6">
            {[
              { key: 'window', label: windowLabel, stats: delivery.window },
              { key: 'window_24h', label: 'Last 24 hours', stats: delivery.window_24h },
            ].map((w) => {
              const s = w.stats
              const rate = s.total > 0 ? ((s.total - s.failed) / s.total) * 100 : 0
              return (
                <div key={w.key} className="space-y-2">
                  <p className="text-xs font-medium text-muted-foreground uppercase tracking-wide">{w.label}</p>
                  <div className="grid grid-cols-3 gap-2 text-center">
                    <div>
                      <p className="text-lg font-semibold tabular-nums">{fmt(s.total)}</p>
                      <p className="text-xs text-muted-foreground">total</p>
                    </div>
                    <div>
                      <p className="text-lg font-semibold tabular-nums text-emerald-600">{fmt(s.sent)}</p>
                      <p className="text-xs text-muted-foreground">sent</p>
                    </div>
                    <div>
                      <p className={cn('text-lg font-semibold tabular-nums', s.failed > 0 ? 'text-red-500' : '')}>{fmt(s.failed)}</p>
                      <p className="text-xs text-muted-foreground">failed</p>
                    </div>
                  </div>
                  <div className="h-1.5 w-full rounded-full bg-muted overflow-hidden">
                    <div
                      className={cn('h-full rounded-full', rate > 95 ? 'bg-emerald-500' : rate > 80 ? 'bg-amber-400' : 'bg-red-500')}
                      style={{ width: `${rate}%` }}
                    />
                  </div>
                  <p className="text-xs text-right text-muted-foreground">{rate.toFixed(1)}% success</p>
                </div>
              )
            })}
          </CardContent>
        </Card>
      </div>

      {/* ── Row 4: Workers ────────────────────────────────────────────────── */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Server className="h-4 w-4 text-muted-foreground" />
              Workers by Priority
            </CardTitle>
            <CardDescription>Live counts from worker process. At least 1 {cadence?.engine === 'standalone' ? 'dispatcher' : 'worker'} per channel × priority expected.</CardDescription>
          </CardHeader>
          <CardContent>
            {workers.total === 0 ? (
              <p className="text-sm text-muted-foreground py-6 text-center">
                No {cadence?.engine === 'standalone' ? 'dispatcher' : 'worker'} data — worker process may not be running yet.
              </p>
            ) : (
              <div className="space-y-3">
                {workerByPriority.map((d) => (
                  <div key={d.name} className="space-y-1">
                    <div className="flex justify-between text-sm">
                      <span className="capitalize font-medium">{d.name}</span>
                      <span className="tabular-nums text-muted-foreground">{d.workers} {cadence?.engine === 'standalone' ? 'disp.' : 'workers'}</span>
                    </div>
                    <div className="h-2 w-full rounded-full bg-muted overflow-hidden">
                      <div
                        className="h-full rounded-full transition-all"
                        style={{
                          width: workers.total > 0 ? `${(d.workers / workers.total) * 100}%` : '0%',
                          backgroundColor: PRIORITY_COLOR[d.name] ?? '#6366f1',
                        }}
                      />
                    </div>
                  </div>
                ))}
                <p className="text-xs text-muted-foreground pt-1">{workers.total} {cadence?.engine === 'standalone' ? 'dispatchers' : 'workers'} total across {Object.keys(workers.by_channel).length} channels</p>
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Layers className="h-4 w-4 text-muted-foreground" />
              Workers by Channel
            </CardTitle>
          </CardHeader>
          <CardContent>
            {workerByChannel.length === 0 ? (
              <p className="text-sm text-muted-foreground py-6 text-center">No data available.</p>
            ) : (
              <ResponsiveContainer width="100%" height={180}>
                <BarChart data={workerByChannel} margin={{ top: 0, right: 0, left: -20, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" />
                  <XAxis dataKey="name" tick={{ fontSize: 11 }} />
                  <YAxis tick={{ fontSize: 11 }} allowDecimals={false} />
                  <Tooltip
                    contentStyle={{ fontSize: 12, borderRadius: 8, border: '1px solid hsl(var(--border))', background: 'hsl(var(--popover))' }}
                    labelStyle={{ fontWeight: 600 }}
                  />
                  <Bar dataKey="workers" radius={[4, 4, 0, 0]}>
                    {workerByChannel.map((entry) => (
                      <Cell key={entry.name} fill={CHANNEL_COLOR[entry.name] ?? '#6366f1'} />
                    ))}
                  </Bar>
                </BarChart>
              </ResponsiveContainer>
            )}
          </CardContent>
        </Card>

        {/* Workers per client */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Users className="h-4 w-4 text-muted-foreground" />
              Workers per Client
            </CardTitle>
            <CardDescription>Dedicated worker threads per API key.</CardDescription>
          </CardHeader>
          <CardContent>
            {workerByClient.length === 0 ? (
              <p className="text-sm text-muted-foreground py-6 text-center">No client worker data yet.</p>
            ) : (
              <div className="space-y-2 max-h-56 overflow-y-auto pr-1">
                {workerByClient.map((c) => (
                  <div key={c.client_id || 'global'} className="flex items-center justify-between text-sm py-1 border-b last:border-0">
                    <div className="min-w-0 flex-1">
                      <p className="font-medium truncate">{c.client_name || 'Global'}</p>
                      <div className="flex gap-2 mt-0.5 flex-wrap">
                        {PRIORITY_ORDER.map((p) =>
                          (c.by_priority[p] ?? 0) > 0 ? (
                            <span key={p} className="text-xs" style={{ color: PRIORITY_COLOR[p] }}>
                              {p[0].toUpperCase()}: {c.by_priority[p]}
                            </span>
                          ) : null
                        )}
                      </div>
                    </div>
                    <span className="ml-3 tabular-nums font-semibold text-base shrink-0">{c.total}</span>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* ── Row 5: MTTD ──────────────────────────────────────────────────── */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Gauge className="h-4 w-4 text-muted-foreground" />
              Mean Time to Deliver by Priority (1h)
            </CardTitle>
            <CardDescription>Bar shows p95 vs SLA. Red = SLA at risk.</CardDescription>
          </CardHeader>
          <CardContent>
            {mttd_by_priority.length === 0 ? (
              <p className="text-sm text-muted-foreground py-6 text-center">No delivery data in the last hour.</p>
            ) : (
              <div className="space-y-5">
                {[...mttd_by_priority]
                  .sort((a, b) => PRIORITY_ORDER.indexOf(a.group) - PRIORITY_ORDER.indexOf(b.group))
                  .map((row) => (
                    <div key={row.group} className="space-y-1.5">
                      <div className="flex items-center justify-between">
                        <PriorityBadge priority={row.group} />
                        <span className="text-xs text-muted-foreground tabular-nums">{row.count.toLocaleString()} deliveries</span>
                      </div>
                      <SLABar avgMs={row.avg_ms} p95Ms={row.p95_ms} priority={row.group} />
                    </div>
                  ))}
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <TrendingUp className="h-4 w-4 text-muted-foreground" />
              Mean Time to Deliver by Vendor (1h)
            </CardTitle>
            <CardDescription>Average and p95 delivery latency per provider.</CardDescription>
          </CardHeader>
          <CardContent>
            {mttd_by_vendor.length === 0 ? (
              <p className="text-sm text-muted-foreground py-6 text-center">No delivery data in the last hour.</p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Vendor</TableHead>
                    <TableHead className="text-right">Avg</TableHead>
                    <TableHead className="text-right">p95</TableHead>
                    <TableHead className="text-right">Count</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {mttd_by_vendor.slice(0, 8).map((row) => (
                    <TableRow key={row.group}>
                      <TableCell className="font-medium capitalize">{row.group}</TableCell>
                      <TableCell className="text-right tabular-nums text-sm">{fmtMs(row.avg_ms)}</TableCell>
                      <TableCell className={cn('text-right tabular-nums text-sm', row.p95_ms > 10000 ? 'text-red-500 font-semibold' : row.p95_ms > 5000 ? 'text-amber-500' : '')}>{fmtMs(row.p95_ms)}</TableCell>
                      <TableCell className="text-right tabular-nums text-muted-foreground text-sm">{row.count.toLocaleString()}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>

      {/* ── Row 6: Migration Preview ──────────────────────────────────────── */}
      {migrations && migrations.dry_run_result && (
        <Card className="border-blue-400/50 bg-blue-50/50 dark:bg-blue-950/10">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-sm">
              <Info className="h-4 w-4 text-blue-500" />
              Migration Preview
              <span className="ml-auto text-xs font-normal text-muted-foreground">
                Estimated duration: {migrations.dry_run_result.estimated_duration}
              </span>
            </CardTitle>
            <CardDescription className="text-xs">
              What would happen if you change the orchestration settings (host/port or namespace).
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 text-xs">
              <div className="rounded-lg border bg-card p-3">
                <p className="text-muted-foreground mb-1">Provider</p>
                <p className="text-sm font-semibold capitalize">{migrations.dry_run_result.old_provider}</p>
              </div>

              <div className="rounded-lg border bg-card p-3">
                <p className="text-muted-foreground mb-1">Current</p>
                <p className="text-sm font-semibold">
                  {migrations.dry_run_result.old_provider === 'cadence' ? migrations.dry_run_result.old_host_port : ''}
                  {migrations.dry_run_result.old_provider === 'temporal' ? migrations.dry_run_result.old_host_port : ''}
                  {migrations.dry_run_result.old_provider === 'cadence' && migrations.dry_run_result.old_namespace ? (
                    <span className="block text-xs text-muted-foreground">Domain: {migrations.dry_run_result.old_namespace}</span>
                  ) : null}
                  {migrations.dry_run_result.old_provider === 'temporal' && migrations.dry_run_result.old_namespace ? (
                    <span className="block text-xs text-muted-foreground">Namespace: {migrations.dry_run_result.old_namespace}</span>
                  ) : null}
                </p>
              </div>

              <div className="rounded-lg border bg-card p-3">
                <p className="text-muted-foreground mb-1">Will be changed to</p>
                <p className="text-sm font-semibold">
                  {migrations.dry_run_result.new_provider === 'cadence' ? migrations.dry_run_result.new_host_port : ''}
                  {migrations.dry_run_result.new_provider === 'temporal' ? migrations.dry_run_result.new_host_port : ''}
                  {migrations.dry_run_result.new_provider === 'cadence' && migrations.dry_run_result.new_namespace ? (
                    <span className="block text-xs text-muted-foreground">Domain: {migrations.dry_run_result.new_namespace}</span>
                  ) : null}
                  {migrations.dry_run_result.new_provider === 'temporal' && migrations.dry_run_result.new_namespace ? (
                    <span className="block text-xs text-muted-foreground">Namespace: {migrations.dry_run_result.new_namespace}</span>
                  ) : null}
                </p>
              </div>

              <div className="rounded-lg border bg-card p-3">
                <p className="text-muted-foreground mb-1">Scheduled to transfer</p>
                <p className="text-sm font-semibold tabular-nums">
                  {migrations.dry_run_result.migrated_scheduled_count}
                  {migrations.dry_run_result.total_scheduled_count > 0 && (
                    <span className="text-xs text-muted-foreground font-normal ml-1">
                      / {migrations.dry_run_result.total_scheduled_count}
                    </span>
                  )}
                </p>
              </div>
            </div>

            <div className="rounded-lg border bg-card p-3">
              <p className="text-muted-foreground mb-1 text-xs">In-flight workflows (last 2 minutes)</p>
              <p className="text-lg font-semibold tabular-nums">
                {migrations.dry_run_result.old_workflow_count}
              </p>
              <p className="text-xs text-muted-foreground">
                These will complete on the current orchestration before the migration proceeds.
              </p>
            </div>

            <div className="flex items-start gap-2 rounded-lg bg-yellow-50 dark:bg-yellow-950/20 border border-yellow-200 dark:border-yellow-900/30 p-3 text-xs">
              <AlertTriangle className="h-4 w-4 text-yellow-600 dark:text-yellow-400 shrink-0 mt-0.5" />
              <div className="space-y-1">
                <p className="font-medium text-yellow-800 dark:text-yellow-200">Migration will keep old workers alive</p>
                <p className="text-muted-foreground">
                  Both old and new orchestration workers will run simultaneously until all in-flight workflows complete.
                  This may temporarily increase resource usage.
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* ── Row 6: Top clients ───────────────────────────────────────────── */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Users className="h-4 w-4 text-muted-foreground" />
              Top Clients by Volume (24h)
            </CardTitle>
            <CardDescription>Notification volume across all channels.</CardDescription>
          </CardHeader>
          <CardContent>
            {topClientsChart.length === 0 ? (
              <p className="text-sm text-muted-foreground py-6 text-center">No client data in the last 24 hours.</p>
            ) : (
              <ResponsiveContainer width="100%" height={220}>
                <BarChart data={topClientsChart} layout="vertical" margin={{ top: 0, right: 16, left: 0, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" horizontal={false} />
                  <XAxis type="number" tick={{ fontSize: 11 }} />
                  <YAxis type="category" dataKey="name" width={90} tick={{ fontSize: 11 }} />
                  <Tooltip
                    contentStyle={{ fontSize: 12, borderRadius: 8, border: '1px solid hsl(var(--border))', background: 'hsl(var(--popover))' }}
                    formatter={(value: number, name: string) => [value.toLocaleString(), name === 'failed' ? 'Failed' : 'Total']}
                  />
                  <Bar dataKey="total" fill="#6366f1" radius={[0, 4, 4, 0]} name="Total" />
                  <Bar dataKey="failed" fill="#ef4444" radius={[0, 4, 4, 0]} name="Failed" />
                </BarChart>
              </ResponsiveContainer>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Users className="h-4 w-4 text-muted-foreground" />
              Client Stats (24h)
            </CardTitle>
          </CardHeader>
          <CardContent>
            {top_clients.length === 0 ? (
              <p className="text-sm text-muted-foreground py-6 text-center">No client data in the last 24 hours.</p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Client</TableHead>
                    <TableHead className="text-right">Total</TableHead>
                    <TableHead className="text-right">Sent</TableHead>
                    <TableHead className="text-right">Fail %</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {top_clients.map((c) => (
                    <TableRow key={c.client_id}>
                      <TableCell>
                        <div className="min-w-0">
                          <p className="font-medium truncate max-w-[140px]">{c.client_name}</p>
                          <p className="text-xs text-muted-foreground font-mono truncate max-w-[140px]">{c.client_id.slice(0, 8)}…</p>
                        </div>
                      </TableCell>
                      <TableCell className="text-right tabular-nums">{c.total.toLocaleString()}</TableCell>
                      <TableCell className="text-right tabular-nums text-emerald-600">{c.sent.toLocaleString()}</TableCell>
                      <TableCell className={cn('text-right tabular-nums', c.failure_rate > 0.05 ? 'text-red-500 font-semibold' : c.failure_rate > 0.02 ? 'text-amber-500' : 'text-muted-foreground')}>
                        {fmtPct(c.failure_rate)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>

      {/* ── Row 7: Migration Preview ──────────────────────────────────────── */}
      {migrations && migrations.dry_run_result && (
        <Card className="border-blue-400/50 bg-blue-50/50 dark:bg-blue-950/10">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-sm">
              <ArrowRight className="h-4 w-4 text-blue-500" />
              Migration Preview
              <span className="ml-auto text-xs font-normal text-muted-foreground">
                Estimated duration: {migrations.dry_run_result.estimated_duration}
              </span>
            </CardTitle>
            <CardDescription className="text-xs">
              What would happen if you change the orchestration settings (host/port or namespace).
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 text-xs">
              {/* Provider */}
              <div className="rounded-lg border bg-card p-3">
                <p className="text-muted-foreground mb-1">Provider</p>
                <p className="text-sm font-semibold capitalize">{migrations.dry_run_result.old_provider}</p>
              </div>

              {/* Old Endpoint */}
              <div className="rounded-lg border bg-card p-3">
                <p className="text-muted-foreground mb-1">Current</p>
                <p className="text-sm font-semibold">
                  {migrations.dry_run_result.old_provider === 'cadence' ? migrations.dry_run_result.old_host_port : ''}
                  {migrations.dry_run_result.old_provider === 'temporal' ? migrations.dry_run_result.old_host_port : ''}
                  {migrations.dry_run_result.old_provider === 'cadence' && migrations.dry_run_result.old_namespace ? (
                    <span className="block text-xs text-muted-foreground">Domain: {migrations.dry_run_result.old_namespace}</span>
                  ) : null}
                  {migrations.dry_run_result.old_provider === 'temporal' && migrations.dry_run_result.old_namespace ? (
                    <span className="block text-xs text-muted-foreground">Namespace: {migrations.dry_run_result.old_namespace}</span>
                  ) : null}
                </p>
              </div>

              {/* New Endpoint */}
              <div className="rounded-lg border bg-card p-3">
                <p className="text-muted-foreground mb-1">Will be changed to</p>
                <p className="text-sm font-semibold">
                  {migrations.dry_run_result.new_provider === 'cadence' ? migrations.dry_run_result.new_host_port : ''}
                  {migrations.dry_run_result.new_provider === 'temporal' ? migrations.dry_run_result.new_host_port : ''}
                  {migrations.dry_run_result.new_provider === 'cadence' && migrations.dry_run_result.new_namespace ? (
                    <span className="block text-xs text-muted-foreground">Domain: {migrations.dry_run_result.new_namespace}</span>
                  ) : null}
                  {migrations.dry_run_result.new_provider === 'temporal' && migrations.dry_run_result.new_namespace ? (
                    <span className="block text-xs text-muted-foreground">Namespace: {migrations.dry_run_result.new_namespace}</span>
                  ) : null}
                </p>
              </div>

              {/* Scheduled */}
              <div className="rounded-lg border bg-card p-3">
                <p className="text-muted-foreground mb-1">Scheduled to transfer</p>
                <p className="text-sm font-semibold tabular-nums">
                  {migrations.dry_run_result.migrated_scheduled_count}
                  {migrations.dry_run_result.total_scheduled_count > 0 && (
                    <span className="text-xs text-muted-foreground font-normal ml-1">
                      / {migrations.dry_run_result.total_scheduled_count}
                    </span>
                  )}
                </p>
              </div>
            </div>

            {/* Old workflow count */}
            <div className="rounded-lg border bg-card p-3">
              <p className="text-muted-foreground mb-1 text-xs">In-flight workflows (last 2 minutes)</p>
              <p className="text-lg font-semibold tabular-nums">
                {migrations.dry_run_result.old_workflow_count}
              </p>
              <p className="text-xs text-muted-foreground">
                These will complete on the current orchestration before the migration proceeds.
              </p>
            </div>

            {/* Warning */}
            <div className="flex items-start gap-2 rounded-lg bg-yellow-50 dark:bg-yellow-950/20 border border-yellow-200 dark:border-yellow-900/30 p-3 text-xs">
              <AlertTriangle className="h-4 w-4 text-yellow-600 dark:text-yellow-400 shrink-0 mt-0.5" />
              <div className="space-y-1">
                <p className="font-medium text-yellow-800 dark:text-yellow-200">Migration will keep old workers alive</p>
                <p className="text-muted-foreground">
                  Both old and new orchestration workers will run simultaneously until all in-flight workflows complete.
                  This may temporarily increase resource usage.
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* ── Row 7: Migration Status ──────────────────────────────────────── */}
      {migrations && migrations.active > 0 && (
        <>
          <div className="space-y-2 mt-4">
            <div className="flex items-center gap-2">
              <ArrowRight className="h-4 w-4 text-amber-500" />
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Orchestration Migration{' '}
              <span className="ml-2 inline-flex items-center gap-1 rounded-full bg-amber-100 dark:bg-amber-900/40 px-2.5 py-0.5 text-xs font-medium text-amber-700 dark:text-amber-300">
                  <Loader2 className="h-3 w-3 animate-spin" />
                  {migrations.active} active
                </span>
              </h2>
            </div>
          </div>

          {migrations.migrations.map((mig) => {
            const progress = calcMigrationProgress(mig)
            const phaseLabel =
              mig.status === 'transferring_scheduled'
                ? 'Transferring scheduled notifications…'
                : mig.status === 'waiting_old_workers'
                  ? 'Waiting for old workflows to complete…'
                  : 'Migration in progress…'

            return (
              <Card key={mig.id} className="border-amber-400/50 bg-amber-50/50 dark:bg-amber-950/10">
                <CardHeader>
                  <CardTitle className="flex items-center gap-2 text-sm">
                    <ArrowRight className="h-4 w-4 text-amber-500" />
                    {mig.old_provider} → {mig.new_provider}
                    {mig.client_name && (
                      <span className="ml-1 text-muted-foreground font-normal">· {mig.client_name}</span>
                    )}
                  </CardTitle>
                  <CardDescription className="text-xs">{phaseLabel}</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  {/* Progress bar */}
                  <div className="space-y-1.5">
                    <div className="flex justify-between text-xs text-muted-foreground">
                      <span>Progress</span>
                      <span>{Math.round(progress * 100)}%</span>
                    </div>
                    <div className="h-2 w-full rounded-full bg-muted overflow-hidden">
                      <div
                        className="h-full rounded-full bg-amber-500 transition-all"
                        style={{ width: `${Math.min(100, Math.round(progress * 100))}%` }}
                      />
                    </div>
                  </div>

                  <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 text-xs">
                    {/* Old workflow count */}
                    <div className="rounded-lg border bg-card p-3">
                      <p className="text-muted-foreground mb-1">Old workflows</p>
                      <p className="text-lg font-semibold tabular-nums">
                        {mig.old_workflow_count - mig.completed_old_workflows}
                        <span className="text-xs text-muted-foreground font-normal ml-1">
                          / {mig.old_workflow_count}
                        </span>
                      </p>
                      <p className="text-xs text-muted-foreground">
                        {mig.completed_old_workflows} completed
                      </p>
                    </div>

                    {/* Scheduled transferred */}
                    <div className="rounded-lg border bg-card p-3">
                      <p className="text-muted-foreground mb-1">Scheduled transferred</p>
                      <p className="text-lg font-semibold tabular-nums">
                        {mig.migrated_scheduled_count}
                        {mig.total_scheduled_count > 0 && (
                          <span className="text-xs text-muted-foreground font-normal ml-1">
                            / {mig.total_scheduled_count}
                          </span>
                        )}
                      </p>
                    </div>

                    {/* Old workers */}
                    <div className="rounded-lg border bg-card p-3">
                      <p className="text-muted-foreground mb-1">Old workers active</p>
                      <p className="text-lg font-semibold tabular-nums text-amber-600">
                        {migrations.old_worker_total}
                      </p>
                      {migrations.old_by_client?.length > 0 && (
                        <p className="text-xs text-muted-foreground truncate">
                          {migrations.old_by_client.map((c) => c.client_name).join(', ')}
                        </p>
                      )}
                    </div>

                    {/* New workers */}
                    <div className="rounded-lg border bg-card p-3">
                      <p className="text-muted-foreground mb-1">New workers active</p>
                      <p className="text-lg font-semibold tabular-nums text-emerald-600">
                        {migrations.new_worker_total}
                      </p>
                      {migrations.new_by_client?.length > 0 && (
                        <p className="text-xs text-muted-foreground truncate">
                          {migrations.new_by_client.map((c) => c.client_name).join(', ')}
                        </p>
                      )}
                    </div>
                  </div>

                  {/* Old workers breakdown by priority */}
                  {migrations.old_by_priority &&
                    Object.keys(migrations.old_by_priority).length > 0 && (
                      <div className="text-xs">
                        <p className="text-muted-foreground mb-1 font-medium">Old workers by priority:</p>
                        <div className="flex gap-3 flex-wrap">
                          {PRIORITY_ORDER.map((p) =>
                            (migrations!.old_by_priority[p] ?? 0) > 0 ? (
                              <span
                                key={p}
                                className="inline-flex items-center gap-1 rounded-full bg-muted px-2 py-0.5"
                                style={{ color: PRIORITY_COLOR[p] }}
                              >
                                {p}: {migrations!.old_by_priority[p]}
                              </span>
                            ) : null,
                          )}
                        </div>
                      </div>
                    )}

                  {/* Old workers breakdown by channel */}
                  {migrations.old_by_channel &&
                    Object.keys(migrations.old_by_channel).length > 0 && (
                      <div className="text-xs">
                        <p className="text-muted-foreground mb-1 font-medium">Old workers by channel:</p>
                        <div className="flex gap-3 flex-wrap">
                          {Object.entries(migrations.old_by_channel).map(([ch, count]) => (
                            <span
                              key={ch}
                              className="inline-flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 capitalize"
                              style={{ color: CHANNEL_COLOR[ch] ?? '#6366f1' }}
                            >
                              {ch}: {count}
                            </span>
                          ))}
                        </div>
                      </div>
                    )}

                  {/* Duration */}
                  <p className="text-xs text-muted-foreground">
                    Started {new Date(mig.started_at).toLocaleString()}
                    {mig.old_workflow_count > 0 &&
                      ` · ${mig.old_workflow_count - mig.completed_old_workflows} workflows remaining`}
                  </p>
                </CardContent>
              </Card>
            )
          })}
        </>
      )}

      {/* Completed migration notification banner */}
      {migrations && migrations.active === 0 && migrations.migrations?.length > 0 && (
        <Card className="border-emerald-400/50 bg-emerald-50 dark:bg-emerald-950/10">
          <CardContent className="pt-5">
            <div className="flex items-start gap-3">
              <CheckCircle2 className="h-5 w-5 text-emerald-500 shrink-0 mt-0.5" />
              <div className="space-y-1">
                <p className="font-medium text-sm">All orchestration migrations completed</p>
                <p className="text-xs text-muted-foreground">
                  {migrations.migrations.filter((m) => m.status === 'completed').length} migrations completed
                  successfully.
                  {migrations.migrations.filter((m) => m.status === 'failed').length > 0 &&
                    ` ${migrations.migrations.filter((m) => m.status === 'failed').length} failed.`}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* ── Row 8: Workflow Cadence ──────────────────────────────────────── */}
      <div className="space-y-2">
        <div className="flex items-center gap-2">
          <Workflow className="h-4 w-4 text-muted-foreground" />
          <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
            Workflow Cadence
            {cadence?.engine && (
              <span className="ml-2 normal-case font-normal capitalize">({cadence.engine})</span>
            )}
          </h2>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Throughput */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Activity className="h-4 w-4 text-muted-foreground" />
              Throughput (1h)
            </CardTitle>
            <CardDescription>Workflows started per channel × priority in the last hour.</CardDescription>
          </CardHeader>
          <CardContent>
            {cadenceThroughput.length === 0 ? (
              <p className="text-sm text-muted-foreground py-6 text-center">No workflow activity in the last hour.</p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Channel</TableHead>
                    <TableHead>Priority</TableHead>
                    <TableHead className="text-right">Count</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {cadenceThroughput.map((r) => (
                    <TableRow key={`${r.channel}-${r.priority}`}>
                      <TableCell className="capitalize font-medium">{r.channel}</TableCell>
                      <TableCell><PriorityBadge priority={r.priority} /></TableCell>
                      <TableCell className="text-right tabular-nums font-semibold">{r.count.toLocaleString()}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        {/* Retry statistics */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Repeat className="h-4 w-4 text-muted-foreground" />
              Retry Statistics (1h)
            </CardTitle>
            <CardDescription>Activity retry rates by channel. High retry rate indicates vendor or config issues.</CardDescription>
          </CardHeader>
          <CardContent>
            {cadenceRetries.length === 0 ? (
              <p className="text-sm text-muted-foreground py-6 text-center">No attempt data in the last hour.</p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Channel</TableHead>
                    <TableHead className="text-right">Retry %</TableHead>
                    <TableHead className="text-right">Avg attempts</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {cadenceRetries.map((r) => {
                    const isHigh = r.retry_rate > 0.2
                    const isMed = !isHigh && r.retry_rate > 0.05
                    return (
                      <TableRow key={r.channel}>
                        <TableCell>
                          <div className="capitalize font-medium">{r.channel}</div>
                          <div className="text-xs text-muted-foreground">{r.total_notifications.toLocaleString()} notifs · {r.retried_count.toLocaleString()} retried</div>
                        </TableCell>
                        <TableCell className={cn('text-right tabular-nums font-semibold', isHigh ? 'text-red-500' : isMed ? 'text-amber-500' : 'text-emerald-600')}>
                          {fmtPct(r.retry_rate)}
                        </TableCell>
                        <TableCell className="text-right tabular-nums text-muted-foreground">
                          {r.avg_attempts.toFixed(2)}×
                        </TableCell>
                      </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        {/* Schedule-to-start latency */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Clock className="h-4 w-4 text-muted-foreground" />
              Schedule-to-Start (1h)
            </CardTitle>
            <CardDescription>Time from notification created to first workflow activity attempt (Kafka→Temporal latency).</CardDescription>
          </CardHeader>
          <CardContent>
            {cadenceSchedule.length === 0 ? (
              <p className="text-sm text-muted-foreground py-6 text-center">No schedule-to-start data in the last hour.</p>
            ) : (
              <div className="space-y-4">
                {cadenceSchedule.map((r) => {
                  const slaMs = r.priority === 'high' ? 2000 : r.priority === 'medium' ? 5000 : 10000
                  const pct = Math.min(100, (r.p95_ms / slaMs) * 100)
                  const color = pct > 80 ? 'bg-red-500' : pct > 50 ? 'bg-amber-400' : 'bg-emerald-500'
                  return (
                    <div key={`${r.channel}-${r.priority}`} className="space-y-1">
                      <div className="flex items-center justify-between text-sm">
                        <span className="capitalize font-medium">{r.channel}</span>
                        <PriorityBadge priority={r.priority} />
                      </div>
                      <div className="flex justify-between text-xs text-muted-foreground">
                        <span>avg {fmtMs(r.avg_ms)}</span>
                        <span>p95 {fmtMs(r.p95_ms)}</span>
                      </div>
                      <div className="h-1.5 w-full rounded-full bg-muted overflow-hidden">
                        <div className={cn('h-full rounded-full transition-all', color)} style={{ width: `${pct}%` }} />
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

