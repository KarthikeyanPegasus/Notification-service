import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { ChannelIcon, channelLabel } from '@/components/shared/channel-icon'
import type { ChannelHealth } from '@/types'
import { cn } from '@/lib/utils'
import { formatPercent, formatNumber } from '@/lib/utils'
import { AlertCircle, CreditCard, ShieldCheck } from 'lucide-react'

interface ChannelHealthCardProps {
  health: ChannelHealth
}

const statusConfig = {
  healthy: { label: 'Healthy', variant: 'success' as const, dot: 'bg-green-500' },
  degraded: { label: 'Degraded', variant: 'warning' as const, dot: 'bg-yellow-500' },
  down: { label: 'Down', variant: 'destructive' as const, dot: 'bg-red-500' },
}

const paymentConfig = {
  paid: { label: 'Paid', variant: 'outline' as const, icon: ShieldCheck, color: 'text-green-500' },
  unpaid: { label: 'Unpaid', variant: 'destructive' as const, icon: AlertCircle, color: 'text-red-500' },
  low_balance: { label: 'Low Balance', variant: 'warning' as const, icon: CreditCard, color: 'text-yellow-500' },
}

export function ChannelHealthCard({ health }: ChannelHealthCardProps) {
  const cfg = statusConfig[health.status]
  const payCfg = paymentConfig[health.payment_status || 'paid']

  return (
    <Card className="overflow-hidden transition-all hover:shadow-md">
      <CardHeader className="pb-3 border-b bg-muted/5">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <div className="p-1.5 rounded-lg bg-background border shadow-sm">
              <ChannelIcon channel={health.channel} size={16} />
            </div>
            <CardTitle className="text-sm font-bold tracking-tight">
              {channelLabel(health.channel)}
            </CardTitle>
          </div>
          <div className="flex items-center gap-1.5">
            <span className={cn('h-1.5 w-1.5 rounded-full animate-pulse', cfg.dot)} />
            <Badge variant={cfg.variant} className="text-[10px] uppercase font-bold py-0 h-4 px-1.5 border-none">
              {cfg.label}
            </Badge>
          </div>
        </div>
      </CardHeader>
      <CardContent className="pt-4 space-y-3">
        {/* Main Stats */}
        <div className="grid grid-cols-2 gap-2">
          <div className="space-y-0.5">
            <p className="text-[10px] uppercase font-semibold text-muted-foreground">Success</p>
            <p className="text-sm font-bold">{formatPercent(health.success_rate)}</p>
          </div>
          <div className="space-y-0.5 text-right">
            <p className="text-[10px] uppercase font-semibold text-muted-foreground">Uptime</p>
            <p className="text-sm font-bold text-green-600">{formatPercent(health.uptime)}</p>
          </div>
        </div>

        {/* Extra Metrics line */}
        <div className="flex items-center justify-between pt-1 border-t text-[11px]">
          <div className="flex items-center gap-1 text-muted-foreground">
            <payCfg.icon size={12} className={payCfg.color} />
            <span className="font-medium">{payCfg.label}</span>
          </div>
          {health.bounce_rate !== undefined && (
            <div className="flex items-center gap-1">
              <span className="text-muted-foreground">Bounce:</span>
              <span className={cn('font-bold', health.bounce_rate > 0.05 ? 'text-red-500' : 'text-green-500')}>
                {formatPercent(health.bounce_rate)}
              </span>
            </div>
          )}
        </div>

        {/* Error Status Indicator */}
        {health.status !== 'healthy' && health.error_status && (
          <div className="px-2 py-1.5 rounded bg-destructive/10 border border-destructive/20">
            <p className="text-[10px] text-destructive font-medium line-clamp-1 flex items-center gap-1">
              <AlertCircle size={10} />
              {health.error_status}
            </p>
          </div>
        )}

        {/* Activity Summary */}
        <div className="flex justify-between items-center text-[11px] text-muted-foreground pt-1">
          <span>{formatNumber(health.total_24h)} requests</span>
          <span>Last 24h</span>
        </div>

        {/* Mini sparkline-like progress */}
        <div className="h-1 w-full rounded-full bg-muted overflow-hidden">
          <div
            className={cn(
              'h-full transition-all duration-500',
              health.success_rate >= 0.95 ? 'bg-green-500' :
              health.success_rate >= 0.8  ? 'bg-yellow-500' : 'bg-red-500',
            )}
            style={{ width: `${health.success_rate * 100}%` }}
          />
        </div>
      </CardContent>
    </Card>
  )
}
