'use client'

import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { PageHeader } from '@/components/shared/page-header'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { DeliveryRateChart } from '@/components/reports/delivery-rate-chart'
import { LatencyChart } from '@/components/reports/latency-chart'
import { ProviderErrorsChart } from '@/components/reports/provider-errors-chart'
import { ChartSkeleton } from '@/components/shared/loading-skeleton'
import { ErrorState } from '@/components/shared/error-state'
import { getReports } from '@/lib/api'
import { getDaysAgo, formatPercent, formatNumber, formatLatency } from '@/lib/utils'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { cn } from '@/lib/utils'

type DateRange = 7 | 30 | 90

export default function ReportsPage() {
  const [range, setRange] = useState<DateRange>(7)

  const now = new Date().toISOString()
  const from = getDaysAgo(range)

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ['reports', range],
    queryFn: () => getReports({ date_from: from, date_to: now }),
    retry: 1,
  })

  const reports = data ?? []

  // Aggregate totals per channel (latest entry per channel)
  const channelMap = new Map<string, any>()
  reports.forEach((r) => {
    const existing = channelMap.get(r.channel)
    if (!existing || r.date > existing.date) channelMap.set(r.channel, r)
  })
  const summary = Array.from(channelMap.values())

  return (
    <div className="space-y-8">
      <PageHeader
        title="Reports"
        description="Delivery metrics, latency, and error analytics"
        breadcrumbs={[{ label: 'Dashboard', href: '/dashboard' }, { label: 'Reports' }]}
        actions={
          <div className="flex rounded-md border overflow-hidden">
            {([7, 30, 90] as DateRange[]).map((d) => (
              <button
                key={d}
                onClick={() => setRange(d)}
                className={cn(
                  'px-3 py-1.5 text-sm font-medium transition-colors',
                  range === d
                    ? 'bg-primary text-primary-foreground'
                    : 'bg-background hover:bg-accent text-foreground',
                )}
              >
                {d}d
              </button>
            ))}
          </div>
        }
      />

      {isError && reports.length === 0 ? (
        <ErrorState onRetry={() => refetch()} />
      ) : (
        <>
          {/* Delivery Rate Chart */}
          <Card>
            <CardHeader>
              <CardTitle>Delivery Rate by Channel</CardTitle>
              <CardDescription>Percentage of notifications delivered successfully over time</CardDescription>
            </CardHeader>
            <CardContent>
              {isLoading ? <ChartSkeleton /> : <DeliveryRateChart reports={reports} />}
            </CardContent>
          </Card>

          {/* Latency Chart */}
          <Card>
            <CardHeader>
              <CardTitle>Delivery Latency</CardTitle>
              <CardDescription>p50 and p95 latency per channel (milliseconds)</CardDescription>
            </CardHeader>
            <CardContent>
              {isLoading ? <ChartSkeleton /> : <LatencyChart reports={reports} />}
            </CardContent>
          </Card>

          {/* Provider Errors Chart */}
          <Card>
            <CardHeader>
              <CardTitle>Failures by Channel</CardTitle>
              <CardDescription>Total failed notification count per channel</CardDescription>
            </CardHeader>
            <CardContent>
              {isLoading ? <ChartSkeleton /> : <ProviderErrorsChart reports={reports} />}
            </CardContent>
          </Card>

          {/* Summary Table */}
          <Card>
            <CardHeader>
              <CardTitle>Summary Statistics</CardTitle>
              <CardDescription>Aggregated metrics per channel (latest data point)</CardDescription>
            </CardHeader>
            <CardContent>
              {isLoading ? (
                <ChartSkeleton height={160} />
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Channel</TableHead>
                      <TableHead className="text-right">Total</TableHead>
                      <TableHead className="text-right">Sent</TableHead>
                      <TableHead className="text-right">Delivered</TableHead>
                      <TableHead className="text-right">Failed</TableHead>
                      <TableHead className="text-right">Success Rate</TableHead>
                      <TableHead className="text-right">p50</TableHead>
                      <TableHead className="text-right">p95</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {summary.map((r) => (
                      <TableRow key={r.channel}>
                        <TableCell className="capitalize font-medium">{r.channel}</TableCell>
                        <TableCell className="text-right">{formatNumber(r.total)}</TableCell>
                        <TableCell className="text-right">{formatNumber(r.sent)}</TableCell>
                        <TableCell className="text-right">{formatNumber(r.delivered)}</TableCell>
                        <TableCell className="text-right text-red-600">{formatNumber(r.failed)}</TableCell>
                        <TableCell className="text-right">
                          <span className={cn(
                            'font-medium',
                            r.success_rate >= 0.95 ? 'text-green-600' :
                            r.success_rate >= 0.8  ? 'text-yellow-600' : 'text-red-600',
                          )}>
                            {formatPercent(r.success_rate)}
                          </span>
                        </TableCell>
                        <TableCell className="text-right text-muted-foreground">
                          {formatLatency(r.p50_latency_ms)}
                        </TableCell>
                        <TableCell className="text-right text-muted-foreground">
                          {formatLatency(r.p95_latency_ms)}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </>
      )}
    </div>
  )
}
