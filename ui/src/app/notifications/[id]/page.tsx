'use client'

import { useQuery } from '@tanstack/react-query'
import Link from 'next/link'
import { getNotification } from '@/lib/api'
import { PageHeader } from '@/components/shared/page-header'
import { StatusBadge } from '@/components/shared/status-badge'
import { ChannelIcon, channelLabel } from '@/components/shared/channel-icon'
import { Timeline } from '@/components/notifications/timeline'
import { AttemptCard } from '@/components/notifications/attempt-card'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { CardSkeleton } from '@/components/shared/loading-skeleton'
import { ErrorState } from '@/components/shared/error-state'
import { formatDate, truncateId } from '@/lib/utils'
import { ArrowLeft, Copy, RefreshCw } from 'lucide-react'
import { toast } from 'sonner'
import { useState } from 'react'
import { syncNotification } from '@/lib/api'

interface Props {
  params: { id: string }
}

const priorityVariants: Record<string, 'default' | 'secondary' | 'warning' | 'destructive'> = {
  low: 'secondary',
  normal: 'default',
  high: 'warning',
  critical: 'destructive',
}

function DetailRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex flex-col sm:flex-row sm:items-start gap-1 sm:gap-4 py-3 border-b last:border-0">
      <span className="text-sm font-medium text-muted-foreground sm:w-40 shrink-0">{label}</span>
      <span className="text-sm text-foreground flex-1 break-all">{value}</span>
    </div>
  )
}

export default function NotificationDetailPage({ params }: Props) {
  const { id } = params

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ['notification', id],
    queryFn: () => getNotification(id),
    retry: 1,
  })

  const [isSyncing, setIsSyncing] = useState(false)

  const handleSync = async () => {
    setIsSyncing(true)
    try {
      const res = await syncNotification(id)
      toast.success(`Synced with vendor: ${res.vendor_status}`)
      refetch()
    } catch (err: any) {
      toast.error(`Sync failed: ${err.message}`)
    } finally {
      setIsSyncing(false)
    }
  }

  // Work with nested notification object from backend response
  const notification = data?.notification ?? null
  
  // Extract flattened fields if available
  const subject = (data as any)?.subject || notification?.subject
  const body = (data as any)?.body || notification?.rendered_content?.body || notification?.body

  if (isLoading) {
    return (
      <div className="space-y-6">
        <CardSkeleton />
        <CardSkeleton />
        <CardSkeleton />
      </div>
    )
  }

  if (!notification) {
    return <ErrorState title="Notification not found" onRetry={() => refetch()} />
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Notification Detail"
        breadcrumbs={[
          { label: 'Dashboard', href: '/dashboard' },
          { label: 'Notifications', href: '/notifications' },
          { label: truncateId(notification.id, 12) },
        ]}
        actions={
          <div className="flex items-center gap-2">
            <Button 
              variant="outline" 
              size="sm" 
              onClick={handleSync} 
              disabled={isSyncing || notification.status === 'failed' || !notification.attempts?.length}
            >
              <RefreshCw className={`mr-2 h-4 w-4 ${isSyncing ? 'animate-spin' : ''}`} />
              Sync from Vendor
            </Button>
            <Button variant="outline" size="sm" asChild>
              <Link href="/notifications">
                <ArrowLeft className="mr-2 h-4 w-4" />
                Back
              </Link>
            </Button>
          </div>
        }
      />

      {/* Status bar */}
      <div className="flex flex-wrap items-center gap-3 rounded-lg border bg-card p-4">
        <StatusBadge status={notification.status} />
        <div className="flex items-center gap-2">
          <ChannelIcon channel={notification.channel} size={18} />
          <span className="text-sm font-medium">{channelLabel(notification.channel)}</span>
        </div>
        <Badge variant={priorityVariants[notification.priority] ?? 'default'} className="capitalize">
          {notification.priority} priority
        </Badge>
        <span className="ml-auto font-mono text-xs text-muted-foreground flex items-center gap-1">
          {notification.id}
          <button
            onClick={() => navigator.clipboard.writeText(notification.id)}
            className="p-1 rounded hover:bg-accent"
            title="Copy ID"
          >
            <Copy className="h-3 w-3" />
          </button>
        </span>
      </div>

      {/* Details */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Notification Details</CardTitle>
        </CardHeader>
        <CardContent>
          <DetailRow label="ID" value={<code className="text-xs">{notification.id}</code>} />
          <DetailRow label="User ID" value={notification.user_id} />
          <DetailRow label="Channel" value={
            <div className="flex items-center gap-2">
              <ChannelIcon channel={notification.channel} size={14} />
              {channelLabel(notification.channel)}
            </div>
          } />
          {subject && <DetailRow label="Subject" value={subject} />}
          <DetailRow label="Body" value={
            <pre className="text-xs bg-muted p-2 rounded-md whitespace-pre-wrap max-h-32 overflow-auto">
              {body}
            </pre>
          } />
          {notification.provider && <DetailRow label="Provider" value={notification.provider} />}
          {notification.idempotency_key && (
            <DetailRow label="Idempotency Key" value={
              <code className="text-xs">{notification.idempotency_key}</code>
            } />
          )}
          {notification.scheduled_at && (
            <DetailRow label="Scheduled At" value={formatDate(notification.scheduled_at)} />
          )}
          {notification.sent_at && (
            <DetailRow label="Sent At" value={formatDate(notification.sent_at)} />
          )}
          <DetailRow label="Created At" value={formatDate(notification.created_at)} />
          <DetailRow label="Updated At" value={formatDate(notification.updated_at)} />
        </CardContent>
      </Card>

      {/* Event Timeline */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Event Timeline</CardTitle>
        </CardHeader>
        <CardContent>
          <Timeline events={data?.events || notification.events || []} />
        </CardContent>
      </Card>

      {/* Delivery Attempts */}
      {(data?.attempts?.length || notification?.attempts?.length || 0) > 0 && (
        <div className="space-y-3">
          <h2 className="text-base font-semibold">
            Delivery Attempts ({(data?.attempts || notification.attempts || []).length})
          </h2>
          {(data?.attempts || notification.attempts || []).map((attempt: any, idx: number) => (
            <AttemptCard key={attempt.id} attempt={attempt} index={idx} />
          ))}
        </div>
      )}
    </div>
  )
}
