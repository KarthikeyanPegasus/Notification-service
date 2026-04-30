import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { formatPercent } from '@/lib/utils'
import {
  Activity, Wifi, ShieldCheck, AlertCircle, Clock,
  DollarSign, ExternalLink, TrendingDown, MailWarning,
} from 'lucide-react'
import type { VendorMetric } from '@/types'
import { VENDORS } from '@/lib/vendors'

interface VendorStatusCardProps {
  metric: VendorMetric
}

const statusConfig = {
  healthy:  { label: 'Online',   variant: 'success'     as const, bg: 'bg-green-500/10',  text: 'text-green-500',  icon: ShieldCheck },
  degraded: { label: 'Degraded', variant: 'warning'     as const, bg: 'bg-yellow-500/10', text: 'text-yellow-500', icon: Activity },
  down:     { label: 'Offline',  variant: 'destructive' as const, bg: 'bg-red-500/10',    text: 'text-red-500',    icon: AlertCircle },
}

function fmtCost(usd: number) {
  if (usd === 0) return 'Free'
  if (usd < 0.01) return `$${(usd * 100).toFixed(3)}¢`
  return `$${usd.toFixed(4)}`
}

function fmtSpend(usd: number) {
  if (usd === 0) return '$0.00'
  if (usd < 0.001) return '<$0.001'
  return usd >= 1 ? `$${usd.toFixed(2)}` : `$${usd.toFixed(4)}`
}

// Traffic-light colours matching ISP sender thresholds
// Bounce: <2% green, 2–5% yellow, >5% red  (per RFC/industry consensus)
// Complaint: <0.05% green, 0.05–0.1% yellow, >0.1% red  (Google/Yahoo 2024 thresholds)
function bounceColor(r: number) {
  if (r === 0) return 'text-muted-foreground/60'
  if (r > 0.05) return 'text-red-500'
  if (r > 0.02) return 'text-yellow-500'
  return 'text-emerald-500'
}
function complaintColor(r: number) {
  if (r === 0) return 'text-muted-foreground/60'
  if (r > 0.001) return 'text-red-500'
  if (r > 0.0005) return 'text-yellow-500'
  return 'text-emerald-500'
}

