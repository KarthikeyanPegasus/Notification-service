'use client'

import React, { useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Loader2 } from 'lucide-react'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { getVendorConfigsScoped, updateVendorConfigScoped } from '@/lib/api'

type RoutingMode = 'backup' | 'round_robin' | 'publish_all' | 'only'

type SMSRouting = {
  mode: RoutingMode
  prefer: string
  fallback: string
  only: string
  participants: string[]
  error_rate_threshold: number
  min_requests: number
}

type EmailRouting = {
  mode: RoutingMode
  prefer: string
  fallback: string
  only: string
  participants: string[]
  error_rate_threshold: number
  min_requests: number
}

type PushRouting = {
  mode: RoutingMode
  prefer: string
  fallback: string
  only: string
  participants: string[]
  error_rate_threshold: number
  min_requests: number
}

const DEFAULT_SMS_ROUTING: SMSRouting = {
  mode: 'backup',
  prefer: 'twilio',
  fallback: 'plivo',
  only: 'twilio',
  participants: ['twilio', 'plivo', 'vonage', 'messagebird'],
  error_rate_threshold: 0,
  min_requests: 20,
}

const DEFAULT_EMAIL_ROUTING: EmailRouting = {
  mode: 'backup',
  prefer: 'ses',
  fallback: 'smtp',
  only: 'ses',
  participants: ['ses', 'smtp', 'mailgun', 'sendgrid', 'postmark'],
  error_rate_threshold: 0,
  min_requests: 20,
}

const DEFAULT_PUSH_ROUTING: PushRouting = {
  mode: 'backup',
  prefer: 'fcm',
  fallback: 'fcm',
  only: 'fcm',
  participants: ['fcm', 'onesignal', 'pusher'],
  error_rate_threshold: 0,
  min_requests: 20,
}

const SMS_VENDOR_LABELS: Record<string, string> = {
  twilio: 'Twilio',
  plivo: 'Plivo',
  vonage: 'Vonage',
  messagebird: 'MessageBird',
}

const EMAIL_VENDOR_LABELS: Record<string, string> = {
  ses: 'Amazon SES',
  smtp: 'SMTP Relay',
  mailgun: 'Mailgun',
  sendgrid: 'SendGrid',
  postmark: 'Postmark',
}

const PUSH_VENDOR_LABELS: Record<string, string> = {
  fcm: 'Firebase',
  onesignal: 'OneSignal',
  pusher: 'Pusher',
}

