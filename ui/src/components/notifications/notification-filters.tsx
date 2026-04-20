'use client'

import { useState, useCallback } from 'react'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Search, X } from 'lucide-react'
import type { Channel, Status, NotificationFilters } from '@/types'
import { ChannelIcon, channelLabel } from '@/components/shared/channel-icon'
import { cn } from '@/lib/utils'

const CHANNELS: Channel[] = ['email', 'sms', 'push', 'websocket', 'webhook', 'slack']
const STATUSES: Status[] = ['PENDING', 'QUEUED', 'SENT', 'DELIVERED', 'FAILED', 'CANCELLED']

const statusColors: Record<Status, string> = {
  PENDING:   'bg-yellow-100 text-yellow-800 border-yellow-200',
  QUEUED:    'bg-blue-100 text-blue-800 border-blue-200',
  SENT:      'bg-blue-100 text-blue-800 border-blue-200',
  DELIVERED: 'bg-green-100 text-green-800 border-green-200',
  FAILED:    'bg-red-100 text-red-800 border-red-200',
  CANCELLED: 'bg-slate-100 text-slate-600 border-slate-200',
}

interface NotificationFiltersProps {
  filters: NotificationFilters
  onChange: (filters: NotificationFilters) => void
}

export function NotificationFiltersBar({ filters, onChange }: NotificationFiltersProps) {
  const [search, setSearch] = useState(filters.search ?? '')

  const handleSearchCommit = useCallback(() => {
    onChange({ ...filters, search, page: 1 })
  }, [search, filters, onChange])

  const toggleChannel = (channel: Channel) => {
    const current = filters.channel ?? []
    const next = current.includes(channel)
      ? current.filter((c) => c !== channel)
      : [...current, channel]
    onChange({ ...filters, channel: next.length ? next : undefined, page: 1 })
  }

  const toggleStatus = (status: Status) => {
    const current = filters.status ?? []
    const next = current.includes(status)
      ? current.filter((s) => s !== status)
      : [...current, status]
    onChange({ ...filters, status: next.length ? next : undefined, page: 1 })
  }

  const clearAll = () => {
    setSearch('')
    onChange({ page: 1, page_size: filters.page_size })
  }

  const hasFilters =
    !!filters.search ||
    (filters.channel?.length ?? 0) > 0 ||
    (filters.status?.length ?? 0) > 0

  return (
    <div className="space-y-3">
      {/* Search bar */}
      <div className="flex gap-2">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <Input
            placeholder="Search by notification ID or user ID..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleSearchCommit()}
            className="pl-9"
          />
        </div>
        <Button onClick={handleSearchCommit} variant="outline">
          Search
        </Button>
        {hasFilters && (
          <Button onClick={clearAll} variant="ghost" size="icon" title="Clear filters">
            <X className="h-4 w-4" />
          </Button>
        )}
      </div>

      {/* Channel filter */}
      <div className="flex flex-wrap gap-2 items-center">
        <span className="text-xs text-muted-foreground font-medium w-16">Channel:</span>
        {CHANNELS.map((ch) => {
          const active = filters.channel?.includes(ch)
          return (
            <button
              key={ch}
              onClick={() => toggleChannel(ch)}
              className={cn(
                'inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-medium transition-colors',
                active
                  ? 'bg-primary text-primary-foreground border-primary'
                  : 'bg-background hover:bg-accent border-border text-foreground',
              )}
            >
              <ChannelIcon channel={ch} size={12} className={active ? 'text-primary-foreground' : undefined} />
              {channelLabel(ch)}
            </button>
          )
        })}
      </div>

      {/* Status filter */}
      <div className="flex flex-wrap gap-2 items-center">
        <span className="text-xs text-muted-foreground font-medium w-16">Status:</span>
        {STATUSES.map((s) => {
          const active = filters.status?.includes(s)
          return (
            <button
              key={s}
              onClick={() => toggleStatus(s)}
              className={cn(
                'inline-flex items-center rounded-full border px-2.5 py-1 text-xs font-medium transition-all',
                active ? `${statusColors[s]} ring-1 ring-offset-1 ring-current` : 'bg-background hover:bg-accent border-border',
              )}
            >
              {s}
            </button>
          )
        })}
      </div>

      {/* Active filter summary */}
      {hasFilters && (
        <div className="flex flex-wrap gap-1 text-xs text-muted-foreground">
          <span>Active:</span>
          {filters.search && <Badge variant="outline">search: {filters.search}</Badge>}
          {filters.channel?.map((c) => <Badge key={c} variant="outline">{c}</Badge>)}
          {filters.status?.map((s) => <Badge key={s} variant="outline">{s}</Badge>)}
        </div>
      )}
    </div>
  )
}
