'use client'

import { useQuery } from '@tanstack/react-query'
import { PageHeader } from '@/components/shared/page-header'
import { KpiCard } from '@/components/dashboard/kpi-card'
import { ChannelHealthCard } from '@/components/dashboard/channel-health-card'
import { LiveFeed } from '@/components/dashboard/live-feed'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { KPISkeleton, CardSkeleton } from '@/components/shared/loading-skeleton'
import {
  getReportsScoped,
  getNotificationsScoped,
  getIngressBreakdownScoped,
  getVendorHealth,
  getVendorBilling,
  getVendorConfigsScoped,
} from '@/lib/api'
import { getDaysAgo, formatPercent, formatNumber } from '@/lib/utils'
import type { ChannelHealth, Channel } from '@/types'
import { ClientScopeSelect } from '@/components/shared/client-scope-select'
import { VendorStatusCard } from '@/components/dashboard/vendor-status-card'
import { VendorBillingCard } from '@/components/dashboard/vendor-billing-card'
import { Badge } from '@/components/ui/badge'
import {
  Send,
  CheckCircle2,
  XCircle,
  Clock,
  Globe,
  Radio,
  Database,
  Terminal,
  Zap,
} from 'lucide-react'
import { useState, useMemo } from 'react'

const CHANNELS: Channel[] = ['email', 'sms', 'push', 'websocket', 'webhook', 'slack']

const VENDOR_TO_CHANNEL: Record<string, Channel> = {
  ses: 'email',
  mailgun: 'email',
  smtp: 'email',
  twilio: 'sms',
  plivo: 'sms',
  vonage: 'sms',
  fcm: 'push',
  slack: 'slack',
  webhook: 'webhook',
}

