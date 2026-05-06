'use client'

import { useState, useEffect } from 'react'
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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Trash2, Plus } from 'lucide-react'
import { formatDistanceToNow } from 'date-fns'
import { toast } from 'sonner'
import { truncateId } from '@/lib/utils'
import { getOptOuts, deleteOptOut, createOptOut } from '@/lib/api'

interface OptOut {
  id: string
  user_id: string
  channel: string
  reason: string
  source: string
  created_at: string
}

const CHANNELS = ['email', 'sms', 'push', 'slack', 'webhook', 'websocket'] as const
const SOURCES = ['manual', 'unsubscribe_link', 'sms_stop', 'api', 'user_request'] as const

const CHANNEL_FILTER_OPTIONS = ['all', ...CHANNELS] as const

export function OptOutTable() {
  const [optOuts, setOptOuts] = useState<OptOut[]>([])
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState<string>('all')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [form, setForm] = useState({
    user_id: '',
    channel: 'email' as typeof CHANNELS[number],
    reason: '',
    source: 'manual' as typeof SOURCES[number],
  })

  useEffect(() => {
    fetchOptOuts()
  }, [])

  const fetchOptOuts = async () => {
    try {
      const data = await getOptOuts()
      setOptOuts(data || [])
    } catch {
      toast.error('Failed to load opt-outs')
    } finally {
      setLoading(false)
    }
  }

  const handleDelete = async (id: string) => {
    try {
      await deleteOptOut(id)
      setOptOuts(optOuts.filter(o => o.id !== id))
      toast.success('Opt-out removed')
    } catch {
      toast.error('Failed to remove opt-out')
    }
  }

  const handleAdd = async () => {
    if (!form.user_id.trim()) {
      toast.error('User ID is required')
      return
    }
    const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i
    if (!uuidRegex.test(form.user_id.trim())) {
      toast.error('User ID must be a valid UUID')
      return
    }
    setSubmitting(true)
    try {
      const created = await createOptOut({
        user_id: form.user_id.trim(),
        channel: form.channel,
        reason: form.reason.trim() || undefined,
        source: form.source,
      })
      setOptOuts([created, ...optOuts])
      setDialogOpen(false)
      setForm({ user_id: '', channel: 'email', reason: '', source: 'manual' })
      toast.success('Opt-out added')
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : 'Failed to add opt-out')
    } finally {
      setSubmitting(false)
    }
  }

  const filtered = filter === 'all' ? optOuts : optOuts.filter(o => o.channel === filter)

  return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <div className="flex items-center gap-2">
          <h2 className="text-xl font-semibold">Active User Opt-outs</h2>
          <Badge variant="outline">{filtered.length}</Badge>
        </div>
        <div className="flex items-center gap-2">
          <Select value={filter} onValueChange={setFilter}>
            <SelectTrigger className="w-36 h-8 text-xs">
              <SelectValue placeholder="Filter by channel" />
            </SelectTrigger>
            <SelectContent>
              {CHANNEL_FILTER_OPTIONS.map(opt => (
                <SelectItem key={opt} value={opt} className="capitalize">
                  {opt === 'all' ? 'All channels' : opt.toUpperCase()}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button size="sm" onClick={() => setDialogOpen(true)}>
            <Plus className="h-4 w-4 mr-2" /> Add Opt-out
          </Button>
        </div>
      </div>

      <div className="rounded-md border bg-card">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>User ID</TableHead>
              <TableHead>Channel</TableHead>
              <TableHead>Reason</TableHead>
              <TableHead>Source</TableHead>
              <TableHead>Added</TableHead>
              <TableHead className="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              <TableRow>
                <TableCell colSpan={6} className="text-center py-8">Loading...</TableCell>
              </TableRow>
            ) : filtered.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="text-center py-8 text-muted-foreground">
                  No opt-outs found
                </TableCell>
              </TableRow>
            ) : (
              filtered.map((o) => (
                <TableRow key={o.id}>
                  <TableCell className="font-mono text-xs">{truncateId(o.user_id)}</TableCell>
                  <TableCell>
                    <Badge variant="outline">{o.channel.toUpperCase()}</Badge>
                  </TableCell>
                  <TableCell>{o.reason || 'Not specified'}</TableCell>
                  <TableCell>
                    <Badge variant="secondary" className="capitalize">{o.source}</Badge>
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {formatDistanceToNow(new Date(o.created_at))} ago
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => handleDelete(o.id)}
                      className="text-destructive hover:text-destructive hover:bg-destructive/10"
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Add Opt-out</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-2">
              <Label>
                User ID <span className="text-destructive">*</span>
              </Label>
              <Input
                placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
                value={form.user_id}
                onChange={e => setForm(f => ({ ...f, user_id: e.target.value }))}
                className="font-mono text-sm"
              />
            </div>
            <div className="space-y-2">
              <Label>Channel</Label>
              <Select
                value={form.channel}
                onValueChange={(v) => setForm(f => ({ ...f, channel: v as typeof CHANNELS[number] }))}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {CHANNELS.map(ch => (
                    <SelectItem key={ch} value={ch} className="uppercase">{ch.toUpperCase()}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>Source</Label>
              <Select
                value={form.source}
                onValueChange={(v) => setForm(f => ({ ...f, source: v as typeof SOURCES[number] }))}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {SOURCES.map(s => (
                    <SelectItem key={s} value={s} className="capitalize">
                      {s.replace(/_/g, ' ')}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>Reason <span className="text-muted-foreground text-xs">(optional)</span></Label>
              <Input
                placeholder="e.g. user requested unsubscribe, SMS STOP reply"
                value={form.reason}
                onChange={e => setForm(f => ({ ...f, reason: e.target.value }))}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)} disabled={submitting}>
              Cancel
            </Button>
            <Button onClick={handleAdd} disabled={submitting}>
              {submitting ? 'Adding...' : 'Add Opt-out'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
