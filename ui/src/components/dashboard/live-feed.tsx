'use client'

import { useQuery } from '@tanstack/react-query'
import { getNotifications } from '@/lib/api'
import { StatusBadge } from '@/components/shared/status-badge'
import { ChannelIcon } from '@/components/shared/channel-icon'
import { formatRelativeTime, truncateId } from '@/lib/utils'
import { TableSkeleton } from '@/components/shared/loading-skeleton'
import { ErrorState } from '@/components/shared/error-state'
import Link from 'next/link'
import { RefreshCw } from 'lucide-react'

export function LiveFeed() {
  const { data, isLoading, isError, refetch, dataUpdatedAt } = useQuery({
    queryKey: ['dashboard-feed'],
    queryFn: () => getNotifications({ page: 1, page_size: 10 }),
    refetchInterval: 30000,
    retry: 1,
  })

  const notifications = data?.data ?? []
  const lastUpdated = dataUpdatedAt ? new Date(dataUpdatedAt).toLocaleTimeString() : null

  return (
    <div>
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
          Recent Activity
        </h3>
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          {lastUpdated && <span>Updated {lastUpdated}</span>}
          <button
            onClick={() => refetch()}
            className="p-1 rounded hover:bg-accent transition-colors"
            title="Refresh"
          >
            <RefreshCw className="h-3 w-3" />
          </button>
        </div>
      </div>

      {isLoading && <TableSkeleton rows={5} cols={4} />}
      {!isLoading && (
        <div className="divide-y">
          {notifications.map((n) => (
            <Link
              key={n.id}
              href={`/notifications/${n.id}`}
              className="flex items-center justify-between py-3 hover:bg-muted/40 px-2 rounded-md transition-colors group"
            >
              <div className="flex items-center gap-3 min-w-0">
                <ChannelIcon channel={n.channel} />
                <div className="min-w-0">
                  <p className="text-sm font-medium truncate group-hover:text-primary">
                    {n.subject ?? truncateId(n.body, 40)}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {truncateId(n.id)} · {n.user_id}
                  </p>
                </div>
              </div>
              <div className="flex items-center gap-3 shrink-0 ml-2">
                <StatusBadge status={n.status} />
                <span className="text-xs text-muted-foreground hidden sm:block">
                  {formatRelativeTime(n.created_at)}
                </span>
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}
