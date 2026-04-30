import { Badge } from '@/components/ui/badge'
import type { Status } from '@/types'
import { cn } from '@/lib/utils'

interface StatusBadgeProps {
  status: Status
  className?: string
}

const statusConfig: Record<Status, { label: string; variant: 'success' | 'info' | 'warning' | 'destructive' | 'muted' }> = {
  delivered: { label: 'Delivered', variant: 'success' },
  sent: { label: 'Sent', variant: 'info' },
  queued: { label: 'Queued', variant: 'warning' },
  pending: { label: 'Pending', variant: 'warning' },
  failed: { label: 'Failed', variant: 'destructive' },
  cancelled: { label: 'Cancelled', variant: 'muted' },
  bounced: { label: 'Bounced', variant: 'muted' },
  suppressed: { label: 'Suppressed', variant: 'muted' },
}

export function StatusBadge({ status, className }: StatusBadgeProps) {
  const normalized = String(status ?? '').toLowerCase() as Status
  const config = statusConfig[normalized] ?? { label: normalized || 'unknown', variant: 'muted' as const }
  return (
    <Badge variant={config.variant} className={cn(className)}>
      {config.label}
    </Badge>
  )
}
