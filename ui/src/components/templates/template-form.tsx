'use client'

import { useState, useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import * as z from 'zod'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormDescription,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Loader2 } from 'lucide-react'

const SMS_MAX_LEN = 160

const formSchema = z.object({
  name: z.string().min(2, 'Name must be at least 2 characters'),
  channel: z.enum(['email', 'sms', 'push', 'webhook', 'websocket']),
  subject: z.string().optional(),
  body: z.string().min(1, 'Body is required'),
}).superRefine((val, ctx) => {
  if (val.channel === 'sms') {
    const len = Array.from(val.body ?? '').length
    if (len > SMS_MAX_LEN) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['body'],
        message: `SMS templates must be ${SMS_MAX_LEN} characters or fewer`,
      })
    }
  }
})

import { createTemplate, updateTemplate } from '@/lib/api'

export function TemplateForm({ template, onSuccess, onCancel }: any) {
  const [loading, setLoading] = useState(false)

  const form = useForm<z.infer<typeof formSchema>>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      name: template?.name || '',
      channel: template?.channel || 'email',
      subject: template?.subject || '',
      body: template?.body || '',
    },
  })

  // Watch channel to conditionally show fields
  const currentChannel = form.watch('channel')
  const currentBody = form.watch('body')
  const bodyLen = Array.from(currentBody ?? '').length

  async function onSubmit(values: z.infer<typeof formSchema>) {
    setLoading(true)
    try {
      if (template) {
        await updateTemplate(template.id, values)
      } else {
        await createTemplate(values)
      }
      onSuccess()
    } catch (error) {
      console.error('Error saving template:', error)
    } finally {
      setLoading(false)
    }
  }

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6">
        <div className="grid grid-cols-2 gap-4">
          <FormField
            control={form.control}
            name="name"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Template Name</FormLabel>
                <FormControl>
                  <Input placeholder="Welcome Email" {...field} className="bg-card/50" />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="channel"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Channel</FormLabel>
                <Select onValueChange={field.onChange} defaultValue={field.value}>
                  <FormControl>
                    <SelectTrigger className="bg-card/50">
                      <SelectValue placeholder="Select a channel" />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    <SelectItem value="email">Email</SelectItem>
                    <SelectItem value="sms">SMS</SelectItem>
                    <SelectItem value="push">Push</SelectItem>
                    <SelectItem value="webhook">Webhook</SelectItem>
                    <SelectItem value="websocket">WebSocket</SelectItem>
                  </SelectContent>
                </Select>
                <FormMessage />
              </FormItem>
            )}
          />
        </div>

        {currentChannel === 'email' && (
          <FormField
            control={form.control}
            name="subject"
            render={({ field }) => (
              <FormItem className="animate-in slide-in-from-top duration-300">
                <FormLabel>Subject</FormLabel>
                <FormControl>
                  <Input placeholder="Welcome to {{company}}!" {...field} className="bg-card/50" />
                </FormControl>
                <FormDescription>
                  Supports variable substitution using {"{{variable_name}}"}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        )}

        <FormField
          control={form.control}
          name="body"
          render={({ field }) => (
            <FormItem>
              <FormLabel>
                {currentChannel === 'email' ? 'Email Body (Markdown/HTML)' : 'Message Content'}
              </FormLabel>
              <FormControl>
                <Textarea 
                  placeholder="Hello {{name}}, welcome aboard!" 
                  className="min-h-[200px] font-mono bg-card/50" 
                  maxLength={currentChannel === 'sms' ? SMS_MAX_LEN : undefined}
                  {...field} 
                />
              </FormControl>
              <FormDescription>
                Uses Handlebars-style syntax for variables.
                {currentChannel === 'sms' && (
                  <span className="ml-2 tabular-nums">
                    {bodyLen}/{SMS_MAX_LEN}
                  </span>
                )}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <div className="flex justify-end gap-3 pt-4">
          <Button type="button" variant="ghost" onClick={onCancel}>
            Cancel
          </Button>
          <Button type="submit" disabled={loading} className="min-w-[100px]">
            {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : template ? 'Save Changes' : 'Create Template'}
          </Button>
        </div>
      </form>
    </Form>
  )
}
