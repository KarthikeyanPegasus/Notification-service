'use client'

import React, { useState, useEffect, useRef, useMemo } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { PageHeader } from '@/components/shared/page-header'
import { Card, CardContent, CardDescription, CardHeader, CardTitle, CardFooter } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import { Badge } from '@/components/ui/badge'
import {
  getVendorConfigsScoped,
  updateVendorConfigScoped,
  sendVendorTest,
  deleteVendorConfigScoped,
  getAuthUser,
} from '@/lib/api'
import type { VendorConfig } from '@/types'
import { ClientScopeSelect } from '@/components/shared/client-scope-select'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger, DialogFooter } from '@/components/ui/dialog'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Mail, MessageSquare, Bell, Save, Loader2, ShieldCheck, AlertCircle, Database, Plus, Upload, FileJson, CheckCircle2, MessageCircle, X, Trash2, Eye, EyeOff, Send } from 'lucide-react'
import { TestDeliveryForm } from '@/components/settings/test-delivery-form'
import { VendorRateLimits } from '@/components/settings/vendor-rate-limits'
import { cn } from '@/lib/utils'

const SOCIAL_VENDORS: { id: string; title: string; description: string; field: 'webhook' | 'api_key' }[] = [
  { id: 'slack', title: 'Slack', description: 'Configure one or more named Slack channels (Incoming Webhooks).', field: 'webhook' },
  { id: 'discord', title: 'Discord', description: 'Discord channel webhook URL.', field: 'webhook' },
  { id: 'teams', title: 'Microsoft Teams', description: 'Teams incoming webhook URL.', field: 'webhook' },
  { id: 'telegram', title: 'Telegram', description: 'Telegram Bot API (bot_token + chat_id).', field: 'api_key' },
]

