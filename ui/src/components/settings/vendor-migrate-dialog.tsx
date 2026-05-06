'use client'

import React, { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { ArrowRight, Loader2, ShieldCheck, Zap, TrendingUp } from 'lucide-react'
import { startVendorMigration } from '@/lib/api'
import type { VendorMigration, MigrationStrategy } from '@/types'
import { cn } from '@/lib/utils'

// ── Vendor metadata ──────────────────────────────────────────────────────────

const CHANNEL_VENDORS: Record<string, { id: string; label: string }[]> = {
  email: [
    { id: 'ses', label: 'Amazon SES' },
    { id: 'smtp', label: 'SMTP Relay' },
    { id: 'mailgun', label: 'Mailgun' },
    { id: 'sendgrid', label: 'SendGrid' },
    { id: 'postmark', label: 'Postmark' },
  ],
  sms: [
    { id: 'twilio', label: 'Twilio' },
    { id: 'plivo', label: 'Plivo' },
    { id: 'vonage', label: 'Vonage' },
    { id: 'messagebird', label: 'MessageBird' },
  ],
  push: [
    { id: 'fcm', label: 'Firebase (FCM)' },
    { id: 'onesignal', label: 'OneSignal' },
    { id: 'pusher', label: 'Pusher Beams' },
  ],
}

const CONFIG_TEMPLATES: Record<string, Record<string, any>> = {
  ses:         { region: '', access_key_id: '', secret_access_key: '', from_address: '', from_name: '', reply_to: '' },
  smtp:        { host: '', port: 587, username: '', password: '', from: '', reply_to: '' },
  mailgun:     { domain: '', api_key: '', from: '', reply_to: '' },
  sendgrid:    { api_key: '', from_email: '', from_name: '', reply_to: '' },
  postmark:    { server_token: '', from_email: '', from_name: '', reply_to: '' },
  twilio:      { account_sid: '', auth_token: '', from_number: '' },
  plivo:       { auth_id: '', auth_token: '', from_number: '' },
  vonage:      { api_key: '', api_secret: '', from: '' },
  messagebird: { access_key: '', originator: '' },
  fcm:         { server_key: '' },
  onesignal:   { app_id: '', rest_api_key: '' },
  pusher:      { instance_id: '', secret_key: '' },
}

// ── Types ────────────────────────────────────────────────────────────────────

type Channel = 'email' | 'sms' | 'push'
type Step = 'channel' | 'vendors' | 'config' | 'strategy' | 'confirm'

const STEPS: Step[] = ['channel', 'vendors', 'config', 'strategy', 'confirm']
const STEP_LABELS: Record<Step, string> = {
  channel:  '1. Channel',
  vendors:  '2. Vendors',
  config:   '3. New Config',
  strategy: '4. Strategy',
  confirm:  '5. Confirm',
}

interface Props {
  /** Pre-select the channel when opening from a specific tab. */
  defaultChannel?: Channel
  apiKeyId?: string
  onMigrationStarted?: (m: VendorMigration) => void
  children?: React.ReactNode
}

// ── Component ────────────────────────────────────────────────────────────────

export function VendorMigrateDialog({ defaultChannel, apiKeyId, onMigrationStarted, children }: Props) {
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [step, setStep] = useState<Step>('channel')
  const [channel, setChannel] = useState<Channel>(defaultChannel ?? 'email')
  const [fromVendor, setFromVendor] = useState('')
  const [toVendor, setToVendor] = useState('')
  const [configJson, setConfigJson] = useState('')
  const [configError, setConfigError] = useState<string | null>(null)
  const [strategy, setStrategy] = useState<MigrationStrategy>('instant')
  const [submitting, setSubmitting] = useState(false)
  const [apiError, setApiError] = useState<string | null>(null)

  const vendors = CHANNEL_VENDORS[channel] ?? []
  const isSameVendor = fromVendor !== '' && fromVendor === toVendor

  function reset() {
    setStep('channel')
    setChannel(defaultChannel ?? 'email')
    setFromVendor('')
    setToVendor('')
    setConfigJson('')
    setConfigError(null)
    setStrategy('instant')
    setApiError(null)
  }

  function handleOpenChange(v: boolean) {
    setOpen(v)
    if (!v) reset()
  }

  function stepIndex(s: Step) { return STEPS.indexOf(s) }
  const currentIdx = stepIndex(step)

  function next() {
    const s = STEPS[currentIdx + 1]
    if (s) setStep(s)
  }
  function back() {
    const s = STEPS[currentIdx - 1]
    if (s) setStep(s)
  }

  // Pre-fill config template when toVendor changes
  function handleToVendorChange(v: string) {
    setToVendor(v)
    const template = CONFIG_TEMPLATES[v]
    if (template) {
      setConfigJson(JSON.stringify(template, null, 2))
    } else {
      setConfigJson('{\n  \n}')
    }
    setConfigError(null)
  }

  function validateConfig(): boolean {
    try {
      JSON.parse(configJson)
      setConfigError(null)
      return true
    } catch {
      setConfigError('Invalid JSON — please check the config.')
      return false
    }
  }

  async function handleSubmit() {
    if (!validateConfig()) return
    setSubmitting(true)
    setApiError(null)
    try {
      const migration = await startVendorMigration(
        {
          channel,
          from_vendor: fromVendor,
          to_vendor: toVendor,
          to_config: JSON.parse(configJson),
          strategy,
        },
        apiKeyId,
      )
      queryClient.invalidateQueries({ queryKey: ['vendor-migrations'] })
      queryClient.invalidateQueries({ queryKey: ['vendor-configs'] })
      onMigrationStarted?.(migration)
      setOpen(false)
      reset()
    } catch (e: any) {
      setApiError(e?.message ?? 'Failed to start migration.')
    } finally {
      setSubmitting(false)
    }
  }

  // ── Step content ──────────────────────────────────────────────────────────

  function StepChannel() {
    return (
      <div className="space-y-4">
        <p className="text-sm text-muted-foreground">
          Which notification channel are you migrating?
        </p>
        <div className="grid grid-cols-3 gap-3">
          {(['email', 'sms', 'push'] as Channel[]).map((ch) => (
            <button
              key={ch}
              onClick={() => { setChannel(ch); setFromVendor(''); setToVendor(''); setConfigJson('') }}
              className={cn(
                'rounded-lg border-2 p-4 text-center text-sm font-medium transition-colors',
                channel === ch
                  ? 'border-primary bg-primary/5 text-primary'
                  : 'border-border hover:border-primary/50 text-muted-foreground',
              )}
            >
              {ch.toUpperCase()}
            </button>
          ))}
        </div>
      </div>
    )
  }

  function StepVendors() {
    return (
      <div className="space-y-4">
        <p className="text-sm text-muted-foreground">
          Select the current vendor and where you want to migrate to.
          Choose the same vendor on both sides to swap credentials only.
        </p>
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <label className="text-xs font-medium text-muted-foreground uppercase tracking-wide">From (current)</label>
            <Select value={fromVendor} onValueChange={setFromVendor}>
              <SelectTrigger>
                <SelectValue placeholder="Current vendor" />
              </SelectTrigger>
              <SelectContent>
                {vendors.map((v) => (
                  <SelectItem key={v.id} value={v.id}>{v.label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <label className="text-xs font-medium text-muted-foreground uppercase tracking-wide">To (new)</label>
            <Select value={toVendor} onValueChange={handleToVendorChange}>
              <SelectTrigger>
                <SelectValue placeholder="Target vendor" />
              </SelectTrigger>
              <SelectContent>
                {vendors.map((v) => (
                  <SelectItem key={v.id} value={v.id}>{v.label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
        {isSameVendor && (
          <div className="rounded-md bg-blue-50 dark:bg-blue-950/30 border border-blue-200 dark:border-blue-800 p-3 text-xs text-blue-700 dark:text-blue-300">
            <strong>Config swap mode:</strong> You selected the same vendor on both sides.
            This will hot-swap credentials (API key, domain, etc.) without changing routing.
            Your historical reputation metrics remain intact.
          </div>
        )}
      </div>
    )
  }

  function StepConfig() {
    return (
      <div className="space-y-3">
        <p className="text-sm text-muted-foreground">
          Provide the credentials for <strong>{toVendor}</strong>.
          The JSON is pre-filled with the expected shape — fill in your actual values.
        </p>
        <Textarea
          value={configJson}
          onChange={(e) => { setConfigJson(e.target.value); setConfigError(null) }}
          rows={10}
          className="font-mono text-xs"
          placeholder="{}"
          onBlur={validateConfig}
        />
        {configError && (
          <p className="text-xs text-destructive">{configError}</p>
        )}
      </div>
    )
  }

  function StepStrategy() {
    return (
      <div className="space-y-4">
        <p className="text-sm text-muted-foreground">
          How do you want to cut over traffic?
        </p>
        <div className="space-y-3">
          <button
            onClick={() => setStrategy('gradual')}
            className={cn(
              'w-full rounded-lg border-2 p-4 text-left transition-colors',
              strategy === 'gradual'
                ? 'border-primary bg-primary/5'
                : 'border-border hover:border-primary/50',
            )}
          >
            <div className="flex items-center gap-2 mb-1">
              <TrendingUp className="h-4 w-4 text-green-600" />
              <span className="font-medium text-sm">Gradual (recommended)</span>
              <Badge variant="secondary" className="text-xs">Safer</Badge>
            </div>
            <p className="text-xs text-muted-foreground">
              New vendor is set as preferred with old vendor as automatic fallback.
              Workers fall back on any new-vendor error — protecting reputation while you ramp.
              You click <em>Complete</em> when confident.
            </p>
          </button>
          <button
            onClick={() => setStrategy('instant')}
            className={cn(
              'w-full rounded-lg border-2 p-4 text-left transition-colors',
              strategy === 'instant'
                ? 'border-primary bg-primary/5'
                : 'border-border hover:border-primary/50',
            )}
          >
            <div className="flex items-center gap-2 mb-1">
              <Zap className="h-4 w-4 text-yellow-500" />
              <span className="font-medium text-sm">Instant</span>
            </div>
            <p className="text-xs text-muted-foreground">
              All traffic switches immediately. Old config is snapshotted in the migration
              record and can be restored with one-click <em>Rollback</em>.
            </p>
          </button>
        </div>
      </div>
    )
  }

  function StepConfirm() {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border bg-muted/30 p-4 space-y-2 text-sm">
          <Row label="Channel" value={channel.toUpperCase()} />
          <Row label="From" value={vendors.find(v => v.id === fromVendor)?.label ?? fromVendor} />
          <Row label="To" value={vendors.find(v => v.id === toVendor)?.label ?? toVendor} />
          <Row label="Type" value={isSameVendor ? 'Config swap (same vendor)' : 'Cross-vendor migration'} />
          <Row label="Strategy" value={strategy === 'gradual' ? 'Gradual (backup routing)' : 'Instant (hard cutover)'} />
        </div>
        <div className="flex items-start gap-2 rounded-md bg-green-50 dark:bg-green-950/30 border border-green-200 dark:border-green-800 p-3 text-xs text-green-700 dark:text-green-300">
          <ShieldCheck className="h-4 w-4 mt-0.5 flex-shrink-0" />
          <span>
            Old credentials are snapshotted for rollback. Post-migration metrics can be
            viewed with the <strong>Since migration</strong> filter on the vendor dashboard.
          </span>
        </div>
        {apiError && (
          <p className="text-xs text-destructive">{apiError}</p>
        )}
      </div>
    )
  }

  function Row({ label, value }: { label: string; value: string }) {
    return (
      <div className="flex justify-between gap-4">
        <span className="text-muted-foreground">{label}</span>
        <span className="font-medium text-right">{value}</span>
      </div>
    )
  }

  // ── Navigation guards ────────────────────────────────────────────────────

  function canAdvance(): boolean {
    switch (step) {
      case 'channel':  return true
      case 'vendors':  return fromVendor !== '' && toVendor !== ''
      case 'config':   return configJson.trim() !== ''
      case 'strategy': return true
      case 'confirm':  return true
    }
  }

  // ── Render ────────────────────────────────────────────────────────────────

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger asChild>
        {children ?? (
          <Button variant="outline" size="sm">
            <ArrowRight className="h-4 w-4 mr-1" />
            Migrate Vendor
          </Button>
        )}
      </DialogTrigger>

      <DialogContent className="sm:max-w-[520px]">
        <DialogHeader>
          <DialogTitle>Migrate Vendor</DialogTitle>
          <DialogDescription>
            Swap providers or update credentials without disrupting sender reputation.
          </DialogDescription>
        </DialogHeader>

        {/* Step indicator */}
        <div className="flex gap-1 mb-2">
          {STEPS.map((s, i) => (
            <div
              key={s}
              className={cn(
                'flex-1 h-1 rounded-full transition-colors',
                i <= currentIdx ? 'bg-primary' : 'bg-muted',
              )}
            />
          ))}
        </div>
        <p className="text-xs text-muted-foreground mb-4">{STEP_LABELS[step]}</p>

        {/* Step body */}
        {step === 'channel'  && <StepChannel />}
        {step === 'vendors'  && <StepVendors />}
        {step === 'config'   && <StepConfig />}
        {step === 'strategy' && <StepStrategy />}
        {step === 'confirm'  && <StepConfirm />}

        <DialogFooter className="mt-4 gap-2">
          {currentIdx > 0 && (
            <Button variant="outline" onClick={back} disabled={submitting}>
              Back
            </Button>
          )}
          {step !== 'confirm' ? (
            <Button onClick={next} disabled={!canAdvance()}>
              Next
            </Button>
          ) : (
            <Button onClick={handleSubmit} disabled={submitting}>
              {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Start Migration
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
