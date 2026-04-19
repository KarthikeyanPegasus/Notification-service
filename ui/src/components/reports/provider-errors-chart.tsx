'use client'

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
import type { ReportSummary } from '@/types'

interface ProviderErrorsChartProps {
  reports: ReportSummary[]
}

const COLORS = ['#ef4444', '#f97316', '#eab308', '#22c55e', '#3b82f6', '#a855f7']

export function ProviderErrorsChart({ reports }: ProviderErrorsChartProps) {
  // Aggregate failed count per channel (latest data)
  const channelMap = new Map<string, ReportSummary>()
  reports.forEach((r) => {
    const existing = channelMap.get(r.channel)
    if (!existing || r.date > existing.date) {
      channelMap.set(r.channel, r)
    }
  })

  const chartData = Array.from(channelMap.values())
    .map((r) => ({
      channel: r.channel.charAt(0).toUpperCase() + r.channel.slice(1),
      failed: r.failed,
      errorRate: Number(((r.failed / Math.max(r.total, 1)) * 100).toFixed(1)),
    }))
    .sort((a, b) => b.failed - a.failed)

  return (
    <ResponsiveContainer width="100%" height={300}>
      <BarChart data={chartData} margin={{ top: 5, right: 20, left: 0, bottom: 5 }} layout="vertical">
        <CartesianGrid strokeDasharray="3 3" className="stroke-border" horizontal={false} />
        <XAxis type="number" tick={{ fontSize: 12 }} />
        <YAxis dataKey="channel" type="category" tick={{ fontSize: 12 }} width={70} />
        <Tooltip
          formatter={(value: number, name: string) => [
            name === 'failed' ? `${value} failures` : `${value}% error rate`,
            name === 'failed' ? 'Failed Count' : 'Error Rate',
          ]}
          contentStyle={{ borderRadius: '8px', border: '1px solid hsl(var(--border))' }}
        />
        <Bar dataKey="failed" radius={[0, 4, 4, 0]}>
          {chartData.map((_, index) => (
            <Cell key={`cell-${index}`} fill={COLORS[index % COLORS.length]} />
          ))}
        </Bar>
      </BarChart>
    </ResponsiveContainer>
  )
}
