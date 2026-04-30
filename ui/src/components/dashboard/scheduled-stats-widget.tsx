'use client'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { CalendarClock, Clock, Loader2, CheckCircle2, Hourglass } from 'lucide-react'
import type { ScheduledStats } from '@/types'

interface ScheduledStatsWidgetProps {
  data: ScheduledStats | undefined
  isLoading: boolean
  rangeLabel: string
}

function formatLatency(ms: number | null): string {
  if (ms === null || ms === undefined) return '—'
  if (ms < 1000) return `${Math.round(ms)} ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)} s`
  if (ms < 3_600_000) return `${(ms / 60_000).toFixed(1)} min`
  return `${(ms / 3_600_000).toFixed(1)} hr`
}

export function ScheduledStatsWidget({ data, isLoading, rangeLabel }: ScheduledStatsWidgetProps) {
  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-center gap-2">
          <CalendarClock size={16} className="text-violet-500" />
          <CardTitle className="text-base">Scheduled Notifications</CardTitle>
          <span className="text-xs text-muted-foreground ml-1">({rangeLabel})</span>
        </div>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="flex items-center gap-2 text-muted-foreground text-sm py-4">
            <Loader2 size={14} className="animate-spin" />
            Loading…
          </div>
        ) : !data || data.total_scheduled === 0 ? (
          <div className="text-center py-6 text-sm text-muted-foreground italic">
            No scheduled notifications in this period
          </div>
        ) : (
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
            <Stat
              icon={<CalendarClock size={15} className="text-violet-500" />}
              label="Total Scheduled"
              value={data.total_scheduled.toLocaleString()}
            />
            <Stat
              icon={<Hourglass size={15} className="text-amber-500" />}
              label="Pending"
              value={data.pending.toLocaleString()}
              dimmed={data.pending === 0}
            />
            <Stat
              icon={<CheckCircle2 size={15} className="text-emerald-500" />}
              label="Delivered"
              value={data.delivered.toLocaleString()}
              dimmed={data.delivered === 0}
            />
            <Stat
              icon={<Clock size={15} className="text-sky-500" />}
              label="Avg Delivery Latency"
              value={formatLatency(data.avg_delivery_latency_ms)}
              subtitle="from scheduled time"
            />
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function Stat({
  icon,
  label,
  value,
  subtitle,
  dimmed,
}: {
  icon: React.ReactNode
  label: string
  value: string
  subtitle?: string
  dimmed?: boolean
}) {
  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
        {icon}
        <span>{label}</span>
      </div>
      <p className={`text-2xl font-semibold tabular-nums ${dimmed ? 'text-muted-foreground' : ''}`}>{value}</p>
      {subtitle && <p className="text-[11px] text-muted-foreground">{subtitle}</p>}
    </div>
  )
}
