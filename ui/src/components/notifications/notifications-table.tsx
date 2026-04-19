'use client'

import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useRouter } from 'next/navigation'
import {
  useReactTable,
  getCoreRowModel,
  flexRender,
  type ColumnDef,
} from '@tanstack/react-table'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { StatusBadge } from '@/components/shared/status-badge'
import { ChannelIcon, channelLabel } from '@/components/shared/channel-icon'
import { NotificationFiltersBar } from './notification-filters'
import { TableSkeleton } from '@/components/shared/loading-skeleton'
import { EmptyState } from '@/components/shared/empty-state'
import { ErrorState } from '@/components/shared/error-state'
import { getNotifications } from '@/lib/api'
import type { Notification, NotificationFilters } from '@/types'
import { formatDate, truncateId } from '@/lib/utils'
import { ChevronLeft, ChevronRight, ExternalLink, Bell } from 'lucide-react'

const priorityVariants: Record<string, 'default' | 'secondary' | 'warning' | 'destructive'> = {
  low:      'secondary',
  normal:   'default',
  high:     'warning',
  critical: 'destructive',
}

export function NotificationsTable() {
  const router = useRouter()
  const [filters, setFilters] = useState<NotificationFilters>({
    page: 1,
    page_size: 20,
  })

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ['notifications', filters],
    queryFn: () => getNotifications(filters),
    retry: 1,
  })

  const notifications = data?.data ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / (filters.page_size ?? 20)))
  const currentPage = filters.page ?? 1

  const columns = useMemo<ColumnDef<Notification>[]>(
    () => [
      {
        accessorKey: 'id',
        header: 'ID',
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">
            {truncateId(row.original.id, 12)}
          </span>
        ),
      },
      {
        accessorKey: 'user_id',
        header: 'User',
        cell: ({ row }) => (
          <span className="text-sm font-medium">{truncateId(row.original.user_id, 10)}</span>
        ),
      },
      {
        accessorKey: 'channel',
        header: 'Channel',
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <ChannelIcon channel={row.original.channel} />
            <span className="text-sm">{channelLabel(row.original.channel)}</span>
          </div>
        ),
      },
      {
        accessorKey: 'status',
        header: 'Status',
        cell: ({ row }) => <StatusBadge status={row.original.status} />,
      },
      {
        accessorKey: 'priority',
        header: 'Priority',
        cell: ({ row }) => (
          <Badge variant={priorityVariants[row.original.priority] ?? 'default'} className="capitalize">
            {row.original.priority}
          </Badge>
        ),
      },
      {
        accessorKey: 'provider',
        header: 'Provider',
        cell: ({ row }) => (
          <span className="text-sm text-muted-foreground">{row.original.provider ?? '—'}</span>
        ),
      },
      {
        accessorKey: 'created_at',
        header: 'Created',
        cell: ({ row }) => (
          <span className="text-sm text-muted-foreground">{formatDate(row.original.created_at)}</span>
        ),
      },
      {
        id: 'actions',
        header: '',
        cell: ({ row }) => (
          <Button
            variant="ghost"
            size="icon"
            onClick={(e) => {
              e.stopPropagation()
              router.push(`/notifications/${row.original.id}`)
            }}
            title="View details"
          >
            <ExternalLink className="h-3.5 w-3.5" />
          </Button>
        ),
      },
    ],
    [router],
  )

  const table = useReactTable({
    data: notifications,
    columns,
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
    pageCount: totalPages,
  })

  return (
    <div className="space-y-4">
      <NotificationFiltersBar filters={filters} onChange={setFilters} />

      <div className="rounded-md border bg-card">
        {isLoading ? (
          <div className="p-6">
            <TableSkeleton rows={8} cols={8} />
          </div>
        ) : isError && !notifications.length ? (
          <ErrorState onRetry={() => refetch()} />
        ) : notifications.length === 0 ? (
          <EmptyState
            icon={Bell}
            title="No notifications found"
            description="Try adjusting your filters or wait for new notifications to arrive."
          />
        ) : (
          <>
            <Table>
              <TableHeader>
                {table.getHeaderGroups().map((headerGroup) => (
                  <TableRow key={headerGroup.id}>
                    {headerGroup.headers.map((header) => (
                      <TableHead key={header.id}>
                        {header.isPlaceholder
                          ? null
                          : flexRender(header.column.columnDef.header, header.getContext())}
                      </TableHead>
                    ))}
                  </TableRow>
                ))}
              </TableHeader>
              <TableBody>
                {table.getRowModel().rows.map((row) => (
                  <TableRow
                    key={row.id}
                    className="cursor-pointer"
                    onClick={() => router.push(`/notifications/${row.original.id}`)}
                  >
                    {row.getVisibleCells().map((cell) => (
                      <TableCell key={cell.id}>
                        {flexRender(cell.column.columnDef.cell, cell.getContext())}
                      </TableCell>
                    ))}
                  </TableRow>
                ))}
              </TableBody>
            </Table>

            {/* Pagination */}
            <div className="flex items-center justify-between px-4 py-3 border-t">
              <p className="text-sm text-muted-foreground">
                Showing {((currentPage - 1) * (filters.page_size ?? 20)) + 1}–
                {Math.min(currentPage * (filters.page_size ?? 20), total)} of {total}
              </p>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="icon"
                  disabled={currentPage <= 1}
                  onClick={() => setFilters((f) => ({ ...f, page: (f.page ?? 1) - 1 }))}
                >
                  <ChevronLeft className="h-4 w-4" />
                </Button>
                <span className="text-sm">
                  {currentPage} / {totalPages}
                </span>
                <Button
                  variant="outline"
                  size="icon"
                  disabled={currentPage >= totalPages}
                  onClick={() => setFilters((f) => ({ ...f, page: (f.page ?? 1) + 1 }))}
                >
                  <ChevronRight className="h-4 w-4" />
                </Button>
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
