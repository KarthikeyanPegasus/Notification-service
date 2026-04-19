'use client'

import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from 'recharts'
import type { ReportSummary } from '@/types'

interface LatencyChartProps {
  reports: ReportSummary[]
}

export function LatencyChart({ reports }: LatencyChartProps) {
  // Aggregate latest data per channel
  const channelMap = new Map<string, ReportSummary>()
  reports.forEach((r) => {
    const existing = channelMap.get(r.channel)
    if (!existing || r.date > existing.date) {
      channelMap.set(r.channel, r)
    }
  })

  const chartData = Array.from(channelMap.values()).map((r) => ({
    channel: r.channel.charAt(0).toUpperCase() + r.channel.slice(1),
    p50: r.p50_latency_ms,
    p95: r.p95_latency_ms,
  }))

  return (
    <ResponsiveContainer width="100%" height={300}>
      <BarChart data={chartData} margin={{ top: 5, right: 20, left: 0, bottom: 5 }}>
        <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
        <XAxis dataKey="channel" tick={{ fontSize: 12 }} />
        <YAxis
          tickFormatter={(v) => `${v}ms`}
          tick={{ fontSize: 12 }}
        />
        <Tooltip
          formatter={(value: number, name: string) => [`${value}ms`, name === 'p50' ? 'p50 Latency' : 'p95 Latency']}
          contentStyle={{ borderRadius: '8px', border: '1px solid hsl(var(--border))' }}
        />
        <Legend formatter={(value) => value === 'p50' ? 'p50 Latency' : 'p95 Latency'} />
        <Bar dataKey="p50" fill="#3b82f6" radius={[4, 4, 0, 0]} />
        <Bar dataKey="p95" fill="#f97316" radius={[4, 4, 0, 0]} />
      </BarChart>
    </ResponsiveContainer>
  )
}