export function ChannelRoutingSettings({ apiKeyId }: { apiKeyId?: string }) {
  const queryClient = useQueryClient()
  const { data: configsData, isLoading } = useQuery({
    queryKey: ['vendor-configs', apiKeyId ?? 'global'],
    queryFn: () => getVendorConfigsScoped(apiKeyId),
    retry: 1,
  })

  const configs = useMemo(() => configsData ?? [], [configsData])

  const configuredSmsVendors = useMemo(() => {
    const all = ['twilio', 'plivo', 'vonage', 'messagebird'] as const
    return all.filter((id) => configs.some((c) => c.vendor_type === id))
  }, [configs])

  const configuredEmailVendors = useMemo(() => {
    const out: Array<'ses' | 'smtp' | 'mailgun' | 'sendgrid' | 'postmark'> = []
    if (configs.some((c) => c.vendor_type === 'ses')) out.push('ses')
    if (configs.some((c) => ['email', 'smtp'].includes(c.vendor_type))) out.push('smtp')
    if (configs.some((c) => c.vendor_type === 'mailgun')) out.push('mailgun')
    if (configs.some((c) => c.vendor_type === 'sendgrid')) out.push('sendgrid')
    if (configs.some((c) => c.vendor_type === 'postmark')) out.push('postmark')
    return out
  }, [configs])

  const configuredPushVendors = useMemo(() => {
    const out: Array<'fcm' | 'onesignal' | 'pusher'> = []
    if (configs.some((c) => c.vendor_type === 'fcm')) out.push('fcm')
    if (configs.some((c) => c.vendor_type === 'onesignal')) out.push('onesignal')
    if (configs.some((c) => c.vendor_type === 'pusher')) out.push('pusher')
    return out
  }, [configs])

  const [smsRouting, setSmsRouting] = useState<SMSRouting>(DEFAULT_SMS_ROUTING)
  const [emailRouting, setEmailRouting] = useState<EmailRouting>(DEFAULT_EMAIL_ROUTING)
  const [pushRouting, setPushRouting] = useState<PushRouting>(DEFAULT_PUSH_ROUTING)

  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)

  useEffect(() => {
    setError(null)
    setSuccess(null)
    for (const cfg of configs) {
      if (cfg.vendor_type === 'sms_routing' && cfg.config_json) setSmsRouting({ ...DEFAULT_SMS_ROUTING, ...(cfg.config_json as any) })
      if (cfg.vendor_type === 'email_routing' && cfg.config_json) setEmailRouting({ ...DEFAULT_EMAIL_ROUTING, ...(cfg.config_json as any) })
      if (cfg.vendor_type === 'push_routing' && cfg.config_json) setPushRouting({ ...DEFAULT_PUSH_ROUTING, ...(cfg.config_json as any) })
    }
  }, [configs])

  const handleSave = async (vendorType: string, routing: any) => {
    setSaving(true)
    setError(null)
    setSuccess(null)
    try {
      await updateVendorConfigScoped(vendorType, routing, apiKeyId)
      queryClient.invalidateQueries({ queryKey: ['vendor-configs', apiKeyId ?? 'global'] })
      setSuccess('Saved.')
      setTimeout(() => setSuccess(null), 2500)
    } catch (e) {
      console.error(e)
      setError(`Failed to save ${vendorType}.`)
    } finally {
      setSaving(false)
    }
  }

  const smsPreferFallbacked =
    configuredSmsVendors.length > 0
      ? {
          ...smsRouting,
          prefer: configuredSmsVendors.includes(smsRouting.prefer as any) ? smsRouting.prefer : configuredSmsVendors[0],
          fallback: configuredSmsVendors.includes(smsRouting.fallback as any) ? smsRouting.fallback : configuredSmsVendors[0],
          only: configuredSmsVendors.includes(smsRouting.only as any) ? smsRouting.only : configuredSmsVendors[0],
          participants: Array.isArray(smsRouting.participants) && smsRouting.participants.length > 0 ? smsRouting.participants : [...configuredSmsVendors],
        }
      : smsRouting

  const emailPreferFallbacked =
    configuredEmailVendors.length > 0
      ? {
          ...emailRouting,
          prefer: configuredEmailVendors.includes(emailRouting.prefer as any) ? emailRouting.prefer : configuredEmailVendors[0],
          fallback: configuredEmailVendors.includes(emailRouting.fallback as any) ? emailRouting.fallback : configuredEmailVendors[0],
          only: configuredEmailVendors.includes(emailRouting.only as any) ? emailRouting.only : configuredEmailVendors[0],
          participants:
            Array.isArray(emailRouting.participants) && emailRouting.participants.length > 0 ? emailRouting.participants : [...configuredEmailVendors],
        }
      : emailRouting

  const pushPreferFallbacked =
    configuredPushVendors.length > 0
      ? {
          ...pushRouting,
          prefer: configuredPushVendors.includes(pushRouting.prefer as any) ? pushRouting.prefer : configuredPushVendors[0],
          fallback: configuredPushVendors.includes(pushRouting.fallback as any) ? pushRouting.fallback : configuredPushVendors[0],
          only: configuredPushVendors.includes(pushRouting.only as any) ? pushRouting.only : configuredPushVendors[0],
          participants:
            Array.isArray(pushRouting.participants) && pushRouting.participants.length > 0 ? pushRouting.participants : [...configuredPushVendors],
        }
      : pushRouting

  return (
    <div className="space-y-4">
      {error && <p className="text-sm text-destructive rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3">{error}</p>}
      {success && <p className="text-sm text-green-700 rounded-lg border border-green-500/30 bg-green-500/10 px-4 py-3">{success}</p>}

      {isLoading ? (
        <div className="flex items-center justify-center py-10 text-muted-foreground">
          <Loader2 className="h-6 w-6 animate-spin" />
        </div>
      ) : (
        <>
          <Card>
            <CardHeader>
              <CardTitle className="text-lg flex items-center gap-2">Delivery Preference (SMS)</CardTitle>
              <CardDescription>How the SMS worker chooses vendors for this client.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <label className="text-xs font-semibold text-muted-foreground uppercase">Routing Mode</label>
                  <Select value={smsPreferFallbacked.mode} onValueChange={(v) => setSmsRouting((p) => ({ ...p, mode: v as any }))}>
                    <SelectTrigger>
                      <SelectValue placeholder="Select routing mode" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="backup">Backup (prefer then fallback)</SelectItem>
                      <SelectItem value="round_robin">Round robin</SelectItem>
                      <SelectItem value="publish_all">Publish all vendors</SelectItem>
                      <SelectItem value="only">Only one vendor</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                {smsPreferFallbacked.mode !== 'publish_all' ? (
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase">Prefer Vendor</label>
                    <Select value={smsPreferFallbacked.prefer} onValueChange={(v) => setSmsRouting((p) => ({ ...p, prefer: v as any }))}>
                      <SelectTrigger>
                        <SelectValue placeholder="Select preferred vendor" />
                      </SelectTrigger>
                      <SelectContent>
                        {configuredSmsVendors.map((id) => (
                          <SelectItem key={id} value={id}>
                            {SMS_VENDOR_LABELS[id] ?? id}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                ) : (
                  <div />
                )}
              </div>

              {smsPreferFallbacked.mode === 'backup' && (
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase">Fallback Vendor</label>
                    <Select value={smsPreferFallbacked.fallback} onValueChange={(v) => setSmsRouting((p) => ({ ...p, fallback: v as any }))}>
                      <SelectTrigger>
                        <SelectValue placeholder="Select fallback vendor" />
                      </SelectTrigger>
                      <SelectContent>
                        {configuredSmsVendors.map((id) => (
                          <SelectItem key={id} value={id}>
                            {SMS_VENDOR_LABELS[id] ?? id}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div />
                </div>
              )}

              {smsPreferFallbacked.mode === 'round_robin' && (
                <div className="space-y-2">
                  <label className="text-xs font-semibold text-muted-foreground uppercase">Round Robin Participants</label>
                  <div className="grid grid-cols-3 gap-3">
                    {configuredSmsVendors.map((v) => {
                      const checked = smsPreferFallbacked.participants.includes(v)
                      return (
                        <label key={v} className="flex items-center gap-2 text-sm">
                          <input
                            type="checkbox"
                            className="h-4 w-4"
                            checked={checked}
                            onChange={(e) => {
                              const next = e.target.checked
                                ? Array.from(new Set([...smsPreferFallbacked.participants, v]))
                                : smsPreferFallbacked.participants.filter((x) => x !== v)
                              setSmsRouting((p) => ({ ...p, participants: next as any }))
                            }}
                          />
                          {SMS_VENDOR_LABELS[v] ?? v}
                        </label>
                      )
                    })}
                  </div>
                  <p className="text-xs text-muted-foreground">At least one participant should be selected.</p>
                </div>
              )}

              {smsPreferFallbacked.mode === 'only' && (
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase">Only Vendor</label>
                    <Select value={smsPreferFallbacked.only} onValueChange={(v) => setSmsRouting((p) => ({ ...p, only: v as any }))}>
                      <SelectTrigger>
                        <SelectValue placeholder="Select only vendor" />
                      </SelectTrigger>
                      <SelectContent>
                        {configuredSmsVendors.map((id) => (
                          <SelectItem key={id} value={id}>
                            {SMS_VENDOR_LABELS[id] ?? id}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div />
                </div>
              )}

              {smsPreferFallbacked.mode === 'backup' && (
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase">Error-rate fallback threshold</label>
                    <Input
                      type="number"
                      step="0.01"
                      min="0"
                      max="1"
                      value={String(smsPreferFallbacked.error_rate_threshold ?? 0)}
                      onChange={(e) => setSmsRouting((p) => ({ ...p, error_rate_threshold: Number(e.target.value) }))}
                      placeholder="0.25"
                    />
                  </div>
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase">Min requests</label>
                    <Input
                      type="number"
                      min="1"
                      value={String(smsPreferFallbacked.min_requests ?? 20)}
                      onChange={(e) => setSmsRouting((p) => ({ ...p, min_requests: Number(e.target.value) }))}
                      placeholder="20"
                    />
                  </div>
                </div>
              )}
            </CardContent>
            <CardFooter className="bg-muted/50 py-3 flex justify-between">
              <p className="text-xs text-muted-foreground italic">Worker vendor selection logic for this client.</p>
              <div className="flex items-center gap-2">
                <Button disabled={saving || configuredSmsVendors.length === 0} onClick={() => handleSave('sms_routing', smsPreferFallbacked)}>
                  {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : 'Save SMS routing'}
                </Button>
              </div>
            </CardFooter>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-lg flex items-center gap-2">Delivery Preference (Email)</CardTitle>
              <CardDescription>How the email worker chooses vendors for this client.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <label className="text-xs font-semibold text-muted-foreground uppercase">Routing Mode</label>
                  <Select value={emailPreferFallbacked.mode} onValueChange={(v) => setEmailRouting((p) => ({ ...p, mode: v as any }))}>
                    <SelectTrigger>
                      <SelectValue placeholder="Select routing mode" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="backup">Backup (prefer then fallback)</SelectItem>
                      <SelectItem value="round_robin">Round robin</SelectItem>
                      <SelectItem value="publish_all">Publish all vendors</SelectItem>
                      <SelectItem value="only">Only one vendor</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                {emailPreferFallbacked.mode !== 'publish_all' ? (
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase">Prefer Vendor</label>
                    <Select value={emailPreferFallbacked.prefer} onValueChange={(v) => setEmailRouting((p) => ({ ...p, prefer: v as any }))}>
                      <SelectTrigger>
                        <SelectValue placeholder="Select preferred vendor" />
                      </SelectTrigger>
                      <SelectContent>
                        {configuredEmailVendors.map((id) => (
                          <SelectItem key={id} value={id}>
                            {EMAIL_VENDOR_LABELS[id] ?? id}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                ) : (
                  <div />
                )}
              </div>

              {emailPreferFallbacked.mode === 'backup' && (
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase">Fallback Vendor</label>
                    <Select value={emailPreferFallbacked.fallback} onValueChange={(v) => setEmailRouting((p) => ({ ...p, fallback: v as any }))}>
                      <SelectTrigger>
                        <SelectValue placeholder="Select fallback vendor" />
                      </SelectTrigger>
                      <SelectContent>
                        {configuredEmailVendors.map((id) => (
                          <SelectItem key={id} value={id}>
                            {EMAIL_VENDOR_LABELS[id] ?? id}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div />
                </div>
              )}

              {emailPreferFallbacked.mode === 'round_robin' && (
                <div className="space-y-2">
                  <label className="text-xs font-semibold text-muted-foreground uppercase">Round Robin Participants</label>
                  <div className="grid grid-cols-3 gap-3">
                    {configuredEmailVendors.map((v) => {
                      const checked = emailPreferFallbacked.participants.includes(v)
                      return (
                        <label key={v} className="flex items-center gap-2 text-sm">
                          <input
                            type="checkbox"
                            className="h-4 w-4"
                            checked={checked}
                            onChange={(e) => {
                              const next = e.target.checked
                                ? Array.from(new Set([...emailPreferFallbacked.participants, v]))
                                : emailPreferFallbacked.participants.filter((x) => x !== v)
                              setEmailRouting((p) => ({ ...p, participants: next as any }))
                            }}
                          />
                          {EMAIL_VENDOR_LABELS[v] ?? v}
                        </label>
                      )
                    })}
                  </div>
                  <p className="text-xs text-muted-foreground">At least one participant should be selected.</p>
                </div>
              )}

              {emailPreferFallbacked.mode === 'only' && (
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase">Only Vendor</label>
                    <Select value={emailPreferFallbacked.only} onValueChange={(v) => setEmailRouting((p) => ({ ...p, only: v as any }))}>
                      <SelectTrigger>
                        <SelectValue placeholder="Select only vendor" />
                      </SelectTrigger>
                      <SelectContent>
                        {configuredEmailVendors.map((id) => (
                          <SelectItem key={id} value={id}>
                            {EMAIL_VENDOR_LABELS[id] ?? id}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div />
                </div>
              )}

              {emailPreferFallbacked.mode === 'backup' && (
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase">Error-rate fallback threshold</label>
                    <Input
                      type="number"
                      step="0.01"
                      min="0"
                      max="1"
                      value={String(emailPreferFallbacked.error_rate_threshold ?? 0)}
                      onChange={(e) => setEmailRouting((p) => ({ ...p, error_rate_threshold: Number(e.target.value) }))}
                      placeholder="0.25"
                    />
                  </div>
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase">Min requests</label>
                    <Input
                      type="number"
                      min="1"
                      value={String(emailPreferFallbacked.min_requests ?? 20)}
                      onChange={(e) => setEmailRouting((p) => ({ ...p, min_requests: Number(e.target.value) }))}
                      placeholder="20"
                    />
                  </div>
                </div>
              )}
            </CardContent>
            <CardFooter className="bg-muted/50 py-3 flex justify-between">
              <p className="text-xs text-muted-foreground italic">Worker vendor selection logic for this client.</p>
              <div className="flex items-center gap-2">
                <Button disabled={saving || configuredEmailVendors.length === 0} onClick={() => handleSave('email_routing', emailPreferFallbacked)}>
                  {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : 'Save Email routing'}
                </Button>
              </div>
            </CardFooter>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-lg flex items-center gap-2">Delivery Preference (Push)</CardTitle>
              <CardDescription>How the push worker chooses vendors for this client.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <label className="text-xs font-semibold text-muted-foreground uppercase">Routing Mode</label>
                  <Select value={pushPreferFallbacked.mode} onValueChange={(v) => setPushRouting((p) => ({ ...p, mode: v as any }))}>
                    <SelectTrigger>
                      <SelectValue placeholder="Select routing mode" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="backup">Backup (prefer then fallback)</SelectItem>
                      <SelectItem value="round_robin">Round robin</SelectItem>
                      <SelectItem value="publish_all">Publish all vendors</SelectItem>
                      <SelectItem value="only">Only one vendor</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                {pushPreferFallbacked.mode !== 'publish_all' ? (
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase">Prefer Vendor</label>
                    <Select value={pushPreferFallbacked.prefer} onValueChange={(v) => setPushRouting((p) => ({ ...p, prefer: v as any }))}>
                      <SelectTrigger>
                        <SelectValue placeholder="Select preferred vendor" />
                      </SelectTrigger>
                      <SelectContent>
                        {configuredPushVendors.map((id) => (
                          <SelectItem key={id} value={id}>
                            {PUSH_VENDOR_LABELS[id] ?? id}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                ) : (
                  <div />
                )}
              </div>

              {pushPreferFallbacked.mode === 'backup' && (
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase">Fallback Vendor</label>
                    <Select value={pushPreferFallbacked.fallback} onValueChange={(v) => setPushRouting((p) => ({ ...p, fallback: v as any }))}>
                      <SelectTrigger>
                        <SelectValue placeholder="Select fallback vendor" />
                      </SelectTrigger>
                      <SelectContent>
                        {configuredPushVendors.map((id) => (
                          <SelectItem key={id} value={id}>
                            {PUSH_VENDOR_LABELS[id] ?? id}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div />
                </div>
              )}

              {pushPreferFallbacked.mode === 'round_robin' && (
                <div className="space-y-2">
                  <label className="text-xs font-semibold text-muted-foreground uppercase">Round Robin Participants</label>
                  <div className="grid grid-cols-3 gap-3">
                    {configuredPushVendors.map((v) => {
                      const checked = pushPreferFallbacked.participants.includes(v)
                      return (
                        <label key={v} className="flex items-center gap-2 text-sm">
                          <input
                            type="checkbox"
                            className="h-4 w-4"
                            checked={checked}
                            onChange={(e) => {
                              const next = e.target.checked
                                ? Array.from(new Set([...pushPreferFallbacked.participants, v]))
                                : pushPreferFallbacked.participants.filter((x) => x !== v)
                              setPushRouting((p) => ({ ...p, participants: next as any }))
                            }}
                          />
                          {PUSH_VENDOR_LABELS[v] ?? v}
                        </label>
                      )
                    })}
                  </div>
                  <p className="text-xs text-muted-foreground">At least one participant should be selected.</p>
                </div>
              )}

              {pushPreferFallbacked.mode === 'only' && (
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase">Only Vendor</label>
                    <Select value={pushPreferFallbacked.only} onValueChange={(v) => setPushRouting((p) => ({ ...p, only: v as any }))}>
                      <SelectTrigger>
                        <SelectValue placeholder="Select only vendor" />
                      </SelectTrigger>
                      <SelectContent>
                        {configuredPushVendors.map((id) => (
                          <SelectItem key={id} value={id}>
                            {PUSH_VENDOR_LABELS[id] ?? id}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div />
                </div>
              )}

              {pushPreferFallbacked.mode === 'backup' && (
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase">Error-rate fallback threshold</label>
                    <Input
                      type="number"
                      step="0.01"
                      min="0"
                      max="1"
                      value={String(pushPreferFallbacked.error_rate_threshold ?? 0)}
                      onChange={(e) => setPushRouting((p) => ({ ...p, error_rate_threshold: Number(e.target.value) }))}
                      placeholder="0.25"
                    />
                  </div>
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase">Min requests</label>
                    <Input
                      type="number"
                      min="1"
                      value={String(pushPreferFallbacked.min_requests ?? 20)}
                      onChange={(e) => setPushRouting((p) => ({ ...p, min_requests: Number(e.target.value) }))}
                      placeholder="20"
                    />
                  </div>
                </div>
              )}
            </CardContent>
            <CardFooter className="bg-muted/50 py-3 flex justify-between">
              <p className="text-xs text-muted-foreground italic">Worker vendor selection logic for this client.</p>
              <div className="flex items-center gap-2">
                <Button disabled={saving || configuredPushVendors.length === 0} onClick={() => handleSave('push_routing', pushPreferFallbacked)}>
                  {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : 'Save Push routing'}
                </Button>
              </div>
            </CardFooter>
          </Card>
        </>
      )}
    </div>
  )
}

