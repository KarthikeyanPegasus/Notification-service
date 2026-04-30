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

interface Suppression {
  id: string
  type: 'email' | 'sms' | 'push' | 'slack' | 'webhook'
  value: string
  reason: string
  created_at: string
}

const TYPE_LABELS: Record<string, string> = {
  email: 'Email',
  sms: 'SMS',
  push: 'Push',
  slack: 'Slack',
  webhook: 'Webhook',
}

const TYPE_VARIANTS: Record<string, 'default' | 'secondary' | 'outline'> = {
  email: 'default',
  sms: 'secondary',
  push: 'outline',
  slack: 'outline',
  webhook: 'outline',
}

const TYPE_PLACEHOLDERS: Record<string, string> = {
  email: 'user@example.com',
  sms: '+15551234567',
  push: 'device-token-abc123',
  slack: 'https://hooks.slack.com/services/...',
  webhook: 'https://example.com/callback',
}

const FILTER_OPTIONS = ['all', 'email', 'sms', 'push', 'slack', 'webhook'] as const

export function SuppressionTable() {
  const [suppressions, setSuppressions] = useState<Suppression[]>([])
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState<string>('all')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [form, setForm] = useState({
    type: 'email' as Suppression['type'],
    value: '',
    reason: '',
  })

  useEffect(() => {
    fetchSuppressions()
  }, [])

  const fetchSuppressions = async () => {
    try {
      const res = await fetch(`${process.env.NEXT_PUBLIC_API_URL}/v1/governance/suppressions`)
      if (!res.ok) throw new Error('Failed to fetch')
      const data = await res.json()
      setSuppressions(data || [])
    } catch {
      toast.error('Failed to load suppressions')
    } finally {
      setLoading(false)
    }
  }

  const handleDelete = async (id: string) => {
    try {
      const res = await fetch(`${process.env.NEXT_PUBLIC_API_URL}/v1/governance/suppressions/${id}`, {
        method: 'DELETE',
      })
      if (!res.ok) throw new Error('Delete failed')
      setSuppressions(suppressions.filter(s => s.id !== id))
      toast.success('Suppression removed')
    } catch {
      toast.error('Failed to remove suppression')
    }
  }

  const handleAdd = async () => {
    if (!form.value.trim()) {
      toast.error('Value is required')
      return
    }
    setSubmitting(true)
    try {
      const res = await fetch(`${process.env.NEXT_PUBLIC_API_URL}/v1/governance/suppressions`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          type: form.type,
          value: form.value.trim(),
          reason: form.reason.trim() || undefined,
        }),
      })
      if (!res.ok) {
        const err = await res.json().catch(() => ({}))
        throw new Error(err.error || 'Failed to add suppression')
      }
      const created = await res.json()
      setSuppressions([created, ...suppressions])
      setDialogOpen(false)
      setForm({ type: 'email', value: '', reason: '' })
      toast.success('Suppression added')
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : 'Failed to add suppression')
    } finally {
      setSubmitting(false)
    }
  }

  const filtered = filter === 'all' ? suppressions : suppressions.filter(s => s.type === filter)

  return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <div className="flex items-center gap-2">
          <h2 className="text-xl font-semibold">Active Suppressions</h2>
          <Badge variant="outline">{filtered.length}</Badge>
        </div>
        <div className="flex items-center gap-2">
          <Select value={filter} onValueChange={setFilter}>
            <SelectTrigger className="w-32 h-8 text-xs">
              <SelectValue placeholder="Filter by type" />
            </SelectTrigger>
            <SelectContent>
              {FILTER_OPTIONS.map(opt => (
                <SelectItem key={opt} value={opt} className="capitalize">
                  {opt === 'all' ? 'All types' : TYPE_LABELS[opt]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button size="sm" onClick={() => setDialogOpen(true)}>
            <Plus className="h-4 w-4 mr-2" /> Add Suppression
          </Button>
        </div>
      </div>

      <div className="rounded-md border bg-card">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Type</TableHead>
              <TableHead>Value</TableHead>
              <TableHead>Reason</TableHead>
              <TableHead>Added</TableHead>
              <TableHead className="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              <TableRow>
                <TableCell colSpan={5} className="text-center py-8">Loading...</TableCell>
              </TableRow>
            ) : filtered.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} className="text-center py-8 text-muted-foreground">
                  No suppressions found
                </TableCell>
              </TableRow>
            ) : (
              filtered.map((s) => (
                <TableRow key={s.id}>
                  <TableCell>
                    <Badge variant={TYPE_VARIANTS[s.type] ?? 'outline'}>
                      {TYPE_LABELS[s.type] ?? s.type.toUpperCase()}
                    </Badge>
                  </TableCell>
                  <TableCell className="font-mono text-xs">{s.value}</TableCell>
                  <TableCell>{s.reason || '--'}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {formatDistanceToNow(new Date(s.created_at))} ago
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => handleDelete(s.id)}
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
            <DialogTitle>Add Suppression</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-2">
              <Label>Channel Type</Label>
              <Select
                value={form.type}
                onValueChange={(v) => setForm(f => ({ ...f, type: v as Suppression['type'], value: '' }))}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {Object.entries(TYPE_LABELS).map(([val, label]) => (
                    <SelectItem key={val} value={val}>{label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>
                {TYPE_LABELS[form.type]} Identifier
                <span className="text-destructive ml-1">*</span>
              </Label>
              <Input
                placeholder={TYPE_PLACEHOLDERS[form.type]}
                value={form.value}
                onChange={e => setForm(f => ({ ...f, value: e.target.value }))}
              />
            </div>
            <div className="space-y-2">
              <Label>Reason <span className="text-muted-foreground text-xs">(optional)</span></Label>
              <Input
                placeholder="e.g. hard bounce, user request, spam complaint"
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
              {submitting ? 'Adding...' : 'Add Suppression'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
