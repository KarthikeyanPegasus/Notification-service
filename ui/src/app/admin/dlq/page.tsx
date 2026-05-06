'use client'

import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { PageHeader } from '@/components/shared/page-header'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { CardSkeleton, KPISkeleton } from '@/components/shared/loading-skeleton'
import { ErrorState } from '@/components/shared/error-state'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  AlertTriangle,
  CheckCircle2,
  RefreshCw,
  RotateCcw,
  MessageSquare,
  Clock,
  Trash2,
} from 'lucide-react'
import { getDLQEntries, getDLQStats, replayDLQEntry, replayAllDLQ } from '@/lib/api'
import { cn } from '@/lib/utils'
import type { DLQEntry } from '@/types'

export default function DLQAdminPage() {
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)

  // ── Queries ─────────────────────────────────────────────────────────────
  const { data: statsData, isLoading: statsLoading } = useQuery({
    queryKey: ['dlq-stats'],
    queryFn: getDLQStats,
    refetchInterval: 5_000,
  })

  const { data: entriesData, isLoading: entriesLoading, isError, refetch } = useQuery({
    queryKey: ['dlq-entries', page],
    queryFn: () => getDLQEntries(page, true),
    refetchInterval: 10_000,
  })

  // ── Mutations ───────────────────────────────────────────────────────────
  const replayMutation = useMutation({
    mutationFn: replayDLQEntry,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['dlq-entries'] })
      queryClient.invalidateQueries({ queryKey: ['dlq-stats'] })
    },
  })

  const replayAllMutation = useMutation({
    mutationFn: replayAllDLQ,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['dlq-entries'] })
      queryClient.invalidateQueries({ queryKey: ['dlq-stats'] })
    },
  })

  const totalPages = entriesData ? Math.max(1, Math.ceil(entriesData.total / 50)) : 1

  if (statsLoading || entriesLoading) {
    return (
      <div className="space-y-8">
        <PageHeader title="Dead-Letter Queue" description="Failed notifications that exceeded max retries" />
        <KPISkeleton />
        <CardSkeleton />
      </div>
    )
  }

  if (isError) {
    return (
      <div className="space-y-8">
        <PageHeader title="Dead-Letter Queue" description="Failed notifications that exceeded max retries" />
        <ErrorState description="Failed to load DLQ entries." onRetry={refetch} />
      </div>
    )
  }

  const stats = statsData
  const entries = entriesData?.data ?? []

  return (
    <div className="space-y-8 animate-in fade-in duration-300">
      <PageHeader
        title="Dead-Letter Queue"
        description={
          stats
            ? `${stats.pending_replay} pending replay · ${stats.total_entries} total entries`
            : 'Failed notifications that exceeded max retries'
        }
      />

      {/* ── Stats Cards ──────────────────────────────────────────────────── */}
      <div className="grid grid-cols-3 gap-4">
        <Card>
          <CardContent className="pt-5">
            <div className="flex items-start justify-between">
              <div>
                <p className="text-xs text-muted-foreground">Total Entries</p>
                <p className="text-2xl font-semibold tabular-nums">{stats?.total_entries ?? 0}</p>
              </div>
              <MessageSquare className="h-5 w-5 text-muted-foreground" />
            </div>
          </CardContent>
        </Card>
        <Card className={stats && stats.pending_replay > 0 ? 'border-amber-400/50 bg-amber-50 dark:bg-amber-950/20' : ''}>
          <CardContent className="pt-5">
            <div className="flex items-start justify-between">
              <div>
                <p className="text-xs text-muted-foreground">Pending Replay</p>
                <p className={cn('text-2xl font-semibold tabular-nums', stats && stats.pending_replay > 0 ? 'text-amber-600' : '')}>
                  {stats?.pending_replay ?? 0}
                </p>
              </div>
              <AlertTriangle className={cn('h-5 w-5', stats && stats.pending_replay > 0 ? 'text-amber-500' : 'text-muted-foreground')} />
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-5">
            <div className="flex items-start justify-between">
              <div>
                <p className="text-xs text-muted-foreground">Replayed</p>
                <p className="text-2xl font-semibold tabular-nums text-emerald-600">{stats?.replayed_entries ?? 0}</p>
              </div>
              <CheckCircle2 className="h-5 w-5 text-emerald-500" />
            </div>
          </CardContent>
        </Card>
      </div>

      {/* ── Actions ──────────────────────────────────────────────────────── */}
      {entries.length > 0 && (
        <div className="flex items-center gap-3">
          <Button
            variant="default"
            size="sm"
            onClick={() => replayAllMutation.mutate()}
            disabled={replayAllMutation.isPending}
          >
            <RotateCcw className={cn('h-4 w-4 mr-1.5', replayAllMutation.isPending && 'animate-spin')} />
            Replay All ({entries.length} pending)
          </Button>
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            <RefreshCw className="h-4 w-4 mr-1.5" />
            Refresh
          </Button>
        </div>
      )}

      {/* ── Entries List ─────────────────────────────────────────────────── */}
      {entries.length === 0 ? (
        <Card>
          <CardContent className="py-12 text-center">
            <CheckCircle2 className="h-8 w-8 text-emerald-500 mx-auto mb-3" />
            <p className="text-sm font-medium">No pending DLQ entries</p>
            <p className="text-xs text-muted-foreground mt-1">All failed notifications have been processed or replayed.</p>
          </CardContent>
        </Card>
      ) : (
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-sm">
              <AlertTriangle className="h-4 w-4 text-amber-500" />
              Pending Entries
              <Badge variant="secondary" className="ml-auto">{entries.length}</Badge>
            </CardTitle>
            <CardDescription>Notifications that failed after {5} retry attempts. Replay to re-queue for delivery.</CardDescription>
          </CardHeader>
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Channel</TableHead>
                  <TableHead>Recipient</TableHead>
                  <TableHead>Reason</TableHead>
                  <TableHead>Attempts</TableHead>
                  <TableHead>Created</TableHead>
                  <TableHead className="w-24">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {entries.map((entry: DLQEntry) => (
                  <TableRow key={entry.id}>
                    <TableCell>
                      <Badge variant="outline" className="capitalize">{entry.channel}</Badge>
                    </TableCell>
                    <TableCell className="max-w-[160px] truncate font-mono text-xs">
                      {entry.recipient ? entry.recipient : '—'}
                    </TableCell>
                    <TableCell className="max-w-[200px] truncate text-xs text-muted-foreground">
                      {entry.reason}
                    </TableCell>
                    <TableCell className="tabular-nums text-xs">{entry.attempt_count}</TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {new Date(entry.created_at).toLocaleString()}
                    </TableCell>
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => replayMutation.mutate(entry.id)}
                        disabled={replayMutation.isPending}
                      >
                        <RotateCcw className="h-3.5 w-3.5 mr-1" />
                        Replay
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>

            {/* ── Pagination ─────────────────────────────────────────────── */}
            {totalPages > 1 && (
              <div className="flex items-center justify-between px-4 py-3 border-t">
                <p className="text-xs text-muted-foreground">
                  Page {page} of {totalPages} · {entriesData?.total ?? 0} total entries
                </p>
                <div className="flex gap-2">
                  <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage(page - 1)}>
                    Previous
                  </Button>
                  <Button variant="outline" size="sm" disabled={page >= totalPages} onClick={() => setPage(page + 1)}>
                    Next
                  </Button>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  )
}
