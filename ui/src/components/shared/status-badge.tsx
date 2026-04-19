import { Badge } from '@/components/ui/badge'
import type { Status } from '@/types'
import { cn } from '@/lib/utils'

interface StatusBadgeProps {
  status: Status
  className?: string
}

const statusConfig: Record<Status, { label: string; variant: 'success' | 'info' | 'warning' | 'destructive' | 'muted' }> = {
  DELIVERED: { label: 'Delivered', variant: 'success' },
  SENT:      { label: 'Sent',      variant: 'info' },
  QUEUED:    { label: 'Queued',    variant: 'info' },
  PENDING:   { label: 'Pending',   variant: 'warning' },
  FAILED:    { label: 'Failed',    variant: 'destructive' },
  CANCELLED: { label: 'Cancelled', variant: 'muted' },
}

export function StatusBadge({ status, className }: StatusBadgeProps) {
  const config = statusConfig[status] ?? { label: status, variant: 'muted' as const }
  return (
    <Badge variant={config.variant} className={cn(className)}>
      {config.label}
    </Badge>
  )
}
