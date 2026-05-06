'use client'

import React, { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { CheckCircle2, RotateCcw, ArrowRight, Loader2, Clock } from 'lucide-react'
import { listVendorMigrations, completeVendorMigration, rollbackVendorMigration } from '@/lib/api'
import type { VendorMigration, VendorMigrationStatus } from '@/types'
import { cn } from '@/lib/utils'

// ── Helpers ──────────────────────────────────────────────────────────────────

function statusBadge(status: VendorMigrationStatus) {
  switch (status) {
    case 'in_progress':  return <Badge className="bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300 border-0">In Progress</Badge>
    case 'completed':    return <Badge className="bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-300 border-0">Completed</Badge>
    case 'rolled_back':  return <Badge className="bg-yellow-100 text-yellow-700 dark:bg-yellow-900/40 dark:text-yellow-300 border-0">Rolled Back</Badge>
    case 'failed':       return <Badge className="bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300 border-0">Failed</Badge>
  }
}

function formatRelative(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime()
  const mins = Math.floor(diff / 60_000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  const hrs = Math.floor(mins / 60)
  if (hrs < 24) return `${hrs}h ago`
  return `${Math.floor(hrs / 24)}d ago`
}

function vendorLabel(id: string): string {
  const labels: Record<string, string> = {
    ses: 'SES', smtp: 'SMTP', mailgun: 'Mailgun', sendgrid: 'SendGrid',
    postmark: 'Postmark', twilio: 'Twilio', plivo: 'Plivo', vonage: 'Vonage',
    messagebird: 'MessageBird', fcm: 'FCM', onesignal: 'OneSignal', pusher: 'Pusher',
  }
  return labels[id] ?? id
}

// ── MigrationRow ─────────────────────────────────────────────────────────────

interface MigrationRowProps {
  migration: VendorMigration
  onComplete: (id: string) => void
  onRollback: (id: string) => void
  loading: boolean
}

function MigrationRow({ migration: m, onComplete, onRollback, loading }: MigrationRowProps) {
  const isSameVendor = m.from_vendor === m.to_vendor

  return (
    <div className="py-3 space-y-2">
      <div className="flex items-center justify-between gap-2 flex-wrap">
        <div className="flex items-center gap-2 text-sm font-medium">
          <span className="text-xs uppercase text-muted-foreground font-semibold">{m.channel}</span>
          <span>{vendorLabel(m.from_vendor)}</span>
          {isSameVendor
            ? <span className="text-xs text-muted-foreground">(config swap)</span>
            : <><ArrowRight className="h-3.5 w-3.5 text-muted-foreground" /><span>{vendorLabel(m.to_vendor)}</span></>
          }
        </div>
        <div className="flex items-center gap-2">
          {statusBadge(m.status)}
          <Badge variant="outline" className="text-xs capitalize">{m.strategy}</Badge>
        </div>
      </div>

      <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <Clock className="h-3 w-3" />
        <span>Started {formatRelative(m.started_at)}</span>
        {m.completed_at && <span>· Ended {formatRelative(m.completed_at)}</span>}
      </div>

      {m.error_message && (
        <p className="text-xs text-destructive">{m.error_message}</p>
      )}

      {m.status === 'in_progress' && (
        <div className="flex gap-2 mt-1">
          {m.strategy === 'gradual' && (
            <Dialog>
              <DialogTrigger asChild>
                <Button size="sm" variant="default" disabled={loading}>
                  {loading ? <Loader2 className="mr-1 h-3 w-3 animate-spin" /> : <CheckCircle2 className="mr-1 h-3 w-3" />}
                  Complete Migration
                </Button>
              </DialogTrigger>
              <DialogContent className="sm:max-w-[420px]">
                <DialogHeader>
                  <DialogTitle>Complete migration?</DialogTitle>
                  <DialogDescription>
                    Routing will be locked to <strong>{vendorLabel(m.to_vendor)}</strong> only.
                    The old vendor fallback will be removed. Use Rollback if you need to revert.
                  </DialogDescription>
                </DialogHeader>
                <DialogFooter>
                  <DialogTrigger asChild>
                    <Button variant="outline">Cancel</Button>
                  </DialogTrigger>
                  <DialogTrigger asChild>
                    <Button onClick={() => onComplete(m.id)}>Complete</Button>
                  </DialogTrigger>
                </DialogFooter>
              </DialogContent>
            </Dialog>
          )}

          <Dialog>
            <DialogTrigger asChild>
              <Button size="sm" variant="outline" disabled={loading}>
                {loading ? <Loader2 className="mr-1 h-3 w-3 animate-spin" /> : <RotateCcw className="mr-1 h-3 w-3" />}
                Rollback
              </Button>
            </DialogTrigger>
            <DialogContent className="sm:max-w-[420px]">
              <DialogHeader>
                <DialogTitle>Rollback migration?</DialogTitle>
                <DialogDescription>
                  The previous <strong>{vendorLabel(m.from_vendor)}</strong> credentials will be
                  restored and routing will revert to the old vendor.
                  Messages already sent via <strong>{vendorLabel(m.to_vendor)}</strong> are unaffected.
                </DialogDescription>
              </DialogHeader>
              <DialogFooter>
                <DialogTrigger asChild>
                  <Button variant="outline">Cancel</Button>
                </DialogTrigger>
                <DialogTrigger asChild>
                  <Button
                    variant="destructive"
                    onClick={() => onRollback(m.id)}
                  >
                    Rollback
                  </Button>
                </DialogTrigger>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </div>
      )}
    </div>
  )
}

// ── VendorMigrationStatus ─────────────────────────────────────────────────────

interface Props {
  apiKeyId?: string
  channel?: 'email' | 'sms' | 'push'
}

export function VendorMigrationStatus({ apiKeyId, channel }: Props) {
  const queryClient = useQueryClient()
  const [actioningId, setActioningId] = useState<string | null>(null)

  const { data: migrations = [], isLoading } = useQuery({
    queryKey: ['vendor-migrations', apiKeyId ?? 'global', channel ?? 'all'],
    queryFn: () => listVendorMigrations({ apiKeyId, channel }),
    refetchInterval: 10_000, // poll every 10s while the panel is open
  })

  const completeMutation = useMutation({
    mutationFn: (id: string) => completeVendorMigration(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['vendor-migrations'] })
      queryClient.invalidateQueries({ queryKey: ['vendor-configs'] })
      setActioningId(null)
    },
    onSettled: () => setActioningId(null),
  })

  const rollbackMutation = useMutation({
    mutationFn: (id: string) => rollbackVendorMigration(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['vendor-migrations'] })
      queryClient.invalidateQueries({ queryKey: ['vendor-configs'] })
      setActioningId(null)
    },
    onSettled: () => setActioningId(null),
  })

  // Only show migrations from the last 7 days.
  const cutoff = Date.now() - 7 * 24 * 60 * 60 * 1000
  const visible = migrations.filter((m) => new Date(m.created_at).getTime() > cutoff)

  const active   = visible.filter((m) => m.status === 'in_progress')
  const historic = visible.filter((m) => m.status !== 'in_progress')

  if (!isLoading && visible.length === 0) return null

  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <CardTitle className="text-sm font-semibold">Vendor Migrations</CardTitle>
          {active.length > 0 && (
            <Badge className="bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300 border-0 text-xs">
              {active.length} active
            </Badge>
          )}
        </div>
        <CardDescription className="text-xs">
          Recent vendor swaps and credential changes (last 7 days).
        </CardDescription>
      </CardHeader>

      <CardContent className="pt-0 space-y-0 divide-y divide-border">
        {isLoading && (
          <div className="py-4 flex justify-center">
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
          </div>
        )}

        {active.map((m) => (
          <MigrationRow
            key={m.id}
            migration={m}
            loading={actioningId === m.id && (completeMutation.isPending || rollbackMutation.isPending)}
            onComplete={(id) => { setActioningId(id); completeMutation.mutate(id) }}
            onRollback={(id) => { setActioningId(id); rollbackMutation.mutate(id) }}
          />
        ))}

        {historic.length > 0 && active.length > 0 && (
          <Separator className="my-1" />
        )}

        {historic.map((m) => (
          <MigrationRow
            key={m.id}
            migration={m}
            loading={false}
            onComplete={() => {}}
            onRollback={() => {}}
          />
        ))}
      </CardContent>
    </Card>
  )
}
