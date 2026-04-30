'use client'

import { useState, useEffect } from 'react'
import { Calendar, Clock, Loader2, Plus, Send, User } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { createNotification, getVendorConfigs, listMyClients, type ApiClientKey } from '@/lib/api'
import { Channel, VendorConfig } from '@/types'
import { toast } from 'sonner'

const EMPTY = 'none'

export function ScheduleNotificationDialog() {
  const [open, setOpen] = useState(false)
  const [loading, setLoading] = useState(false)
  const [vendors, setVendors] = useState<VendorConfig[]>([])
  const [clients, setClients] = useState<ApiClientKey[]>([])

  const [form, setForm] = useState({
    channel: 'email' as Channel,
    recipient: '',
    vendor: EMPTY,
    client_id: EMPTY,
    scheduled_at: '',
    subject: '',
    body: '',
  })

  useEffect(() => {
    if (!open) return
    getVendorConfigs().then(setVendors).catch(console.error)
    listMyClients().then(setClients).catch(console.error)
  }, [open])

  const isScheduled = Boolean(form.scheduled_at)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)
    try {
      await createNotification({
        channels: [form.channel],
        recipient: form.recipient,
        type: 'transactional',
        subject: form.channel === 'email' ? form.subject : undefined,
        body: form.body,
        scheduled_at: form.scheduled_at ? new Date(form.scheduled_at).toISOString() : undefined,
        forced_vendor: form.vendor === EMPTY ? undefined : form.vendor,
        client_id: form.client_id === EMPTY ? undefined : form.client_id,
        idempotency_key: crypto.randomUUID(),
      })
      toast.success(isScheduled ? 'Notification scheduled' : 'Notification sent')
      setOpen(false)
      setForm({
        channel: 'email',
        recipient: '',
        vendor: EMPTY,
        client_id: EMPTY,
        scheduled_at: '',
        subject: '',
        body: '',
      })
      window.location.reload()
    } catch (error: any) {
      toast.error(error.message || 'Failed to schedule notification')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button className="gap-2">
          <Plus className="h-4 w-4" />
          Schedule Message
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-[520px]">
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>Schedule Notification</DialogTitle>
            <DialogDescription>
              Create a notification to be sent immediately or at a future time.
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-4 py-4">
            {/* Channel + Vendor */}
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="channel">Channel</Label>
                <Select
                  value={form.channel}
                  onValueChange={(v) => setForm({ ...form, channel: v as Channel })}
                >
                  <SelectTrigger id="channel">
                    <SelectValue placeholder="Select channel" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="email">Email</SelectItem>
                    <SelectItem value="sms">SMS</SelectItem>
                    <SelectItem value="push">Push</SelectItem>
                    <SelectItem value="slack">Slack</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <div className="space-y-2">
                <Label htmlFor="vendor">Vendor</Label>
                <Select
                  value={form.vendor}
                  onValueChange={(v) => setForm({ ...form, vendor: v })}
                >
                  <SelectTrigger id="vendor">
                    <SelectValue placeholder="Auto-route" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={EMPTY}>Auto-route (default)</SelectItem>
                    {vendors.map((v) => (
                      <SelectItem key={v.id} value={v.vendor_type}>
                        {v.vendor_type}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            {/* Recipient */}
            <div className="space-y-2">
              <Label htmlFor="recipient">Recipient</Label>
              <Input
                id="recipient"
                placeholder={form.channel === 'email' ? 'email@example.com' : '+1234567890'}
                value={form.recipient}
                onChange={(e) => setForm({ ...form, recipient: e.target.value })}
                required
              />
            </div>

            {/* Email subject */}
            {form.channel === 'email' && (
              <div className="space-y-2">
                <Label htmlFor="subject">Subject</Label>
                <Input
                  id="subject"
                  placeholder="Notification subject"
                  value={form.subject}
                  onChange={(e) => setForm({ ...form, subject: e.target.value })}
                  required
                />
              </div>
            )}

            {/* Body */}
            <div className="space-y-2">
              <Label htmlFor="body">Message</Label>
              <Textarea
                id="body"
                placeholder="Message body..."
                value={form.body}
                onChange={(e) => setForm({ ...form, body: e.target.value })}
                required
                className="min-h-[100px]"
              />
            </div>

            {/* Schedule time */}
            <div className="space-y-2">
              <Label htmlFor="scheduled_at">Schedule Time (optional)</Label>
              <div className="relative">
                <Input
                  id="scheduled_at"
                  type="datetime-local"
                  value={form.scheduled_at}
                  onChange={(e) => setForm({ ...form, scheduled_at: e.target.value })}
                  className="pl-9"
                />
                <Clock className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
              </div>
              <p className="text-[10px] text-muted-foreground">Leave empty to send immediately.</p>
            </div>

            {/* Client selector — only relevant when scheduling */}
            {isScheduled && (
              <div className="space-y-2">
                <Label htmlFor="client_id" className="flex items-center gap-1.5">
                  <User className="h-3.5 w-3.5 text-muted-foreground" />
                  Client (workflow engine)
                </Label>
                <Select
                  value={form.client_id}
                  onValueChange={(v) => setForm({ ...form, client_id: v })}
                >
                  <SelectTrigger id="client_id">
                    <SelectValue placeholder="Default (global)" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={EMPTY}>Default (global engine)</SelectItem>
                    {clients.map((c) => (
                      <SelectItem key={c.id} value={c.id}>
                        {c.name}
                        <span className="ml-1.5 text-muted-foreground text-xs">({c.prefix})</span>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-[10px] text-muted-foreground">
                  Selects which client&apos;s Temporal / Cadence configuration handles this
                  schedule. Uses the global engine when left on default.
                </p>
              </div>
            )}
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={loading} className="gap-2">
              {loading ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : isScheduled ? (
                <Calendar className="h-4 w-4" />
              ) : (
                <Send className="h-4 w-4" />
              )}
              {isScheduled ? 'Schedule' : 'Send Now'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