export default function DashboardPage() {
  const [apiKeyId, setApiKeyId] = useState<string | undefined>(undefined)

  const now = new Date().toISOString()
  const dayAgo = getDaysAgo(1)
  const reportsQuery = useQuery({
    queryKey: ['reports', 'dashboard', apiKeyId ?? 'global'],
    queryFn: () => getReportsScoped({ date_from: dayAgo, date_to: now }, apiKeyId),
    retry: 1,
  })

  const recentQuery = useQuery({
    queryKey: ['notifications', 'dashboard-kpi', apiKeyId ?? 'global'],
    queryFn: () => getNotificationsScoped({ page: 1, page_size: 100 } as any, apiKeyId),
    retry: 1,
  })

  const ingressQuery = useQuery({
    queryKey: ['ingress', 'dashboard', apiKeyId ?? 'global'],
    queryFn: () => getIngressBreakdownScoped({ date_from: dayAgo, date_to: now } as any, apiKeyId),
    retry: 1,
  })

  // Real-time Vendor Health (10s refresh)
  const vendorQuery = useQuery({
    queryKey: ['vendor-health', apiKeyId ?? 'global'],
    queryFn: () => getVendorHealth(apiKeyId),
    refetchInterval: 10000,
    retry: 1,
  })

  const billingQuery = useQuery({
    queryKey: ['vendor-billing', apiKeyId ?? 'global'],
    queryFn: () => getVendorBilling(apiKeyId),
    refetchInterval: 30000,
    retry: 1,
  })

  const configQuery = useQuery({
    queryKey: ['vendor-configs', apiKeyId ?? 'global'],
    queryFn: () => getVendorConfigsScoped(apiKeyId),
    retry: 1,
  })

  const reports = Array.isArray(reportsQuery.data) ? reportsQuery.data : []
  const ingressData = Array.isArray(ingressQuery.data) ? ingressQuery.data : []
  const recentData = Array.isArray(recentQuery.data?.data) ? recentQuery.data.data : []
  const configs = Array.isArray(configQuery.data) ? configQuery.data : []

  // Determine connected channels
  const connectedChannels = useMemo(() => {
    const set = new Set<Channel>()
    set.add('websocket') // Always "connected" as internal channel
    configs.forEach((c) => {
      if (c.is_active) {
        const chan = VENDOR_TO_CHANNEL[c.vendor_type]
        if (chan) set.add(chan)
      }
    })
    // Also include if there was any activity in last 24h
    recentData.forEach((n) => {
      if (n.channel) set.add(n.channel as Channel)
    })
    return set
  }, [configs, recentData])

  // Compute KPIs from aggregated reports (full 24h window)
  const kpis = useMemo(() => {
    const totals = reports.reduce(
      (acc, r) => {
        acc.total += r.total
        acc.sent += r.sent
        acc.delivered += r.delivered
        acc.failed += r.failed
        return acc
      },
      { total: 0, sent: 0, delivered: 0, failed: 0 },
    )

    // Fallback to recent data if reports are empty (e.g. first hour of the day)
    if (totals.total === 0 && recentData.length > 0) {
      totals.total = recentData.length
      totals.delivered = recentData.filter((n) => n.status?.toLowerCase() === 'delivered').length
      totals.sent = recentData.filter((n) => ['sent', 'delivered'].includes(n.status?.toLowerCase())).length
      totals.failed = recentData.filter((n) => n.status?.toLowerCase() === 'failed').length
    }

    return {
      total_sent: totals.sent,
      success_rate: totals.total > 0 ? totals.delivered / totals.total : 0,
      failed: totals.failed,
      pending: recentData.filter((n) => n.status?.toLowerCase() === 'pending').length,
    }
  }, [reports, recentData])

  // Build channel health from reports
  const channelHealth: ChannelHealth[] = CHANNELS.filter(ch => connectedChannels.has(ch)).map((ch) => {
    const channelReports = reports.filter((r) => r.channel === ch)
    const channelActivity = recentData.filter((n) => n.channel === ch)
    const latestFailed = channelActivity.find((n) => n.status?.toLowerCase() === 'failed')
    const errorStatus = latestFailed?.attempts?.[0]?.error || null

    // Aggregate stats from all reports in the window
    const totals = channelReports.reduce((acc, r) => {
      acc.total += r.total
      acc.sent += r.sent
      acc.delivered += r.delivered
      acc.failed += r.failed
      acc.bounced += r.bounced
      return acc
    }, { total: 0, sent: 0, delivered: 0, failed: 0, bounced: 0 })

    const isConnected = connectedChannels.has(ch)
    
    // Use recent activity if no report data yet (e.g. today's first messages)
    if (totals.total === 0 && channelActivity.length > 0) {
      totals.total = channelActivity.length
      totals.delivered = channelActivity.filter((n) => n.status?.toLowerCase() === 'delivered').length
      totals.sent = channelActivity.filter((n) => ['sent', 'delivered'].includes(n.status?.toLowerCase())).length
      totals.failed = channelActivity.filter((n) => n.status?.toLowerCase() === 'failed').length
      totals.bounced = channelActivity.filter((n) => n.status?.toLowerCase() === 'bounced').length
    }

    const successCount = totals.delivered + totals.sent
    const rate = totals.total > 0 ? successCount / totals.total : 0
    const bounceRate = totals.total > 0 ? totals.bounced / totals.total : undefined
    
    // Status logic: healthy if idle OR high success rate. Down if failing.
    let status: ChannelHealth['status'] = 'healthy'
    if (totals.total > 0) {
      if (rate < 0.5 || totals.failed > successCount) status = 'down'
      else if (rate < 0.9) status = 'degraded'
    } else if (!isConnected) {
      status = 'down'
    }

    return {
      channel: ch,
      status,
      success_rate: rate,
      total_24h: totals.total,
      uptime: totals.total > 0 ? (totals.total - totals.failed) / totals.total : (isConnected ? 1.0 : 0),
      payment_status: 'paid',
      error_status: errorStatus,
      bounce_rate: bounceRate,
    }
  })

  const isLoadingKPIs = recentQuery.isLoading || reportsQuery.isLoading

  return (
    <div className="space-y-8">
      <div className="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
        <PageHeader
          title="Dashboard"
          description="Real-time overview of your notification service"
        />
        <ClientScopeSelect className="w-full md:w-72" includeAll onScopeChange={setApiKeyId} />
      </div>

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

      {/* Vendor Health Section (Real-time) */}
      <div>
        <div className="flex items-center gap-2 mb-4">
          <Zap className="text-yellow-500 fill-yellow-500" size={20} />
          <h2 className="text-lg font-semibold">Vendor Connectivity & Performance</h2>
          <Badge variant="outline" className="ml-auto text-[10px] animate-pulse border-yellow-500/50 text-yellow-600">LIVE</Badge>
        </div>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {vendorQuery.isLoading ? (
            Array.from({ length: 4 }).map((_, i) => <CardSkeleton key={i} />)
          ) : vendorQuery.data && vendorQuery.data.length > 0 ? (
            vendorQuery.data.map((v) => (
              <VendorStatusCard key={v.provider} metric={v} />
            ))
          ) : (
            <div className="col-span-full p-8 text-center border border-dashed rounded-xl bg-muted/10 text-muted-foreground italic text-sm">
              No active vendor telemetry detected in the current window.
            </div>
          )}
        </div>
      </div>

      {/* Vendor Billing & Cost */}
      <div>
        <div className="flex items-center gap-2 mb-4">
          <Zap className="text-amber-500" size={20} />
          <h2 className="text-lg font-semibold">Vendor Billing & Cost</h2>
        </div>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {billingQuery.isLoading ? (
            Array.from({ length: 4 }).map((_, i) => <CardSkeleton key={i} />)
          ) : billingQuery.data && billingQuery.data.length > 0 ? (
            billingQuery.data.map((b) => (
              <VendorBillingCard key={b.provider} billing={b} />
            ))
          ) : (
            <div className="col-span-full p-8 text-center border border-dashed rounded-xl bg-muted/10 text-muted-foreground italic text-sm">
              No vendor billing data available for the current scope.
            </div>
          )}
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
          <LiveFeed apiKeyId={apiKeyId} />
        </CardContent>
      </Card>
    </div>
  )
}