export default function SettingsPage() {
  const queryClient = useQueryClient()
  const role = getAuthUser()?.role
  const canRemoveVendor = role === 'admin' || role === 'manager'
  const canEditRateLimits = role === 'admin' || role === 'manager' || role === 'dev'
  const canDeleteRateLimits = role === 'admin'

  const DEFAULT_SMS_ROUTING = {
    mode: 'backup' as 'backup' | 'round_robin' | 'publish_all' | 'only',
    prefer: 'twilio',
    fallback: 'plivo',
    only: 'twilio',
    participants: ['twilio', 'plivo', 'vonage', 'messagebird'] as string[],
    error_rate_threshold: 0,
    min_requests: 20,
  }

  const DEFAULT_EMAIL_ROUTING = {
    mode: 'backup' as 'backup' | 'round_robin' | 'publish_all' | 'only',
    prefer: 'ses',
    fallback: 'smtp',
    only: 'ses',
    participants: ['ses', 'smtp', 'mailgun', 'sendgrid', 'postmark'] as string[],
    error_rate_threshold: 0,
    min_requests: 20,
  }

  const DEFAULT_PUSH_ROUTING = {
    mode: 'backup' as 'backup' | 'round_robin' | 'publish_all' | 'only',
    prefer: 'fcm',
    fallback: 'fcm',
    only: 'fcm',
    participants: ['fcm', 'onesignal', 'pusher'] as string[],
    error_rate_threshold: 0,
    min_requests: 20,
  }
  const [activeTab, setActiveTab] = useState<'sms' | 'email' | 'push' | 'social' | 'store'>('sms')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [newVendorType, setNewVendorType] = useState('')
  const [newVendorJson, setNewVendorJson] = useState('{\n  \n}')
  const [isDialogOpen, setIsDialogOpen] = useState(false)
  const [apiKeyId, setApiKeyId] = useState<string | undefined>(undefined)
  const [expandedVendor, setExpandedVendor] = useState<string | null>(null)

  // Local state for forms
  const [smsConfig, setSmsConfig] = useState({
    primary: 'twilio',
    twilio: { account_sid: '', auth_token: '', from_number: '' },
    plivo: { auth_id: '', auth_token: '', from_number: '' },
    vonage: { api_key: '', api_secret: '', from: '' },
    messagebird: { access_key: '', originator: '' },
  })

  const [smsRouting, setSmsRouting] = useState(DEFAULT_SMS_ROUTING)

  const [emailConfig, setEmailConfig] = useState({
    primary: 'smtp',
    ses: { region: '', access_key_id: '', access_secret: '', secret_access_key: '', from_address: '', from_name: '', smtp_username: '', smtp_password: '' },
    smtp: { host: '', port: 587, username: '', password: '', from: '' },
    mailgun: { domain: '', api_key: '', from: '' },
    sendgrid: { api_key: '', from_email: '', from_name: '' },
    postmark: { server_token: '', from_email: '', from_name: '' },
  })

  const [sesAuthMode, setSesAuthMode] = useState<'smtp' | 'keys'>('smtp')
  const sesWebhookEndpoint = useMemo(() => {
    if (typeof window === 'undefined') return '/v1/webhooks/ses'
    return `${window.location.origin}/v1/webhooks/ses`
  }, [])

  const [emailRouting, setEmailRouting] = useState(DEFAULT_EMAIL_ROUTING)

  const [pushRouting, setPushRouting] = useState(DEFAULT_PUSH_ROUTING)

  type FcmConfig = {
    server_key?: string
    service_account?: Record<string, any> | null
    [key: string]: any
  }

  const [fcmConfig, setFcmConfig] = useState<FcmConfig>({
    server_key: '',
    service_account: null,
  })

  const [oneSignalConfig, setOneSignalConfig] = useState({ app_id: '', rest_api_key: '' })
  const [pusherConfig, setPusherConfig] = useState({ instance_id: '', secret_key: '' })

  const [socialConfigs, setSocialConfigs] = useState<Record<string, Record<string, any>>>({})

  const { data: configsData, isLoading: queryLoading, error: queryError } = useQuery({
    queryKey: ['vendor-configs', apiKeyId ?? 'global'],
    queryFn: () => getVendorConfigsScoped(apiKeyId),
    retry: 1,
  })

  const configs = useMemo(() => configsData || [], [configsData])
  const orchestrationMode = useMemo(() => 
    configs.find(c => c.vendor_type === 'workflow_orchestration')?.config_json?.provider || 'temporal', 
    [configs]
  )
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

  const loading = queryLoading

  // Ensure routing picks only from configured vendors.
  useEffect(() => {
    if (configuredSmsVendors.length === 0) return
    setSmsRouting((prev) => {
      const prefer = configuredSmsVendors.includes(prev.prefer as any) ? prev.prefer : configuredSmsVendors[0]
      const fallback = configuredSmsVendors.includes(prev.fallback as any) ? prev.fallback : configuredSmsVendors[0]
      const only = configuredSmsVendors.includes(prev.only as any) ? prev.only : configuredSmsVendors[0]
      const participants = (Array.isArray(prev.participants) ? prev.participants : [])
        .filter((p) => configuredSmsVendors.includes(p as any))
      const nextParticipants = participants.length > 0 ? participants : [...configuredSmsVendors]
      return { ...prev, prefer, fallback, only, participants: nextParticipants }
    })
  }, [configuredSmsVendors])

  useEffect(() => {
    if (configuredEmailVendors.length === 0) return
    setEmailRouting((prev) => {
      const prefer = configuredEmailVendors.includes(prev.prefer as any) ? prev.prefer : configuredEmailVendors[0]
      const fallback = configuredEmailVendors.includes(prev.fallback as any) ? prev.fallback : configuredEmailVendors[0]
      const only = configuredEmailVendors.includes(prev.only as any) ? prev.only : configuredEmailVendors[0]
      const participants = (Array.isArray(prev.participants) ? prev.participants : [])
        .filter((p) => configuredEmailVendors.includes(p as any))
      const nextParticipants = participants.length > 0 ? participants : [...configuredEmailVendors]
      return { ...prev, prefer, fallback, only, participants: nextParticipants }
    })
  }, [configuredEmailVendors])

  useEffect(() => {
    if (configs.length > 0) {
      // Populate form state from DB configs
      configs.forEach(cfg => {
        if (cfg.vendor_type === 'sms') setSmsConfig(prev => ({ ...prev, ...cfg.config_json }))
        if (cfg.vendor_type === 'twilio') setSmsConfig(prev => ({ ...prev, twilio: cfg.config_json }))
        if (cfg.vendor_type === 'plivo') setSmsConfig(prev => ({ ...prev, plivo: cfg.config_json }))
        if (cfg.vendor_type === 'vonage') setSmsConfig(prev => ({ ...prev, vonage: cfg.config_json }))
        if (cfg.vendor_type === 'messagebird') setSmsConfig(prev => ({ ...prev, messagebird: cfg.config_json }))
        if (cfg.vendor_type === 'sms_routing') setSmsRouting(prev => ({ ...prev, ...cfg.config_json }))
        
        if (cfg.vendor_type === 'email') setEmailConfig(prev => ({ ...prev, ...cfg.config_json }))
        if (cfg.vendor_type === 'ses') {
          const ses = cfg.config_json as any
          setEmailConfig(prev => ({
            ...prev,
            ses: {
              ...prev.ses,
              ...ses,
              access_secret: String(ses?.access_secret ?? ''),
              secret_access_key: String(ses?.secret_access_key ?? ''),
            },
          }))
          const hasSMTP = Boolean(String(ses?.smtp_username ?? '').trim() || String(ses?.smtp_password ?? '').trim())
          const hasKeys = Boolean(String(ses?.access_key_id ?? '').trim() || String(ses?.access_secret ?? ses?.secret_access_key ?? '').trim())
          setSesAuthMode(hasSMTP ? 'smtp' : hasKeys ? 'keys' : 'smtp')
        }
        if (cfg.vendor_type === 'mailgun') setEmailConfig(prev => ({ ...prev, mailgun: { ...prev.mailgun, ...(cfg.config_json as any) } }))
        if (cfg.vendor_type === 'sendgrid') setEmailConfig(prev => ({ ...prev, sendgrid: { ...prev.sendgrid, ...(cfg.config_json as any) } }))
        if (cfg.vendor_type === 'postmark') setEmailConfig(prev => ({ ...prev, postmark: { ...prev.postmark, ...(cfg.config_json as any) } }))

        if (cfg.vendor_type === 'email_routing') setEmailRouting(prev => ({ ...prev, ...cfg.config_json }))

        if (cfg.vendor_type === 'push_routing') setPushRouting(prev => ({ ...prev, ...cfg.config_json }))
        if (cfg.vendor_type === 'fcm') setFcmConfig(prev => ({ ...prev, ...cfg.config_json }))
        if (cfg.vendor_type === 'onesignal') setOneSignalConfig(prev => ({ ...prev, ...(cfg.config_json as any) }))
        if (cfg.vendor_type === 'pusher') setPusherConfig(prev => ({ ...prev, ...(cfg.config_json as any) }))
        if (SOCIAL_VENDORS.some((s) => s.id === cfg.vendor_type)) {
          setSocialConfigs((prev) => ({
            ...prev,
            [cfg.vendor_type]: { ...(cfg.config_json as Record<string, any>) },
          }))
        }
      })
    }
  }, [configs])

  // Map query status to local state for compatibility with existing UI
  useEffect(() => {
    if (queryError) {
      console.error(queryError)
      setError('Failed to load settings from server.')
    }
  }, [queryLoading, queryError])

  const loadConfigs = async () => {
    // No-op now as useQuery handles it, but kept to avoid breaking handleSave calls
    // In a full refactor we would invalidate the query instead
  }

  const handleFcmFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = async (event) => {
      try {
        const json = JSON.parse(event.target?.result as string)
        const next = { ...fcmConfig, service_account: json }
        setFcmConfig(next)
        await handleSave('fcm', next)
      } catch (err) {
        setError('Invalid JSON file.')
        setTimeout(() => setError(null), 3000)
      } finally {
        // allow re-uploading same file
        e.target.value = ''
      }
    }
    reader.readAsText(file)
  }

  const handleSave = async (type: string, config: any) => {
    try {
      setSaving(true)
      setError(null)
      setSuccess(null)
      await updateVendorConfigScoped(type, config, apiKeyId)
      setSuccess(`${type.toUpperCase()} configuration updated successfully.`)
      queryClient.invalidateQueries({ queryKey: ['vendor-configs'] })
      setTimeout(() => setSuccess(null), 3000)
      if (activeTab === 'store') {
        setIsDialogOpen(false)
        setNewVendorType('')
        setNewVendorJson('{\n  \n}')
      }
      if (activeTab === 'social' && SOCIAL_VENDORS.some((s) => s.id === type)) {
        queryClient.invalidateQueries({ queryKey: ['vendor-configs'] })
      }
    } catch (err) {
      setError(`Failed to update ${type} settings.`)
    } finally {
      setSaving(false)
    }
  }

  const handleRemoveVendor = async (type: string) => {
    if (!canRemoveVendor) return
    const ok = window.confirm(`Remove ${type.toUpperCase()} configuration for the current scope?`)
    if (!ok) return
    try {
      setSaving(true)
      setError(null)
      setSuccess(null)
      await deleteVendorConfigScoped(type, apiKeyId)
      setSuccess(`${type.toUpperCase()} configuration removed.`)
      setTimeout(() => setSuccess(null), 3000)
      setExpandedVendor(null)
      // Reset local state for routing configs so the UI reflects removal immediately.
      if (type === 'sms_routing') setSmsRouting(DEFAULT_SMS_ROUTING)
      if (type === 'email_routing') setEmailRouting(DEFAULT_EMAIL_ROUTING)
      if (type === 'push_routing') setPushRouting(DEFAULT_PUSH_ROUTING)
      queryClient.invalidateQueries({ queryKey: ['vendor-configs'] })
    } catch (e) {
      setError(`Failed to remove ${type} settings.`)
    } finally {
      setSaving(false)
    }
  }


  return (
    <div className="flex flex-col gap-6 p-6 max-w-5xl mx-auto">
      <PageHeader 
        title="Settings" 
        description="Manage your notification delivery providers in real-time." 
      />

      <div className="flex items-center justify-between gap-3 rounded-lg border bg-card p-4">
        <div>
          <p className="text-sm font-medium">Configuration scope</p>
          <p className="text-xs text-muted-foreground">
            Choose whether you’re editing global vendor credentials or a specific client’s (API key) credentials.
          </p>
        </div>
        <div className="min-w-[260px]">
          <ClientScopeSelect className="w-full" onScopeChange={setApiKeyId} />
        </div>
      </div>

      {error && (
        <div className="flex items-center gap-2 p-4 text-sm text-destructive bg-destructive/10 rounded-lg border border-destructive/20">
          <AlertCircle className="h-4 w-4" />
          {error}
        </div>
      )}

      {success && (
        <div className="flex items-center gap-2 p-4 text-sm text-green-600 bg-green-50 rounded-lg border border-green-200">
          <ShieldCheck className="h-4 w-4" />
          {success}
        </div>
      )}

      <div className="flex gap-4 border-b">
        <button
          onClick={() => setActiveTab('sms')}
          className={cn(
            "pb-3 px-2 text-sm font-medium transition-colors border-b-2",
            activeTab === 'sms' ? "border-primary text-primary" : "border-transparent text-muted-foreground hover:text-foreground"
          )}
        >
          <div className="flex items-center gap-2">
            <MessageSquare className="h-4 w-4" />
            SMS Providers
          </div>
        </button>
        <button
          onClick={() => setActiveTab('email')}
          className={cn(
            "pb-3 px-2 text-sm font-medium transition-colors border-b-2",
            activeTab === 'email' ? "border-primary text-primary" : "border-transparent text-muted-foreground hover:text-foreground"
          )}
        >
          <div className="flex items-center gap-2">
            <Mail className="h-4 w-4" />
            Email Providers
          </div>
        </button>
        <button
          onClick={() => setActiveTab('push')}
          className={cn(
            "pb-3 px-2 text-sm font-medium transition-colors border-b-2",
            activeTab === 'push' ? "border-primary text-primary" : "border-transparent text-muted-foreground hover:text-foreground"
          )}
        >
          <div className="flex items-center gap-2">
            <Bell className="h-4 w-4" />
            Push Notifications
          </div>
        </button>
        <button
          onClick={() => setActiveTab('social')}
          className={cn(
            "pb-3 px-2 text-sm font-medium transition-colors border-b-2",
            activeTab === 'social' ? "border-primary text-primary" : "border-transparent text-muted-foreground hover:text-foreground"
          )}
        >
          <div className="flex items-center gap-2">
            <MessageCircle className="h-4 w-4" />
            Social
          </div>
        </button>
        <button
          onClick={() => setActiveTab('store')}
          className={cn(
            "pb-3 px-2 text-sm font-medium transition-colors border-b-2",
            activeTab === 'store' ? "border-primary text-primary" : "border-transparent text-muted-foreground hover:text-foreground"
          )}
        >
          <div className="flex items-center gap-2">
            <Database className="h-4 w-4" />
            Config Store
          </div>
        </button>
      </div>

      <div className="mt-4">
        {loading ? (
          <div className="flex h-64 items-center justify-center">
            <Loader2 className="h-8 w-8 animate-spin text-primary" />
          </div>
        ) : (
          <>
            {activeTab === 'sms' && (
              <div className="grid gap-6">
                <Card>
                  <CardHeader>
                    <CardTitle className="text-lg flex items-center gap-2">Delivery Preference</CardTitle>
                    <CardDescription>Choose how the SMS worker selects vendors when multiple are configured.</CardDescription>
                  </CardHeader>
                <CardContent className="space-y-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Routing Mode</label>
                      <Select value={smsRouting.mode} onValueChange={(v) => setSmsRouting({ ...smsRouting, mode: v as any })}>
                        <SelectTrigger><SelectValue placeholder="Select routing mode" /></SelectTrigger>
                        <SelectContent>
                          <SelectItem value="backup">Backup (prefer then fallback)</SelectItem>
                          <SelectItem value="round_robin">Round robin</SelectItem>
                          <SelectItem value="publish_all">Publish all vendors</SelectItem>
                          <SelectItem value="only">Only one vendor</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    {smsRouting.mode !== 'publish_all' ? (
                      <div className="space-y-2">
                        <label className="text-xs font-semibold text-muted-foreground uppercase">Prefer Vendor</label>
                        <Select value={smsRouting.prefer} onValueChange={(v) => setSmsRouting({ ...smsRouting, prefer: v as any })}>
                          <SelectTrigger><SelectValue placeholder="Select preferred vendor" /></SelectTrigger>
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

                  {smsRouting.mode === 'backup' && (
                    <div className="grid grid-cols-2 gap-4">
                      <div className="space-y-2">
                        <label className="text-xs font-semibold text-muted-foreground uppercase">Fallback Vendor</label>
                        <Select value={smsRouting.fallback} onValueChange={(v) => setSmsRouting({ ...smsRouting, fallback: v as any })}>
                          <SelectTrigger><SelectValue placeholder="Select fallback vendor" /></SelectTrigger>
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

                  {smsRouting.mode === 'round_robin' && (
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Round Robin Participants</label>
                      <div className="grid grid-cols-3 gap-3">
                        {configuredSmsVendors.map((v) => {
                          const checked = smsRouting.participants.includes(v)
                          return (
                            <label key={v} className="flex items-center gap-2 text-sm">
                              <input
                                type="checkbox"
                                className="h-4 w-4"
                                checked={checked}
                                onChange={(e) => {
                                  const next = e.target.checked
                                    ? Array.from(new Set([...smsRouting.participants, v]))
                                    : smsRouting.participants.filter((x) => x !== v)
                                  setSmsRouting({ ...smsRouting, participants: next as any })
                                }}
                              />
                              {SMS_VENDOR_LABELS[v] ?? v}
                            </label>
                          )
                        })}
                      </div>
                      <p className="text-xs text-muted-foreground">At least one participant must be selected.</p>
                    </div>
                  )}

                  {smsRouting.mode === 'only' && (
                    <div className="grid grid-cols-2 gap-4">
                      <div className="space-y-2">
                        <label className="text-xs font-semibold text-muted-foreground uppercase">Only Vendor</label>
                        <Select value={smsRouting.only} onValueChange={(v) => setSmsRouting({ ...smsRouting, only: v as any })}>
                          <SelectTrigger><SelectValue placeholder="Select only vendor" /></SelectTrigger>
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

                  {smsRouting.mode === 'backup' && (
                    <div className="grid grid-cols-2 gap-4">
                      <div className="space-y-2">
                        <label className="text-xs font-semibold text-muted-foreground uppercase">Error-rate fallback threshold</label>
                        <Input
                          type="number"
                          step="0.01"
                          min="0"
                          max="1"
                          value={String(smsRouting.error_rate_threshold ?? 0)}
                          onChange={(e) =>
                            setSmsRouting({
                              ...smsRouting,
                              error_rate_threshold: Number(e.target.value),
                            })
                          }
                          placeholder="0.25"
                        />
                        <p className="text-xs text-muted-foreground">
                          Set to <code className="text-xs">0</code> to disable. When the preferred vendor’s recent error rate exceeds this
                          value, the worker will try fallback first.
                        </p>
                      </div>
                      <div className="space-y-2">
                        <label className="text-xs font-semibold text-muted-foreground uppercase">Min requests</label>
                        <Input
                          type="number"
                          min="1"
                          value={String(smsRouting.min_requests ?? 20)}
                          onChange={(e) =>
                            setSmsRouting({
                              ...smsRouting,
                              min_requests: Number(e.target.value),
                            })
                          }
                          placeholder="20"
                        />
                        <p className="text-xs text-muted-foreground">
                          Minimum recent attempts before applying error-rate based fallback.
                        </p>
                      </div>
                    </div>
                  )}
                </CardContent>
                <CardFooter className="bg-muted/50 py-3 flex justify-between">
                  <p className="text-xs text-muted-foreground italic">This controls vendor selection in the worker.</p>
                    <div className="flex items-center gap-2">
                      <Button disabled={saving} onClick={() => handleSave('sms_routing', smsRouting)}>
                        {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                        Save Preference
                      </Button>
                    </div>
                </CardFooter>
              </Card>

              {canEditRateLimits && (
                <VendorRateLimits
                  apiKeyId={apiKeyId}
                  vendorIds={configuredSmsVendors}
                  canEdit={canEditRateLimits}
                  canDelete={canDeleteRateLimits}
                />
              )}

            {configs.some(c => c.vendor_type === 'twilio') && (
              <Card>
                <CardHeader>
                  <CardTitle className="text-lg flex items-center gap-2">
                    <Badge variant="outline" className="text-xs">Primary</Badge>
                    Twilio Configuration
                  </CardTitle>
                  <CardDescription>Direct SMS delivery via Twilio API.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Account SID</label>
                      <Input 
                        value={smsConfig.twilio.account_sid} 
                        onChange={e => setSmsConfig({...smsConfig, twilio: {...smsConfig.twilio, account_sid: e.target.value}})}
                        placeholder="ACxxxxxxxxxxxxxxxx"
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Auth Token</label>
                      <Input 
                        type="password"
                        value={smsConfig.twilio.auth_token} 
                        onChange={e => setSmsConfig({...smsConfig, twilio: {...smsConfig.twilio, auth_token: e.target.value}})}
                        placeholder="••••••••••••••••"
                      />
                    </div>
                  </div>
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase">From Number</label>
                    <Input 
                      value={smsConfig.twilio.from_number} 
                      onChange={e => setSmsConfig({...smsConfig, twilio: {...smsConfig.twilio, from_number: e.target.value}})}
                      placeholder="+1234567890"
                    />
                  </div>
                </CardContent>
                <CardFooter className="bg-muted/50 py-3 flex justify-between">
                  <p className="text-xs text-muted-foreground italic">Current settings override config.yaml</p>
                  <div className="flex items-center gap-2">
                    <Button 
                      variant="outline" 
                      onClick={() => setExpandedVendor(expandedVendor === 'twilio' ? null : 'twilio')}
                      className={expandedVendor === 'twilio' ? 'bg-white/10' : ''}
                    >
                      {expandedVendor === 'twilio' ? 'Cancel Test' : 'Send Test'}
                    </Button>
                    {canRemoveVendor && (
                      <Button
                        variant="destructive"
                        disabled={saving}
                        onClick={() => handleRemoveVendor('twilio')}
                      >
                        <Trash2 className="h-4 w-4 mr-2" />
                        Remove
                      </Button>
                    )}
                    <Button disabled={saving} onClick={() => handleSave('twilio', smsConfig.twilio)}>
                      {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                      Save Twilio Settings
                    </Button>
                  </div>
                </CardFooter>
                {expandedVendor === 'twilio' && (
                  <div className="p-4 border-t border-white/5">
                    <TestDeliveryForm vendorType="twilio" channel="sms" orchestrationMode={orchestrationMode} apiKeyId={apiKeyId} />
                  </div>
                )}
              </Card>
            )}

            {configs.some(c => c.vendor_type === 'plivo') && (
              <Card>
                <CardHeader>
                  <CardTitle className="text-lg flex items-center gap-2">
                    Plivo Configuration
                  </CardTitle>
                  <CardDescription>SMS and Voice delivery via Plivo.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Auth ID</label>
                      <Input 
                        value={smsConfig.plivo?.auth_id || ''} 
                        onChange={e => setSmsConfig({...smsConfig, plivo: {...smsConfig.plivo, auth_id: e.target.value}})}
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Auth Token</label>
                      <Input 
                        type="password"
                        value={smsConfig.plivo?.auth_token || ''} 
                        onChange={e => setSmsConfig({...smsConfig, plivo: {...smsConfig.plivo, auth_token: e.target.value}})}
                      />
                    </div>
                  </div>
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase">From Number</label>
                    <Input 
                      value={smsConfig.plivo?.from_number || ''} 
                      onChange={e => setSmsConfig({...smsConfig, plivo: {...smsConfig.plivo, from_number: e.target.value}})}
                    />
                  </div>
                </CardContent>
                <CardFooter className="bg-muted/50 py-3 flex justify-end">
                  <div className="flex items-center gap-2">
                    <Button 
                      variant="outline" 
                      onClick={() => setExpandedVendor(expandedVendor === 'plivo' ? null : 'plivo')}
                      className={expandedVendor === 'plivo' ? 'bg-white/10' : ''}
                    >
                      {expandedVendor === 'plivo' ? 'Cancel Test' : 'Send Test'}
                    </Button>
                    {canRemoveVendor && (
                      <Button
                        variant="destructive"
                        disabled={saving}
                        onClick={() => handleRemoveVendor('plivo')}
                      >
                        <Trash2 className="h-4 w-4 mr-2" />
                        Remove
                      </Button>
                    )}
                    <Button disabled={saving} onClick={() => handleSave('plivo', smsConfig.plivo)}>
                      {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                      Save Plivo Settings
                    </Button>
                  </div>
                </CardFooter>
                {expandedVendor === 'plivo' && (
                  <div className="p-4 border-t border-white/5">
                    <TestDeliveryForm vendorType="plivo" channel="sms" orchestrationMode={orchestrationMode} apiKeyId={apiKeyId} />
                  </div>
                )}
              </Card>
            )}

            {configs.some(c => c.vendor_type === 'vonage') && (
              <Card>
                <CardHeader>
                  <CardTitle className="text-lg flex items-center gap-2">
                    Vonage Configuration
                  </CardTitle>
                  <CardDescription>Transactional SMS delivery via Vonage.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">API Key</label>
                      <Input 
                        value={smsConfig.vonage?.api_key || ''} 
                        onChange={e => setSmsConfig({...smsConfig, vonage: {...smsConfig.vonage, api_key: e.target.value}})}
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">API Secret</label>
                      <Input 
                        type="password"
                        value={smsConfig.vonage?.api_secret || ''} 
                        onChange={e => setSmsConfig({...smsConfig, vonage: {...smsConfig.vonage, api_secret: e.target.value}})}
                      />
                    </div>
                  </div>
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase">From</label>
                    <Input 
                      value={smsConfig.vonage?.from || ''} 
                      onChange={e => setSmsConfig({...smsConfig, vonage: {...smsConfig.vonage, from: e.target.value}})}
                    />
                  </div>
                </CardContent>
                <CardFooter className="bg-muted/50 py-3 flex justify-end">
                  <div className="flex items-center gap-2">
                    <Button 
                      variant="outline" 
                      onClick={() => setExpandedVendor(expandedVendor === 'vonage' ? null : 'vonage')}
                      className={expandedVendor === 'vonage' ? 'bg-white/10' : ''}
                    >
                      {expandedVendor === 'vonage' ? 'Cancel Test' : 'Send Test'}
                    </Button>
                    {canRemoveVendor && (
                      <Button
                        variant="destructive"
                        disabled={saving}
                        onClick={() => handleRemoveVendor('vonage')}
                      >
                        <Trash2 className="h-4 w-4 mr-2" />
                        Remove
                      </Button>
                    )}
                    <Button disabled={saving} onClick={() => handleSave('vonage', smsConfig.vonage)}>
                      {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                      Save Vonage Settings
                    </Button>
                  </div>
                </CardFooter>
                {expandedVendor === 'vonage' && (
                  <div className="p-4 border-t border-white/5">
                    <TestDeliveryForm vendorType="vonage" channel="sms" orchestrationMode={orchestrationMode} apiKeyId={apiKeyId} />
                  </div>
                )}
              </Card>
            )}

            {configs.some((c) => c.vendor_type === 'messagebird') && (
              <Card>
                <CardHeader>
                  <CardTitle className="text-lg flex items-center gap-2">MessageBird Configuration</CardTitle>
                  <CardDescription>SMS delivery via MessageBird.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Access Key</label>
                      <Input
                        type="password"
                        value={smsConfig.messagebird?.access_key || ''}
                        onChange={(e) => setSmsConfig({ ...smsConfig, messagebird: { ...smsConfig.messagebird, access_key: e.target.value } })}
                        placeholder="live_xxxxxxxxxxxxxxxxx"
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Originator (From)</label>
                      <Input
                        value={smsConfig.messagebird?.originator || ''}
                        onChange={(e) => setSmsConfig({ ...smsConfig, messagebird: { ...smsConfig.messagebird, originator: e.target.value } })}
                        placeholder="YourBrand or +1234567890"
                      />
                    </div>
                  </div>
                </CardContent>
                <CardFooter className="bg-muted/50 py-3 flex justify-end">
                  <div className="flex items-center gap-2">
                    <Button
                      variant="outline"
                      onClick={() => setExpandedVendor(expandedVendor === 'messagebird' ? null : 'messagebird')}
                      className={expandedVendor === 'messagebird' ? 'bg-white/10' : ''}
                    >
                      {expandedVendor === 'messagebird' ? 'Cancel Test' : 'Send Test'}
                    </Button>
                    {canRemoveVendor && (
                      <Button
                        variant="destructive"
                        disabled={saving}
                        onClick={() => handleRemoveVendor('messagebird')}
                      >
                        <Trash2 className="h-4 w-4 mr-2" />
                        Remove
                      </Button>
                    )}
                    <Button disabled={saving} onClick={() => handleSave('messagebird', smsConfig.messagebird)}>
                      {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                      Save MessageBird Settings
                    </Button>
                  </div>
                </CardFooter>
                {expandedVendor === 'messagebird' && (
                  <div className="p-4 border-t border-white/5">
                    <TestDeliveryForm vendorType="messagebird" channel="sms" orchestrationMode={orchestrationMode} apiKeyId={apiKeyId} />
                  </div>
                )}
              </Card>
            )}

            {!configs.some(c => ['twilio', 'plivo', 'vonage', 'messagebird'].includes(c.vendor_type)) && (
              <div className="flex flex-col items-center justify-center py-20 bg-muted/20 border border-dashed rounded-xl">
                 <AlertCircle className="h-8 w-8 text-muted-foreground/40 mb-3" />
                 <p className="text-sm text-muted-foreground font-medium">No SMS providers connected.</p>
                 <Button variant="link" size="sm" className="mt-2" onClick={() => (window as any).location.href = '/app-store'}>Connect a provider in the App Store</Button>
              </div>
            )}
          </div>
        )}

        {activeTab === 'email' && (
          <div className="grid gap-6">
            <Card>
              <CardHeader>
                <CardTitle className="text-lg flex items-center gap-2">
                  Delivery Preference
                </CardTitle>
                  <CardDescription>Choose how the email worker selects vendors when multiple are configured.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Routing Mode</label>
                      <Select value={emailRouting.mode} onValueChange={(v) => setEmailRouting({ ...emailRouting, mode: v as any })}>
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
                    {emailRouting.mode !== 'publish_all' ? (
                      <div className="space-y-2">
                        <label className="text-xs font-semibold text-muted-foreground uppercase">Prefer Vendor</label>
                        <Select value={emailRouting.prefer} onValueChange={(v) => setEmailRouting({ ...emailRouting, prefer: v as any })}>
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

                  {emailRouting.mode === 'backup' && (
                    <div className="grid grid-cols-2 gap-4">
                      <div className="space-y-2">
                        <label className="text-xs font-semibold text-muted-foreground uppercase">Fallback Vendor</label>
                        <Select value={emailRouting.fallback} onValueChange={(v) => setEmailRouting({ ...emailRouting, fallback: v as any })}>
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

                  {emailRouting.mode === 'round_robin' && (
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Round Robin Participants</label>
                      <div className="grid grid-cols-3 gap-3">
                        {configuredEmailVendors.map((v) => {
                          const checked = emailRouting.participants.includes(v)
                          return (
                            <label key={v} className="flex items-center gap-2 text-sm">
                              <input
                                type="checkbox"
                                className="h-4 w-4"
                                checked={checked}
                                onChange={(e) => {
                                  const next = e.target.checked
                                    ? Array.from(new Set([...emailRouting.participants, v]))
                                    : emailRouting.participants.filter((x) => x !== v)
                                  setEmailRouting({ ...emailRouting, participants: next as any })
                                }}
                              />
                              {EMAIL_VENDOR_LABELS[v] ?? v}
                            </label>
                          )
                        })}
                      </div>
                      <p className="text-xs text-muted-foreground">At least one participant must be selected.</p>
                    </div>
                  )}

                  {emailRouting.mode === 'only' && (
                    <div className="grid grid-cols-2 gap-4">
                      <div className="space-y-2">
                        <label className="text-xs font-semibold text-muted-foreground uppercase">Only Vendor</label>
                        <Select value={emailRouting.only} onValueChange={(v) => setEmailRouting({ ...emailRouting, only: v as any })}>
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

                  {emailRouting.mode === 'backup' && (
                    <div className="grid grid-cols-2 gap-4">
                      <div className="space-y-2">
                        <label className="text-xs font-semibold text-muted-foreground uppercase">Error-rate fallback threshold</label>
                        <Input
                          type="number"
                          step="0.01"
                          min="0"
                          max="1"
                          value={String(emailRouting.error_rate_threshold ?? 0)}
                          onChange={(e) =>
                            setEmailRouting({
                              ...emailRouting,
                              error_rate_threshold: Number(e.target.value),
                            })
                          }
                          placeholder="0.25"
                        />
                        <p className="text-xs text-muted-foreground">
                          Set to <code className="text-xs">0</code> to disable. When the preferred vendor’s recent error rate exceeds this
                          value, the worker will try fallback first.
                        </p>
                      </div>
                      <div className="space-y-2">
                        <label className="text-xs font-semibold text-muted-foreground uppercase">Min requests</label>
                        <Input
                          type="number"
                          min="1"
                          value={String(emailRouting.min_requests ?? 20)}
                          onChange={(e) =>
                            setEmailRouting({
                              ...emailRouting,
                              min_requests: Number(e.target.value),
                            })
                          }
                          placeholder="20"
                        />
                        <p className="text-xs text-muted-foreground">
                          Minimum recent attempts before applying error-rate based fallback.
                        </p>
                      </div>
                    </div>
                  )}
                </CardContent>
                <CardFooter className="bg-muted/50 py-3 flex justify-between">
                  <p className="text-xs text-muted-foreground italic">This controls vendor selection in the worker.</p>
                    <div className="flex items-center gap-2">
                      <Button disabled={saving} onClick={() => handleSave('email_routing', emailRouting)}>
                        {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                        Save Preference
                      </Button>
                    </div>
                </CardFooter>
              </Card>

              {canEditRateLimits && (
                <VendorRateLimits
                  apiKeyId={apiKeyId}
                  vendorIds={configuredEmailVendors}
                  canEdit={canEditRateLimits}
                  canDelete={canDeleteRateLimits}
                />
              )}

            {configs.some(c => c.vendor_type === 'ses') && (
              <Card>
                <CardHeader>
                  <CardTitle className="text-lg flex items-center gap-2">
                    Amazon SES
                  </CardTitle>
                  <CardDescription>AWS Simple Email Service configuration.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Region</label>
                      <Input 
                        value={emailConfig.ses?.region || ''} 
                        onChange={e => setEmailConfig({...emailConfig, ses: {...emailConfig.ses, region: e.target.value}})}
                        placeholder="us-east-1"
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">From Address</label>
                      <Input 
                        value={emailConfig.ses?.from_address || ''} 
                        onChange={e => setEmailConfig({...emailConfig, ses: {...emailConfig.ses, from_address: e.target.value}})}
                      />
                    </div>
                  </div>

                  <div className="rounded-lg border border-white/10 bg-muted/30 p-3 space-y-2">
                    <div className="flex items-start gap-2">
                      <AlertCircle className="h-4 w-4 mt-0.5 text-muted-foreground" />
                      <div className="space-y-1">
                        <p className="text-sm font-medium">Delivery updates for SES come from webhooks (SNS), not status polling.</p>
                        <p className="text-xs text-muted-foreground">
                          Configure AWS to push delivery/bounce/complaint events to this endpoint:
                        </p>
                        <p className="text-xs">
                          <code className="text-xs">{sesWebhookEndpoint}</code>
                        </p>
                      </div>
                    </div>
                    <div className="text-xs text-muted-foreground space-y-1">
                      <p className="font-semibold text-muted-foreground uppercase tracking-wide">AWS setup</p>
                      <ol className="list-decimal pl-4 space-y-1">
                        <li>Create an SNS topic (Standard).</li>
                        <li>
                          Add an SNS subscription with protocol <span className="font-mono">HTTPS</span> and endpoint{' '}
                          <span className="font-mono">{sesWebhookEndpoint}</span>.
                          Subscription confirmation is handled automatically by this service.
                        </li>
                        <li>
                          In SES, create/use a <span className="font-mono">Configuration set</span> and add an{' '}
                          <span className="font-mono">Event destination</span> to that SNS topic for{' '}
                          <span className="font-mono">Delivery</span>, <span className="font-mono">Bounce</span>, and{' '}
                          <span className="font-mono">Complaint</span>.
                        </li>
                      </ol>
                      <p className="pt-1">
                        Important: to have events map back to notifications in this app, use <b>AWS access keys</b> mode (SES API).
                        SMTP sends don’t include the internal <span className="font-mono">notification-id</span> tag used for matching.
                      </p>
                    </div>
                  </div>

                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase">Authentication method</label>
                    <Select value={sesAuthMode} onValueChange={(v) => setSesAuthMode(v as any)}>
                      <SelectTrigger><SelectValue placeholder="Select auth method" /></SelectTrigger>
                      <SelectContent>
                        <SelectItem value="smtp">SMTP credentials (recommended)</SelectItem>
                        <SelectItem value="keys">AWS access keys</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>

                  {sesAuthMode === 'smtp' ? (
                    <div className="grid grid-cols-2 gap-4">
                      <div className="space-y-2">
                        <label className="text-xs font-semibold text-muted-foreground uppercase">SMTP Username</label>
                        <Input 
                          value={emailConfig.ses?.smtp_username || ''} 
                          onChange={e => setEmailConfig({...emailConfig, ses: {...emailConfig.ses, smtp_username: e.target.value}})}
                        />
                      </div>
                      <div className="space-y-2">
                        <label className="text-xs font-semibold text-muted-foreground uppercase">SMTP Password</label>
                        <Input 
                          type="password"
                          value={emailConfig.ses?.smtp_password || ''} 
                          onChange={e => setEmailConfig({...emailConfig, ses: {...emailConfig.ses, smtp_password: e.target.value}})}
                        />
                      </div>
                    </div>
                  ) : (
                    <div className="grid grid-cols-2 gap-4">
                      <div className="space-y-2">
                        <label className="text-xs font-semibold text-muted-foreground uppercase">Access Key ID</label>
                        <Input
                          value={emailConfig.ses?.access_key_id || ''}
                          onChange={(e) => setEmailConfig({ ...emailConfig, ses: { ...emailConfig.ses, access_key_id: e.target.value } })}
                          placeholder="AKIA..."
                          className="font-mono text-sm"
                        />
                      </div>
                      <div className="space-y-2">
                        <label className="text-xs font-semibold text-muted-foreground uppercase">Access Secret</label>
                        <Input
                          type="password"
                          value={String(emailConfig.ses?.access_secret || emailConfig.ses?.secret_access_key || '')}
                          onChange={(e) =>
                            setEmailConfig({
                              ...emailConfig,
                              ses: {
                                ...emailConfig.ses,
                                access_secret: e.target.value,
                                secret_access_key: e.target.value,
                              },
                            })
                          }
                          placeholder="••••••••••••••••"
                        />
                      </div>
                    </div>
                  )}
                </CardContent>
                <CardFooter className="bg-muted/50 py-3 flex justify-end">
                  <div className="flex items-center gap-2">
                    <Button 
                      variant="outline" 
                      onClick={() => setExpandedVendor(expandedVendor === 'ses' ? null : 'ses')}
                      className={expandedVendor === 'ses' ? 'bg-white/10' : ''}
                    >
                      {expandedVendor === 'ses' ? 'Cancel Test' : 'Send Test'}
                    </Button>
                    {canRemoveVendor && (
                      <Button
                        variant="destructive"
                        disabled={saving}
                        onClick={() => handleRemoveVendor('ses')}
                      >
                        <Trash2 className="h-4 w-4 mr-2" />
                        Remove
                      </Button>
                    )}
                    <Button disabled={saving} onClick={() => handleSave('ses', emailConfig.ses)}>
                      {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                      Save SES Settings
                    </Button>
                  </div>
                </CardFooter>
                {expandedVendor === 'ses' && (
                  <div className="p-4 border-t border-white/5">
                    <TestDeliveryForm vendorType="ses" channel="email" orchestrationMode={orchestrationMode} apiKeyId={apiKeyId} />
                  </div>
                )}
              </Card>
            )}

            {configs.some(c => ['email', 'smtp'].includes(c.vendor_type)) && (
              <Card>
                <CardHeader>
                  <CardTitle className="text-lg flex items-center gap-2">
                    SMTP Relay
                  </CardTitle>
                  <CardDescription>Standard SMTP configuration for email delivery.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="grid grid-cols-3 gap-4">
                    <div className="col-span-2 space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">SMTP Host</label>
                      <Input 
                        value={emailConfig.smtp.host} 
                        onChange={e => setEmailConfig({...emailConfig, smtp: {...emailConfig.smtp, host: e.target.value}})}
                        placeholder="smtp.mailtrap.io"
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Port</label>
                      <Input 
                        type="number"
                        value={emailConfig.smtp.port} 
                        onChange={e => setEmailConfig({...emailConfig, smtp: {...emailConfig.smtp, port: parseInt(e.target.value)}})}
                        placeholder="587"
                      />
                    </div>
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Username</label>
                      <Input 
                        value={emailConfig.smtp.username} 
                        onChange={e => setEmailConfig({...emailConfig, smtp: {...emailConfig.smtp, username: e.target.value}})}
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Password</label>
                      <Input 
                        type="password"
                        value={emailConfig.smtp.password} 
                        onChange={e => setEmailConfig({...emailConfig, smtp: {...emailConfig.smtp, password: e.target.value}})}
                      />
                    </div>
                  </div>
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase">From Address</label>
                    <Input 
                      value={emailConfig.smtp.from} 
                      onChange={e => setEmailConfig({...emailConfig, smtp: {...emailConfig.smtp, from: e.target.value}})}
                      placeholder="noreply@notifyhub.io"
                    />
                  </div>
                </CardContent>
                <CardFooter className="bg-muted/50 py-3 flex justify-end">
                  <div className="flex items-center gap-2">
                    <Button 
                      variant="outline" 
                      onClick={() => setExpandedVendor(expandedVendor === 'email' ? null : 'email')}
                      className={expandedVendor === 'email' ? 'bg-white/10' : ''}
                    >
                      {expandedVendor === 'email' ? 'Cancel Test' : 'Send Test'}
                    </Button>
                    {canRemoveVendor && (
                      <Button
                        variant="destructive"
                        disabled={saving}
                        onClick={() => handleRemoveVendor('email')}
                      >
                        <Trash2 className="h-4 w-4 mr-2" />
                        Remove
                      </Button>
                    )}
                    <Button disabled={saving} onClick={() => handleSave('email', emailConfig)}>
                      {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                      Save SMTP Settings
                    </Button>
                  </div>
                </CardFooter>
                {expandedVendor === 'email' && (
                  <div className="p-4 border-t border-white/5">
                    <TestDeliveryForm vendorType="email" channel="email" orchestrationMode={orchestrationMode} apiKeyId={apiKeyId} />
                  </div>
                )}
              </Card>
            )}

            {configs.some((c) => c.vendor_type === 'mailgun') && (
              <Card>
                <CardHeader>
                  <CardTitle className="text-lg flex items-center gap-2">Mailgun</CardTitle>
                  <CardDescription>Transactional email via Mailgun API.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Domain</label>
                      <Input
                        value={String(emailConfig.mailgun?.domain || '')}
                        onChange={(e) => setEmailConfig({ ...emailConfig, mailgun: { ...emailConfig.mailgun, domain: e.target.value } })}
                        placeholder="mg.example.com"
                        className="font-mono text-sm"
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">API Key</label>
                      <Input
                        type="password"
                        value={String(emailConfig.mailgun?.api_key || '')}
                        onChange={(e) => setEmailConfig({ ...emailConfig, mailgun: { ...emailConfig.mailgun, api_key: e.target.value } })}
                        placeholder="key-xxxxxxxxxxxxxxxx"
                        className="font-mono text-sm"
                      />
                    </div>
                  </div>
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase">From (optional)</label>
                    <Input
                      value={String(emailConfig.mailgun?.from || '')}
                      onChange={(e) => setEmailConfig({ ...emailConfig, mailgun: { ...emailConfig.mailgun, from: e.target.value } })}
                      placeholder="NotifyHub <noreply@mg.example.com>"
                    />
                  </div>
                </CardContent>
                <CardFooter className="bg-muted/50 py-3 flex justify-end">
                  <div className="flex items-center gap-2">
                    <Button
                      variant="outline"
                      onClick={() => setExpandedVendor(expandedVendor === 'mailgun' ? null : 'mailgun')}
                      className={expandedVendor === 'mailgun' ? 'bg-white/10' : ''}
                    >
                      {expandedVendor === 'mailgun' ? 'Cancel Test' : 'Send Test'}
                    </Button>
                    {canRemoveVendor && (
                      <Button variant="destructive" disabled={saving} onClick={() => handleRemoveVendor('mailgun')}>
                        <Trash2 className="h-4 w-4 mr-2" />
                        Remove
                      </Button>
                    )}
                    <Button disabled={saving} onClick={() => handleSave('mailgun', emailConfig.mailgun)}>
                      {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                      Save Mailgun Settings
                    </Button>
                  </div>
                </CardFooter>
                {expandedVendor === 'mailgun' && (
                  <div className="p-4 border-t border-white/5">
                    <TestDeliveryForm vendorType="mailgun" channel="email" orchestrationMode={orchestrationMode} apiKeyId={apiKeyId} />
                  </div>
                )}
              </Card>
            )}

            {configs.some((c) => c.vendor_type === 'sendgrid') && (
              <Card>
                <CardHeader>
                  <CardTitle className="text-lg flex items-center gap-2">SendGrid</CardTitle>
                  <CardDescription>Email delivery and marketing campaigns.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">API Key</label>
                      <Input
                        type="password"
                        value={String(emailConfig.sendgrid?.api_key || '')}
                        onChange={(e) => setEmailConfig({ ...emailConfig, sendgrid: { ...emailConfig.sendgrid, api_key: e.target.value } })}
                        placeholder="SG.xxxxxxxxxxxxx"
                        className="font-mono text-sm"
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">From Email</label>
                      <Input
                        value={String(emailConfig.sendgrid?.from_email || '')}
                        onChange={(e) => setEmailConfig({ ...emailConfig, sendgrid: { ...emailConfig.sendgrid, from_email: e.target.value } })}
                        placeholder="noreply@example.com"
                      />
                    </div>
                  </div>
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase">From Name (optional)</label>
                    <Input
                      value={String(emailConfig.sendgrid?.from_name || '')}
                      onChange={(e) => setEmailConfig({ ...emailConfig, sendgrid: { ...emailConfig.sendgrid, from_name: e.target.value } })}
                      placeholder="NotifyHub"
                    />
                  </div>
                </CardContent>
                <CardFooter className="bg-muted/50 py-3 flex justify-end">
                  <div className="flex items-center gap-2">
                    <Button
                      variant="outline"
                      onClick={() => setExpandedVendor(expandedVendor === 'sendgrid' ? null : 'sendgrid')}
                      className={expandedVendor === 'sendgrid' ? 'bg-white/10' : ''}
                    >
                      {expandedVendor === 'sendgrid' ? 'Cancel Test' : 'Send Test'}
                    </Button>
                    {canRemoveVendor && (
                      <Button variant="destructive" disabled={saving} onClick={() => handleRemoveVendor('sendgrid')}>
                        <Trash2 className="h-4 w-4 mr-2" />
                        Remove
                      </Button>
                    )}
                    <Button disabled={saving} onClick={() => handleSave('sendgrid', emailConfig.sendgrid)}>
                      {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                      Save SendGrid Settings
                    </Button>
                  </div>
                </CardFooter>
                {expandedVendor === 'sendgrid' && (
                  <div className="p-4 border-t border-white/5">
                    <TestDeliveryForm vendorType="sendgrid" channel="email" orchestrationMode={orchestrationMode} apiKeyId={apiKeyId} />
                  </div>
                )}
              </Card>
            )}

            {configs.some((c) => c.vendor_type === 'postmark') && (
              <Card>
                <CardHeader>
                  <CardTitle className="text-lg flex items-center gap-2">Postmark</CardTitle>
                  <CardDescription>Fast transactional email via Postmark.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Server Token</label>
                      <Input
                        type="password"
                        value={String(emailConfig.postmark?.server_token || '')}
                        onChange={(e) => setEmailConfig({ ...emailConfig, postmark: { ...emailConfig.postmark, server_token: e.target.value } })}
                        placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
                        className="font-mono text-sm"
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">From Email</label>
                      <Input
                        value={String(emailConfig.postmark?.from_email || '')}
                        onChange={(e) => setEmailConfig({ ...emailConfig, postmark: { ...emailConfig.postmark, from_email: e.target.value } })}
                        placeholder="noreply@example.com"
                      />
                    </div>
                  </div>
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase">From Name (optional)</label>
                    <Input
                      value={String(emailConfig.postmark?.from_name || '')}
                      onChange={(e) => setEmailConfig({ ...emailConfig, postmark: { ...emailConfig.postmark, from_name: e.target.value } })}
                      placeholder="NotifyHub"
                    />
                  </div>
                </CardContent>
                <CardFooter className="bg-muted/50 py-3 flex justify-end">
                  <div className="flex items-center gap-2">
                    <Button
                      variant="outline"
                      onClick={() => setExpandedVendor(expandedVendor === 'postmark' ? null : 'postmark')}
                      className={expandedVendor === 'postmark' ? 'bg-white/10' : ''}
                    >
                      {expandedVendor === 'postmark' ? 'Cancel Test' : 'Send Test'}
                    </Button>
                    {canRemoveVendor && (
                      <Button variant="destructive" disabled={saving} onClick={() => handleRemoveVendor('postmark')}>
                        <Trash2 className="h-4 w-4 mr-2" />
                        Remove
                      </Button>
                    )}
                    <Button disabled={saving} onClick={() => handleSave('postmark', emailConfig.postmark)}>
                      {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                      Save Postmark Settings
                    </Button>
                  </div>
                </CardFooter>
                {expandedVendor === 'postmark' && (
                  <div className="p-4 border-t border-white/5">
                    <TestDeliveryForm vendorType="postmark" channel="email" orchestrationMode={orchestrationMode} apiKeyId={apiKeyId} />
                  </div>
                )}
              </Card>
            )}

            {!configs.some(c => ['ses', 'email', 'smtp', 'mailgun', 'sendgrid', 'postmark'].includes(c.vendor_type)) && (
              <div className="flex flex-col items-center justify-center py-20 bg-muted/20 border border-dashed rounded-xl">
                 <AlertCircle className="h-8 w-8 text-muted-foreground/40 mb-3" />
                 <p className="text-sm text-muted-foreground font-medium">No Email providers connected.</p>
                 <Button variant="link" size="sm" className="mt-2" onClick={() => (window as any).location.href = '/app-store'}>Connect a provider in the App Store</Button>
              </div>
            )}
          </div>
        )}

        {activeTab === 'push' && (
          <div className="grid gap-6">
            <Card>
              <CardHeader>
                <CardTitle className="text-lg flex items-center gap-2">Delivery Preference</CardTitle>
                  <CardDescription>Choose how the push worker selects vendors when multiple are configured.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Routing Mode</label>
                      <Select value={pushRouting.mode} onValueChange={(v) => setPushRouting({ ...pushRouting, mode: v as any })}>
                        <SelectTrigger><SelectValue placeholder="Select routing mode" /></SelectTrigger>
                        <SelectContent>
                          <SelectItem value="backup">Backup (prefer then fallback)</SelectItem>
                          <SelectItem value="round_robin">Round robin</SelectItem>
                          <SelectItem value="publish_all">Publish all vendors</SelectItem>
                          <SelectItem value="only">Only one vendor</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    {pushRouting.mode !== 'publish_all' ? (
                      <div className="space-y-2">
                        <label className="text-xs font-semibold text-muted-foreground uppercase">Prefer Vendor</label>
                        <Select value={pushRouting.prefer} onValueChange={(v) => setPushRouting({ ...pushRouting, prefer: v as any })}>
                          <SelectTrigger><SelectValue placeholder="Select preferred vendor" /></SelectTrigger>
                          <SelectContent>
                            <SelectItem value="fcm">FCM</SelectItem>
                            <SelectItem value="onesignal">OneSignal</SelectItem>
                            <SelectItem value="pusher">Pusher Beams</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                    ) : (
                      <div />
                    )}
                  </div>

                  {pushRouting.mode === 'backup' && (
                    <div className="grid grid-cols-2 gap-4">
                      <div className="space-y-2">
                        <label className="text-xs font-semibold text-muted-foreground uppercase">Fallback Vendor</label>
                        <Select value={pushRouting.fallback} onValueChange={(v) => setPushRouting({ ...pushRouting, fallback: v as any })}>
                          <SelectTrigger><SelectValue placeholder="Select fallback vendor" /></SelectTrigger>
                          <SelectContent>
                            <SelectItem value="fcm">FCM</SelectItem>
                            <SelectItem value="onesignal">OneSignal</SelectItem>
                            <SelectItem value="pusher">Pusher Beams</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                      <div />
                    </div>
                  )}

                  {pushRouting.mode === 'round_robin' && (
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Round Robin Participants</label>
                      <div className="grid grid-cols-3 gap-3">
                        {(['fcm', 'onesignal', 'pusher'] as const).map((v) => {
                          const checked = pushRouting.participants.includes(v)
                          return (
                            <label key={v} className="flex items-center gap-2 text-sm">
                              <input
                                type="checkbox"
                                className="h-4 w-4"
                                checked={checked}
                                onChange={(e) => {
                                  const next = e.target.checked
                                    ? Array.from(new Set([...pushRouting.participants, v]))
                                    : pushRouting.participants.filter((x) => x !== v)
                                  setPushRouting({ ...pushRouting, participants: next as any })
                                }}
                              />
                              {v}
                            </label>
                          )
                        })}
                      </div>
                      <p className="text-xs text-muted-foreground">At least one participant must be selected.</p>
                    </div>
                  )}

                  {pushRouting.mode === 'only' && (
                    <div className="grid grid-cols-2 gap-4">
                      <div className="space-y-2">
                        <label className="text-xs font-semibold text-muted-foreground uppercase">Only Vendor</label>
                        <Select value={pushRouting.only} onValueChange={(v) => setPushRouting({ ...pushRouting, only: v as any })}>
                          <SelectTrigger><SelectValue placeholder="Select only vendor" /></SelectTrigger>
                          <SelectContent>
                            <SelectItem value="fcm">FCM</SelectItem>
                            <SelectItem value="onesignal">OneSignal</SelectItem>
                            <SelectItem value="pusher">Pusher Beams</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                      <div />
                    </div>
                  )}

                  {pushRouting.mode === 'backup' && (
                    <div className="grid grid-cols-2 gap-4">
                      <div className="space-y-2">
                        <label className="text-xs font-semibold text-muted-foreground uppercase">Error-rate fallback threshold</label>
                        <Input
                          type="number"
                          step="0.01"
                          min="0"
                          max="1"
                          value={String(pushRouting.error_rate_threshold ?? 0)}
                          onChange={(e) =>
                            setPushRouting({
                              ...pushRouting,
                              error_rate_threshold: Number(e.target.value),
                            })
                          }
                          placeholder="0.25"
                        />
                        <p className="text-xs text-muted-foreground">
                          Set to <code className="text-xs">0</code> to disable. When the preferred vendor’s recent error rate exceeds this
                          value, the worker will try fallback first.
                        </p>
                      </div>
                      <div className="space-y-2">
                        <label className="text-xs font-semibold text-muted-foreground uppercase">Min requests</label>
                        <Input
                          type="number"
                          min="1"
                          value={String(pushRouting.min_requests ?? 20)}
                          onChange={(e) =>
                            setPushRouting({
                              ...pushRouting,
                              min_requests: Number(e.target.value),
                            })
                          }
                          placeholder="20"
                        />
                        <p className="text-xs text-muted-foreground">
                          Minimum recent attempts before applying error-rate based fallback.
                        </p>
                      </div>
                    </div>
                  )}
                </CardContent>
                <CardFooter className="bg-muted/50 py-3 flex justify-between">
                  <p className="text-xs text-muted-foreground italic">This controls vendor selection in the worker.</p>
                    <div className="flex items-center gap-2">
                      <Button disabled={saving} onClick={() => handleSave('push_routing', pushRouting)}>
                        {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                        Save Preference
                      </Button>
                    </div>
                </CardFooter>
              </Card>

              {canEditRateLimits && (
                <VendorRateLimits
                  apiKeyId={apiKeyId}
                  vendorIds={configuredPushVendors}
                  canEdit={canEditRateLimits}
                  canDelete={canDeleteRateLimits}
                />
              )}

            {configs.some(c => c.vendor_type === 'fcm') && (
              <Card>
                <CardHeader>
                  <CardTitle>Firebase Cloud Messaging</CardTitle>
                  <CardDescription>Mobile push delivery via FCM.</CardDescription>
                </CardHeader>
                <CardContent>
                  <div className="flex items-center justify-between gap-3">
                    <div className="space-y-1">
                      <p className="text-sm font-medium">FCM Provider</p>
                      <p className="text-xs text-muted-foreground">
                        {fcmConfig?.service_account
                          ? 'Service account JSON is configured.'
                          : fcmConfig?.server_key
                            ? 'Legacy server key is configured.'
                            : 'No credentials configured yet.'}
                      </p>
                    </div>
                    {(fcmConfig?.service_account || fcmConfig?.server_key) ? (
                      <Badge variant="secondary" className="bg-green-500/10 text-green-700 border-green-500/20 gap-1">
                        <CheckCircle2 className="h-3 w-3" />
                        Connected
                      </Badge>
                    ) : (
                      <Badge variant="outline" className="gap-1">
                        <AlertCircle className="h-3 w-3" />
                        Not configured
                      </Badge>
                    )}
                  </div>

                  {fcmConfig?.service_account && (
                    <div className="mt-4 rounded-lg border bg-muted/20 p-3">
                      <p className="text-xs font-semibold text-muted-foreground uppercase mb-2">Service Account</p>
                      <div className="text-xs text-muted-foreground space-y-1">
                        {fcmConfig.service_account.project_id && (
                          <div className="flex items-center justify-between gap-2">
                            <span>project_id</span>
                            <span className="font-mono text-foreground">{String(fcmConfig.service_account.project_id)}</span>
                          </div>
                        )}
                        {fcmConfig.service_account.client_email && (
                          <div className="flex items-center justify-between gap-2">
                            <span>client_email</span>
                            <span className="font-mono text-foreground">{String(fcmConfig.service_account.client_email)}</span>
                          </div>
                        )}
                      </div>
                    </div>
                  )}

                  <div className={cn(
                    "relative mt-4 border-2 border-dashed rounded-lg p-5 flex flex-col items-center justify-center transition-colors text-center",
                    fcmConfig?.service_account ? "border-green-500/50 bg-green-50/50" : "border-muted-foreground/20 hover:border-primary/50 hover:bg-primary/5"
                  )}>
                    <div className="mb-2 h-10 w-10 rounded-full bg-muted flex items-center justify-center">
                      {fcmConfig?.service_account ? <FileJson className="h-5 w-5 text-green-600" /> : <Upload className="h-5 w-5 text-muted-foreground" />}
                    </div>
                    <p className="text-sm font-medium">
                      {fcmConfig?.service_account ? "Replace service account JSON" : "Upload service_account.json"}
                    </p>
                    <p className="text-xs text-muted-foreground mt-1">Used for Firebase HTTP v1 API</p>
                    <input
                      type="file"
                      accept=".json,application/json"
                      className="absolute inset-0 opacity-0 cursor-pointer"
                      onChange={handleFcmFileUpload}
                    />
                  </div>
                </CardContent>
                {canRemoveVendor && (
                  <CardFooter className="bg-muted/50 py-3 flex justify-end">
                    <Button
                      variant="destructive"
                      disabled={saving}
                      onClick={() => handleRemoveVendor('fcm')}
                    >
                      <Trash2 className="h-4 w-4 mr-2" />
                      Remove
                    </Button>
                  </CardFooter>
                )}
              </Card>
            )}

            {configs.some((c) => c.vendor_type === 'onesignal') && (
              <Card>
                <CardHeader>
                  <CardTitle>OneSignal</CardTitle>
                  <CardDescription>Omnichannel customer engagement platform.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">App ID</label>
                      <Input
                        value={String(oneSignalConfig.app_id ?? '')}
                        onChange={(e) => setOneSignalConfig({ ...oneSignalConfig, app_id: e.target.value })}
                        placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
                        className="font-mono text-sm"
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">REST API Key</label>
                      <Input
                        type="password"
                        value={String(oneSignalConfig.rest_api_key ?? '')}
                        onChange={(e) => setOneSignalConfig({ ...oneSignalConfig, rest_api_key: e.target.value })}
                        placeholder="••••••••••••••••"
                        className="font-mono text-sm"
                      />
                    </div>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    This integration targets OneSignal <code className="text-xs">external_user_id</code> (recipient is that id).
                  </p>
                </CardContent>
                <CardFooter className="bg-muted/50 py-3 flex justify-end">
                  <div className="flex items-center gap-2">
                    {canRemoveVendor && (
                      <Button variant="destructive" disabled={saving} onClick={() => handleRemoveVendor('onesignal')}>
                        <Trash2 className="h-4 w-4 mr-2" />
                        Remove
                      </Button>
                    )}
                    <Button disabled={saving} onClick={() => handleSave('onesignal', oneSignalConfig)}>
                      {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                      Save OneSignal Settings
                    </Button>
                  </div>
                </CardFooter>
              </Card>
            )}

            {configs.some((c) => c.vendor_type === 'pusher') && (
              <Card>
                <CardHeader>
                  <CardTitle>Pusher Beams</CardTitle>
                  <CardDescription>Real-time infrastructure for modern apps.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Instance ID</label>
                      <Input
                        value={String(pusherConfig.instance_id ?? '')}
                        onChange={(e) => setPusherConfig({ ...pusherConfig, instance_id: e.target.value })}
                        placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
                        className="font-mono text-sm"
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Secret Key</label>
                      <Input
                        type="password"
                        value={String(pusherConfig.secret_key ?? '')}
                        onChange={(e) => setPusherConfig({ ...pusherConfig, secret_key: e.target.value })}
                        placeholder="••••••••••••••••"
                        className="font-mono text-sm"
                      />
                    </div>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    This integration publishes to a single interest (recipient is the interest name).
                  </p>
                </CardContent>
                <CardFooter className="bg-muted/50 py-3 flex justify-end">
                  <div className="flex items-center gap-2">
                    {canRemoveVendor && (
                      <Button variant="destructive" disabled={saving} onClick={() => handleRemoveVendor('pusher')}>
                        <Trash2 className="h-4 w-4 mr-2" />
                        Remove
                      </Button>
                    )}
                    <Button disabled={saving} onClick={() => handleSave('pusher', pusherConfig)}>
                      {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                      Save Pusher Settings
                    </Button>
                  </div>
                </CardFooter>
              </Card>
            )}

            {!configs.some(c => ['fcm', 'onesignal', 'pusher'].includes(c.vendor_type)) && (
              <div className="flex flex-col items-center justify-center py-20 bg-muted/20 border border-dashed rounded-xl">
                 <AlertCircle className="h-8 w-8 text-muted-foreground/40 mb-3" />
                 <p className="text-sm text-muted-foreground font-medium">No Push providers connected.</p>
                 <Button variant="link" size="sm" className="mt-2" onClick={() => (window as any).location.href = '/app-store'}>Connect a provider in the App Store</Button>
              </div>
            )}
          </div>
        )}

        {activeTab === 'social' && (
          <div className="grid gap-6">
            <Card className="border-muted-foreground/15 bg-muted/20">
              <CardHeader className="pb-2">
                <CardTitle className="text-lg">Social and apps</CardTitle>
                <CardDescription>
                  Manage credentials for chat and collaboration integrations. This section has no worker routing or delivery preferences—only stored configuration.
                </CardDescription>
              </CardHeader>
            </Card>

            {SOCIAL_VENDORS.map((v) => {
              const connected = configs.some((c) => c.vendor_type === v.id)
              if (!connected) return null
              const data = socialConfigs[v.id] || {}
              return (
                <Card key={v.id}>
                  <CardHeader>
                    <CardTitle className="text-lg">{v.title}</CardTitle>
                    <CardDescription>{v.description}</CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    {v.field === 'webhook' ? (
                      v.id === 'slack' ? (
                        <div className="space-y-4">
                          <div className="space-y-2">
                            <label className="text-xs font-semibold text-muted-foreground uppercase">Default username (optional)</label>
                            <Input
                              value={String(data.default_username ?? '')}
                              onChange={(e) =>
                                setSocialConfigs((prev) => ({
                                  ...prev,
                                  [v.id]: { ...prev[v.id], default_username: e.target.value },
                                }))
                              }
                              placeholder="NotifyHub"
                              className="text-sm"
                            />
                          </div>

                          <div className="space-y-2">
                            <div className="flex items-center justify-between gap-3">
                              <label className="text-xs font-semibold text-muted-foreground uppercase">Slack channels</label>
                              <Button
                                type="button"
                                variant="outline"
                                size="sm"
                                onClick={() =>
                                  setSocialConfigs((prev) => ({
                                    ...prev,
                                    [v.id]: {
                                      ...(prev[v.id] ?? {}),
                                      channels: [
                                        ...((prev[v.id]?.channels as any[]) ?? []),
                                        { channel_name: '', webhook_url: '' },
                                      ],
                                    },
                                  }))
                                }
                              >
                                <Plus className="h-4 w-4 mr-2" />
                                Add channel
                              </Button>
                            </div>

                            <div className="space-y-2">
                              {(((data.channels as any[]) ?? []) as any[]).length === 0 ? (
                                <p className="text-sm text-muted-foreground">
                                  No channels configured yet. Add one, or use the legacy single webhook below.
                                </p>
                              ) : (
                                (((data.channels as any[]) ?? []) as any[]).map((ch, idx) => (
                                  <div key={idx} className="grid grid-cols-1 md:grid-cols-[1fr_2fr_auto] gap-2">
                                    <Input
                                      value={String(ch.channel_name ?? '')}
                                      onChange={(e) =>
                                        setSocialConfigs((prev) => {
                                          const prevCh = ((prev[v.id]?.channels as any[]) ?? []).slice()
                                          prevCh[idx] = { ...(prevCh[idx] ?? {}), channel_name: e.target.value }
                                          return { ...prev, [v.id]: { ...(prev[v.id] ?? {}), channels: prevCh } }
                                        })
                                      }
                                      placeholder="alerts"
                                    />
                                    <Input
                                      value={String(ch.webhook_url ?? '')}
                                      onChange={(e) =>
                                        setSocialConfigs((prev) => {
                                          const prevCh = ((prev[v.id]?.channels as any[]) ?? []).slice()
                                          prevCh[idx] = { ...(prevCh[idx] ?? {}), webhook_url: e.target.value }
                                          return { ...prev, [v.id]: { ...(prev[v.id] ?? {}), channels: prevCh } }
                                        })
                                      }
                                      placeholder="https://hooks.slack.com/services/..."
                                      className="font-mono text-sm"
                                    />
                                    <Button
                                      type="button"
                                      variant="ghost"
                                      size="icon"
                                      className="text-destructive hover:text-destructive"
                                      onClick={() =>
                                        setSocialConfigs((prev) => {
                                          const prevCh = ((prev[v.id]?.channels as any[]) ?? []).slice()
                                          prevCh.splice(idx, 1)
                                          return { ...prev, [v.id]: { ...(prev[v.id] ?? {}), channels: prevCh } }
                                        })
                                      }
                                      aria-label="Remove channel"
                                    >
                                      <X className="h-4 w-4" aria-hidden />
                                    </Button>
                                  </div>
                                ))
                              )}
                            </div>
                          </div>

                          <div className="space-y-2">
                            <label className="text-xs font-semibold text-muted-foreground uppercase">
                              Legacy single webhook (optional)
                            </label>
                            <Input
                              value={String(data.webhook_url ?? '')}
                              onChange={(e) =>
                                setSocialConfigs((prev) => ({
                                  ...prev,
                                  [v.id]: { ...prev[v.id], webhook_url: e.target.value },
                                }))
                              }
                              placeholder="https://hooks.slack.com/services/..."
                              className="font-mono text-sm"
                            />
                          </div>
                        </div>
                      ) : (
                        <div className="space-y-2">
                          <label className="text-xs font-semibold text-muted-foreground uppercase">Webhook URL</label>
                          <Input
                            value={String(data.webhook_url ?? '')}
                            onChange={(e) =>
                              setSocialConfigs((prev) => ({
                                ...prev,
                                [v.id]: { ...prev[v.id], webhook_url: e.target.value },
                              }))
                            }
                            placeholder="https://..."
                            className="font-mono text-sm"
                          />
                        </div>
                      )
                    ) : (
                      v.id === 'telegram' ? (
                        <div className="space-y-3">
                          <div className="space-y-2">
                            <label className="text-xs font-semibold text-muted-foreground uppercase">Bot token</label>
                            <Input
                              type="password"
                              value={String(data.bot_token ?? '')}
                              onChange={(e) =>
                                setSocialConfigs((prev) => ({
                                  ...prev,
                                  [v.id]: { ...prev[v.id], bot_token: e.target.value },
                                }))
                              }
                              placeholder="123456789:AA..."
                              className="font-mono text-sm"
                            />
                          </div>
                          <div className="space-y-2">
                            <label className="text-xs font-semibold text-muted-foreground uppercase">Chat ID</label>
                            <Input
                              value={String(data.chat_id ?? '')}
                              onChange={(e) =>
                                setSocialConfigs((prev) => ({
                                  ...prev,
                                  [v.id]: { ...prev[v.id], chat_id: e.target.value },
                                }))
                              }
                              placeholder="-1001234567890"
                              className="font-mono text-sm"
                            />
                          </div>
                        </div>
                      ) : (
                        <div className="space-y-2">
                          <label className="text-xs font-semibold text-muted-foreground uppercase">API key / token</label>
                          <Input
                            type="password"
                            value={String(data.api_key ?? '')}
                            onChange={(e) =>
                              setSocialConfigs((prev) => ({
                                ...prev,
                                [v.id]: { ...prev[v.id], api_key: e.target.value },
                              }))
                            }
                            placeholder="••••••••"
                            className="font-mono text-sm"
                          />
                        </div>
                      )
                    )}
                  </CardContent>
                  <CardFooter className="bg-muted/50 py-3 flex justify-between">
                    <p className="text-xs text-muted-foreground italic">Overrides are persisted per vendor in the config store.</p>
                    <div className="flex items-center gap-2">
                      {v.id === 'slack' && (
                        <Button 
                          variant="outline" 
                          onClick={() => setExpandedVendor(expandedVendor === 'slack' ? null : 'slack')}
                          className={expandedVendor === 'slack' ? 'bg-white/10' : ''}
                        >
                          {expandedVendor === 'slack' ? 'Cancel Test' : 'Send Test'}
                        </Button>
                      )}
                      {canRemoveVendor && (
                        <Button
                          variant="destructive"
                          disabled={saving}
                          onClick={() => handleRemoveVendor(v.id)}
                        >
                          <Trash2 className="h-4 w-4 mr-2" />
                          Remove
                        </Button>
                      )}
                      <Button disabled={saving} onClick={() => handleSave(v.id, socialConfigs[v.id] || {})}>
                        {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                        Save
                      </Button>
                    </div>
                  </CardFooter>
                  {expandedVendor === v.id && (
                    <div className="p-4 border-t border-white/5">
                      <TestDeliveryForm vendorType={v.id} channel={v.id === 'slack' ? 'slack' : 'webhook' as any} orchestrationMode={orchestrationMode} apiKeyId={apiKeyId} />
                    </div>
                  )}
                </Card>
              )
            })}

            {!SOCIAL_VENDORS.some((v) => configs.some((c) => c.vendor_type === v.id)) && (
              <div className="flex flex-col items-center justify-center py-20 bg-muted/20 border border-dashed rounded-xl">
                <AlertCircle className="h-8 w-8 text-muted-foreground/40 mb-3" />
                <p className="text-sm text-muted-foreground font-medium">No social providers connected.</p>
                <Button variant="link" size="sm" className="mt-2" onClick={() => ((window as any).location.href = '/app-store')}>
                  Connect a provider in the App Store
                </Button>
              </div>
            )}
          </div>
        )}

        {activeTab === 'store' && (
          <div className="grid gap-6">
            <Card>
              <CardHeader className="flex flex-row items-center justify-between pb-2">
                <div>
                  <CardTitle className="text-lg">Advanced Config Store</CardTitle>
                  <CardDescription>Manage arbitrary JSON configurations for custom providers (e.g. Slack, Discord, Webhooks).</CardDescription>
                </div>
                <Dialog open={isDialogOpen} onOpenChange={setIsDialogOpen}>
                  <DialogTrigger asChild>
                    <Button size="sm" className="gap-2">
                      <Plus className="h-4 w-4" />
                      Add Custom Vendor
                    </Button>
                  </DialogTrigger>
                  <DialogContent className="sm:max-w-[500px]">
                    <DialogHeader>
                      <DialogTitle>Add Custom Vendor</DialogTitle>
                      <DialogDescription>
                        Register a new vendor configuration. Provide a unique vendor alias and valid JSON settings.
                      </DialogDescription>
                    </DialogHeader>
                    <div className="grid gap-4 py-4">
                      <div className="space-y-2">
                        <label className="text-xs font-semibold text-muted-foreground uppercase">Vendor Identifier</label>
                        <Input
                          value={newVendorType}
                          onChange={(e) => setNewVendorType(e.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, ''))}
                          placeholder="e.g. slack_webhook"
                        />
                      </div>
                      <div className="space-y-2">
                        <label className="text-xs font-semibold text-muted-foreground uppercase">Configuration (JSON)</label>
                        <textarea
                          className="flex w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 min-h-[150px] font-mono"
                          value={newVendorJson}
                          onChange={(e) => setNewVendorJson(e.target.value)}
                        />
                      </div>
                    </div>
                    <DialogFooter>
                      <Button variant="outline" onClick={() => setIsDialogOpen(false)}>Cancel</Button>
                      <Button 
                        disabled={saving || !newVendorType || !newVendorJson} 
                        onClick={() => {
                          try {
                            const parsed = JSON.parse(newVendorJson)
                            handleSave(newVendorType, parsed)
                          } catch (e) {
                            setError('Invalid JSON format.')
                          }
                        }}
                      >
                        {saving ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : <Save className="h-4 w-4 mr-2" />}
                        Save Vendor
                      </Button>
                    </DialogFooter>
                  </DialogContent>
                </Dialog>
              </CardHeader>
              <CardContent className="space-y-6">
                <Separator />
                {configs.filter(c => !['sms', 'email', 'push', 'workflow_orchestration'].includes(c.vendor_type)).length === 0 ? (
                  <div className="text-center py-8 text-sm text-muted-foreground border rounded-lg border-dashed">
                    No custom configurations found. Add a vendor to begin.
                  </div>
                ) : (
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className="w-[150px]">Vendor Alias</TableHead>
                        <TableHead>Config Snippet</TableHead>
                        <TableHead className="text-right">Actions</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {configs.filter(c => !['sms', 'email', 'push', 'workflow_orchestration'].includes(c.vendor_type)).map((cfg) => (
                        <TableRow key={cfg.id}>
                          <TableCell className="font-mono text-xs">{cfg.vendor_type}</TableCell>
                          <TableCell className="font-mono text-xs text-muted-foreground truncate max-w-[200px]">
                            {JSON.stringify(cfg.config_json)}
                          </TableCell>
                          <TableCell className="text-right">
                            <div className="flex items-center justify-end gap-2">
                              {canRemoveVendor && (
                                <Button
                                  variant="destructive"
                                  size="sm"
                                  disabled={saving}
                                  onClick={() => handleRemoveVendor(cfg.vendor_type)}
                                >
                                  <Trash2 className="h-4 w-4 mr-2" />
                                  Remove
                                </Button>
                              )}
                              <Button 
                                variant="ghost" 
                                size="sm"
                                onClick={() => {
                                  setNewVendorType(cfg.vendor_type)
                                  setNewVendorJson(JSON.stringify(cfg.config_json, null, 2))
                                  setIsDialogOpen(true)
                                }}
                              >
                                Edit
                              </Button>
                            </div>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                )}
              </CardContent>
            </Card>
          </div>
        )}
      </>
    )}
      </div>
    </div>
  )
}
