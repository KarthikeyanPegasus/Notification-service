'use client'

import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { rescheduleNotification } from '@/lib/api'
import type { ScheduledNotification } from '@/types'
import { toISODateString } from '@/lib/utils'
import { format } from 'date-fns'
import { Calendar, AlertCircle } from 'lucide-react'

interface RescheduleDialogProps {
  notification: ScheduledNotification | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function RescheduleDialog({ notification, open, onOpenChange }: RescheduleDialogProps) {
  const queryClient = useQueryClient()
  const [dateValue, setDateValue] = useState('')
  const [timeValue, setTimeValue] = useState('')

  const mutation = useMutation({
    mutationFn: ({ id, scheduledAt }: { id: string; scheduledAt: string }) =>
      rescheduleNotification(id, scheduledAt),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['scheduled'] })
      onOpenChange(false)
      setDateValue('')
      setTimeValue('')
    },
  })

  const handleSubmit = () => {
    if (!notification || !dateValue || !timeValue) return
    const dt = new Date(`${dateValue}T${timeValue}:00`)
    if (isNaN(dt.getTime())) return
    mutation.mutate({ id: notification.notification_id, scheduledAt: toISODateString(dt) })
  }

  const minDate = format(new Date(), 'yyyy-MM-dd')

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Calendar className="h-4 w-4" />
            Reschedule Notification
          </DialogTitle>
          <DialogDescription>
            Choose a new date and time to send notification{' '}
            <code className="text-xs bg-muted px-1 rounded">
              {notification?.id?.substring(0, 12)}...
            </code>
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <label className="text-sm font-medium">New Date</label>
            <Input
              type="date"
              value={dateValue}
              min={minDate}
              onChange={(e) => setDateValue(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <label className="text-sm font-medium">New Time</label>
            <Input
              type="time"
              value={timeValue}
              onChange={(e) => setTimeValue(e.target.value)}
            />
          </div>

          {mutation.isError && (
            <div className="flex items-center gap-2 text-sm text-destructive bg-destructive/10 rounded-md p-3">
              <AlertCircle className="h-4 w-4 shrink-0" />
              Failed to reschedule. Please try again.
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={mutation.isPending}>
            Cancel
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={!dateValue || !timeValue || mutation.isPending}
          >
            {mutation.isPending ? 'Rescheduling...' : 'Confirm Reschedule'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