export function VendorStatusCard({ metric }: VendorStatusCardProps) {
  const isIdle      = metric.total === 0
  const lastFailed  = String(metric.last_status ?? '').toLowerCase() === 'failed'

  const status =
    lastFailed            ? 'down'
    : isIdle              ? 'healthy'
    : metric.success_rate >= 0.98 ? 'healthy'
    : metric.success_rate >= 0.85 ? 'degraded'
    : 'down'
  const cfg = statusConfig[status]

  const hasCost      = metric.cost_per_msg > 0
  const bounceRate   = metric.bounce_rate    ?? 0
  const complaintRate= metric.complaint_rate ?? 0
  const allTimeSends = metric.all_time_total ?? 0

  // Has any historical data to show rates against
  const hasHistory = allTimeSends > 0

  const vendorDef   = VENDORS.find((v) => v.id === metric.provider)
  const dashboardUrl= vendorDef?.dashboardUrl

  const inner = (
    <Card className={cn(
      'border-none shadow-sm bg-muted/30 transition-colors',
      dashboardUrl ? 'hover:bg-muted/50 cursor-pointer group' : 'hover:bg-muted/50',
    )}>
      {/* ── Header ── */}
      <CardHeader className="pb-2 pt-4 px-4 flex flex-row items-center justify-between space-y-0">
        <div className="flex items-center gap-2">
          <div className={cn('p-2 rounded-full', cfg.bg, cfg.text)}>
            <cfg.icon size={16} />
          </div>
          <CardTitle className="text-xs font-bold uppercase tracking-wider text-muted-foreground">
            {metric.provider}
          </CardTitle>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-[10px] font-bold text-muted-foreground">PING:</span>
          <span className={cn(
            'text-[10px] font-mono font-bold',
            metric.avg_latency_ms > 500 ? 'text-yellow-500' : 'text-green-500',
          )}>
            {Math.round(metric.avg_latency_ms)}ms
          </span>
          {dashboardUrl && (
            <ExternalLink
              size={12}
              className="text-muted-foreground/40 group-hover:text-muted-foreground transition-colors shrink-0"
            />
          )}
        </div>
      </CardHeader>

      <CardContent className="px-4 pb-4 space-y-3">
        {/* ── Success rate + status ── */}
        <div className="flex items-end justify-between">
          <div className="space-y-0.5">
            <div className="flex items-center gap-2">
              <span className="text-2xl font-black tracking-tighter tabular-nums">
                {formatPercent(metric.success_rate)}
              </span>
              <Badge variant={cfg.variant} className="text-[9px] h-4 px-1.5 leading-none">
                {cfg.label}
              </Badge>
            </div>
            <p className="text-[10px] text-muted-foreground font-medium flex items-center gap-1">
              <Wifi size={10} className="text-green-500" />
              Direct Connectivity Active
            </p>
          </div>
          <div className="text-right space-y-0.5">
            <div className="flex items-center justify-end gap-1">
              <DollarSign size={10} className={hasCost ? 'text-amber-500' : 'text-muted-foreground'} />
              <span className="text-[10px] font-bold text-muted-foreground uppercase">Per msg</span>
            </div>
            <p className={cn('text-sm font-black tabular-nums', hasCost ? 'text-amber-500' : 'text-emerald-500')}>
              {fmtCost(metric.cost_per_msg)}
            </p>
          </div>
        </div>

        {/* ── Bounce & Complaint rates (always shown) ── */}
        <div className="grid grid-cols-2 gap-2">
          {/* Bounce */}
          <div className="rounded-md bg-muted/40 border border-border/50 px-2.5 py-2">
            <div className="flex items-center gap-1 mb-1">
              <TrendingDown size={9} className="text-muted-foreground/60" />
              <span className="text-[9px] font-bold uppercase tracking-wide text-muted-foreground/70">Bounce</span>
            </div>
            <p className={cn('text-base font-black tabular-nums leading-none', bounceColor(bounceRate))}>
              {hasHistory ? formatPercent(bounceRate) : '—'}
            </p>
            <p className="text-[9px] text-muted-foreground/50 mt-0.5">
              {hasHistory
                ? metric.bounce_count > 0
                  ? `${metric.bounce_count} / ${allTimeSends.toLocaleString()}`
                  : `0 / ${allTimeSends.toLocaleString()}`
                : 'no data yet'}
            </p>
          </div>

          {/* Complaint */}
          <div className="rounded-md bg-muted/40 border border-border/50 px-2.5 py-2">
            <div className="flex items-center gap-1 mb-1">
              <MailWarning size={9} className="text-muted-foreground/60" />
              <span className="text-[9px] font-bold uppercase tracking-wide text-muted-foreground/70">Complaint</span>
            </div>
            <p className={cn('text-base font-black tabular-nums leading-none', complaintColor(complaintRate))}>
              {hasHistory ? formatPercent(complaintRate) : '—'}
            </p>
            <p className="text-[9px] text-muted-foreground/50 mt-0.5">
              {hasHistory
                ? metric.complaint_count > 0
                  ? `${metric.complaint_count} / ${allTimeSends.toLocaleString()}`
                  : `0 / ${allTimeSends.toLocaleString()}`
                : 'no data yet'}
            </p>
          </div>
        </div>

        {/* ── Estimated spend ── */}
        {hasCost && metric.total > 0 && (
          <div className="flex items-center justify-between rounded-md bg-amber-500/5 border border-amber-500/20 px-2 py-1.5">
            <span className="text-[10px] font-bold text-amber-600 uppercase tracking-wide">Est. spend (12h)</span>
            <span className="text-[11px] font-black tabular-nums text-amber-600">{fmtSpend(metric.estimated_cost)}</span>
          </div>
        )}
        {!hasCost && metric.total > 0 && (
          <div className="flex items-center justify-between rounded-md bg-emerald-500/5 border border-emerald-500/20 px-2 py-1.5">
            <span className="text-[10px] font-bold text-emerald-600 uppercase tracking-wide">Est. spend (12h)</span>
            <span className="text-[11px] font-black tabular-nums text-emerald-600">$0.00</span>
          </div>
        )}

        {/* ── Error banner ── */}
        {status !== 'healthy' && metric.latest_error && (
          <div className="p-2 rounded border border-destructive/20 bg-destructive/5">
            <p className="text-[10px] text-destructive leading-tight italic truncate">
              ERR: {metric.latest_error}
            </p>
          </div>
        )}

        {/* ── Footer ── */}
        <div className="flex items-center gap-4 text-[10px] tabular-nums font-bold text-muted-foreground/60 border-t pt-2">
          <span className="flex items-center gap-1"><Clock size={10} /> 12H WINDOW</span>
          <span className="flex items-center gap-1"><Activity size={10} /> {metric.total} REQS</span>
          {hasHistory && (
            <span className="ml-auto text-[9px] text-muted-foreground/40">rates: all-time</span>
          )}
        </div>
      </CardContent>
    </Card>
  )

  if (dashboardUrl) {
    return (
      <a
        href={dashboardUrl}
        target="_blank"
        rel="noopener noreferrer"
        className="block focus:outline-none focus-visible:ring-2 focus-visible:ring-ring rounded-xl"
        title={`Open ${vendorDef?.name ?? metric.provider} dashboard`}
      >
        {inner}
      </a>
    )
  }
  return inner
}
