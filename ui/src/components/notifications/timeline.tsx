import type { Event } from '@/types'
import { formatDate } from '@/lib/utils'
import {
  Clock,
  Send,
  CheckCircle2,
  XCircle,
  RotateCcw,
  Layers,
  CircleDot,
} from 'lucide-react'
import { cn } from '@/lib/utils'

interface TimelineProps {
  events: Event[]
}

const eventConfig: Record<string, { icon: React.ElementType; color: string; label: string }> = {
  QUEUED:    { icon: Layers,        color: 'text-blue-500 bg-blue-50 border-blue-200',   label: 'Queued' },
  SENT:      { icon: Send,          color: 'text-blue-600 bg-blue-50 border-blue-200',   label: 'Sent' },
  DELIVERED: { icon: CheckCircle2,  color: 'text-green-600 bg-green-50 border-green-200', label: 'Delivered' },
  FAILED:    { icon: XCircle,       color: 'text-red-500 bg-red-50 border-red-200',      label: 'Failed' },
  RETRY:     { icon: RotateCcw,     color: 'text-yellow-600 bg-yellow-50 border-yellow-200', label: 'Retried' },
  CANCELLED: { icon: XCircle,       color: 'text-slate-500 bg-slate-50 border-slate-200', label: 'Cancelled' },
}

function getEventConfig(eventType: string) {
  return eventConfig[eventType] ?? {
    icon: CircleDot,
    color: 'text-slate-500 bg-slate-50 border-slate-200',
    label: eventType.charAt(0) + eventType.slice(1).toLowerCase().replace(/_/g, ' '),
  }
}

export function Timeline({ events }: TimelineProps) {
  const sorted = [...events].sort(
    (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
  )

  return (
    <div className="relative">
      {/* Vertical line */}
      <div className="absolute left-5 top-5 bottom-5 w-px bg-border" />

      <ol className="space-y-6">
        {sorted.map((event, idx) => {
          const cfg = getEventConfig(event.event_type)
          const Icon = cfg.icon
          return (
            <li key={event.id} className="relative flex gap-4">
              {/* Icon bubble */}
              <div
                className={cn(
                  'relative z-10 flex h-10 w-10 shrink-0 items-center justify-center rounded-full border-2',
                  cfg.color,
                )}
              >
                <Icon className="h-4 w-4" />
              </div>

              {/* Content */}
              <div className="flex-1 pt-1.5">
                <div className="flex items-center justify-between gap-2">
                  <p className="text-sm font-semibold">{cfg.label}</p>
                  <time className="text-xs text-muted-foreground">{formatDate(event.created_at)}</time>
                </div>
                {event.metadata && Object.keys(event.metadata).length > 0 && (
                  <pre className="mt-2 text-xs rounded-md bg-muted p-2 overflow-auto max-h-32">
                    {JSON.stringify(event.metadata, null, 2)}
                  </pre>
                )}
              </div>
            </li>
          )
        })}
      </ol>

      {sorted.length === 0 && (
        <p className="text-sm text-muted-foreground py-4 pl-14">No events recorded yet.</p>
      )}
    </div>
  )
}
