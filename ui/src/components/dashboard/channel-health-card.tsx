import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { ChannelIcon, channelLabel } from '@/components/shared/channel-icon'
import type { Channel } from '@/types'
import type { ChannelHealth } from '@/types'
import { cn } from '@/lib/utils'
import { formatPercent, formatNumber } from '@/lib/utils'

interface ChannelHealthCardProps {
  health: ChannelHealth
}

const statusConfig = {
  healthy:  { label: 'Healthy',  variant: 'success'      as const, dot: 'bg-green-500' },
  degraded: { label: 'Degraded', variant: 'warning'      as const, dot: 'bg-yellow-500' },
  down:     { label: 'Down',     variant: 'destructive'  as const, dot: 'bg-red-500' },
}

export function ChannelHealthCard({ health }: ChannelHealthCardProps) {
  const cfg = statusConfig[health.status]
  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <ChannelIcon channel={health.channel} size={18} />
            <CardTitle className="text-sm font-semibold">{channelLabel(health.channel)}</CardTitle>
          </div>
          <div className="flex items-center gap-1.5">
            <span className={cn('h-2 w-2 rounded-full', cfg.dot)} />
            <Badge variant={cfg.variant} className="text-xs">{cfg.label}</Badge>
          </div>
        </div>
      </CardHeader>
      <CardContent>
        <div className="flex justify-between text-sm">
          <span className="text-muted-foreground">Success rate</span>
          <span className="font-medium">{formatPercent(health.success_rate)}</span>
        </div>
        <div className="flex justify-between text-sm mt-1">
          <span className="text-muted-foreground">Last 24h</span>
          <span className="font-medium">{formatNumber(health.total_24h)}</span>
        </div>
        {/* progress bar */}
        <div className="mt-3 h-1.5 w-full rounded-full bg-muted">
          <div
            className={cn(
              'h-1.5 rounded-full',
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
