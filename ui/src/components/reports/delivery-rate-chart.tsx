'use client'

import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from 'recharts'
import type { ReportSummary } from '@/types'
import type { Channel } from '@/types'
import { formatDateShort } from '@/lib/utils'

interface DeliveryRateChartProps {
  reports: ReportSummary[]
}

const CHANNEL_COLORS: Record<Channel | string, string> = {
  email:     '#3b82f6',
  sms:       '#22c55e',
  push:      '#f97316',
  websocket: '#06b6d4',
  webhook:   '#ec4899',
}

export function DeliveryRateChart({ reports }: DeliveryRateChartProps) {
  // Group by date, each entry has a key per channel
  const dateMap = new Map<string, any>()
  reports.forEach((r) => {
    const key = formatDateShort(r.date)
    if (!dateMap.has(key)) dateMap.set(key, { date: key })
    dateMap.get(key)![r.channel] = Math.round(r.success_rate * 100)
  })
  const chartData = Array.from(dateMap.values()).sort((a, b) =>
    String(a.date).localeCompare(String(b.date)),
  )

  const channels = [...new Set(reports.map((r) => r.channel))]

  return (
    <ResponsiveContainer width="100%" height={300}>
      <LineChart data={chartData} margin={{ top: 5, right: 20, left: 0, bottom: 5 }}>
        <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
        <XAxis dataKey="date" tick={{ fontSize: 12 }} />
        <YAxis
          tickFormatter={(v) => `${v}%`}
          domain={[0, 100]}
          tick={{ fontSize: 12 }}
        />
        <Tooltip
          formatter={(value: number, name: string) => [`${value}%`, name]}
          contentStyle={{ borderRadius: '8px', border: '1px solid hsl(var(--border))' }}
        />
        <Legend />
        {channels.map((ch) => (
          <Line
            key={ch}
            type="monotone"
            dataKey={ch}
            stroke={CHANNEL_COLORS[ch] ?? '#94a3b8'}
            strokeWidth={2}
            dot={false}
            activeDot={{ r: 4 }}
          />
        ))}
      </LineChart>
    </ResponsiveContainer>
  )
}
