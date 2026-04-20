'use client'

import { useQuery } from '@tanstack/react-query'
import { PageHeader } from '@/components/shared/page-header'
import { KpiCard } from '@/components/dashboard/kpi-card'
import { ChannelHealthCard } from '@/components/dashboard/channel-health-card'
import { LiveFeed } from '@/components/dashboard/live-feed'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { KPISkeleton, CardSkeleton } from '@/components/shared/loading-skeleton'
import { getReports, getNotifications, getIngressBreakdown } from '@/lib/api'
import { getDaysAgo, formatPercent, formatNumber } from '@/lib/utils'
import type { ChannelHealth, Channel } from '@/types'
import {
  Send,
  CheckCircle2,
  XCircle,
  Clock,
  Globe,
  Radio,
  Database,
  Terminal,
} from 'lucide-react'

const CHANNELS: Channel[] = ['email', 'sms', 'push', 'websocket', 'webhook']

export default function DashboardPage() {
  const now = new Date().toISOString()
  const dayAgo = getDaysAgo(1)

  const reportsQuery = useQuery({
    queryKey: ['reports', 'dashboard'],
    queryFn: () => getReports({ date_from: dayAgo, date_to: now }),
    retry: 1,
  })

  const recentQuery = useQuery({
    queryKey: ['notifications', 'dashboard-kpi'],
    queryFn: () => getNotifications({ page: 1, page_size: 100 }),
    retry: 1,
  })

  const ingressQuery = useQuery({
    queryKey: ['ingress', 'dashboard'],
    queryFn: () => getIngressBreakdown({ date_from: dayAgo, date_to: now }),
    retry: 1,
  })

  const reports = Array.isArray(reportsQuery.data) ? reportsQuery.data : []
  const ingressData = Array.isArray(ingressQuery.data) ? ingressQuery.data : []
  const recentData = Array.isArray(recentQuery.data?.data) ? recentQuery.data.data : []

  // Compute KPIs from recent notifications (API returns lowercase statuses)
  const kpis = {
    total_sent: recentData.filter((n) => ['sent', 'delivered'].includes(n.status?.toLowerCase())).length,
    success_rate:
      recentData.length > 0
        ? recentData.filter((n) => n.status?.toLowerCase() === 'delivered').length / recentData.length
        : 0,
    failed: recentData.filter((n) => n.status?.toLowerCase() === 'failed').length,
    pending: recentData.filter((n) => n.status?.toLowerCase() === 'pending').length,
  }

  // Build channel health from reports
  const channelHealth: ChannelHealth[] = CHANNELS.map((ch) => {
    const channelReports = reports.filter((r) => r.channel === ch)
    if (channelReports.length === 0) {
      // No report data — check if channel has recent activity
      const channelActivity = recentData.filter((n) => n.channel === ch)
      const hasActivity = channelActivity.length > 0
      const delivered = channelActivity.filter((n) => n.status?.toLowerCase() === 'delivered').length
      const rate = hasActivity ? delivered / channelActivity.length : 0
      return {
        channel: ch,
        status: hasActivity ? (rate >= 0.95 ? 'healthy' : rate >= 0.8 ? 'degraded' : 'down') : 'down',
        success_rate: rate,
        total_24h: channelActivity.length,
      }
    }
    const latest = channelReports.sort((a, b) => b.date.localeCompare(a.date))[0]
    const rate = latest?.success_rate ?? 0
    return {
      channel: ch,
      status: rate >= 0.95 ? 'healthy' : rate >= 0.8 ? 'degraded' : 'down',
      success_rate: rate,
      total_24h: latest?.total ?? 0,
    }
  })

  const isLoadingKPIs = recentQuery.isLoading || reportsQuery.isLoading

  return (
    <div className="space-y-8">
      <PageHeader
        title="Dashboard"
        description="Real-time overview of your notification service"
      />

      {/* KPI Cards */}
      {isLoadingKPIs ? (
        <KPISkeleton />
      ) : (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <KpiCard
            title="Total Sent (24h)"
            value={kpis ? formatNumber(kpis.total_sent) : '—'}
            description="SENT + DELIVERED"
            icon={Send}
          />
          <KpiCard
            title="Success Rate"
            value={kpis ? formatPercent(kpis.success_rate) : '—'}
            description="Delivered / total"
            icon={CheckCircle2}
          />
          <KpiCard
            title="Failed"
            value={kpis ? formatNumber(kpis.failed) : '—'}
            description="Last 24 hours"
            icon={XCircle}
          />
          <KpiCard
            title="Pending"
            value={kpis ? formatNumber(kpis.pending) : '—'}
            description="Awaiting delivery"
            icon={Clock}
          />
        </div>
      )}

      {/* Channel Health */}
      <div>
        <h2 className="text-lg font-semibold mb-4">Channel Health</h2>
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-6">
          {isLoadingKPIs
            ? Array.from({ length: 6 }).map((_, i) => <CardSkeleton key={i} />)
            : channelHealth.map((h) => (
                <ChannelHealthCard key={h.channel} health={h} />
              ))}
        </div>
      </div>

      {/* Ingress Sources */}
      <div>
        <h2 className="text-lg font-semibold mb-4">Ingress Sources (24h)</h2>
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          {ingressQuery.isLoading ? (
            Array.from({ length: 4 }).map((_, i) => <CardSkeleton key={i} />)
          ) : ingressData.length > 0 ? (
            ingressData.map((item) => {
              let Icon = Terminal
              if (item.source === 'api') Icon = Globe
              if (item.source === 'pubsub') Icon = Radio
              if (item.source === 'redis') Icon = Database
              
              return (
                <KpiCard
                  key={item.source}
                  title={item.source.toUpperCase()}
                  value={formatNumber(item.count)}
                  description="Requests"
                  icon={Icon}
                />
              )
            })
          ) : (
            <div className="col-span-full p-8 text-center border rounded-lg bg-muted/20 text-muted-foreground">
              No ingress data for the selected period
            </div>
          )}
        </div>
      </div>

      {/* Live Feed */}
      <Card>
        <CardHeader>
          <CardTitle>Recent Notifications</CardTitle>
        </CardHeader>
        <CardContent>
          <LiveFeed />
        </CardContent>
      </Card>
    </div>
  )
}
