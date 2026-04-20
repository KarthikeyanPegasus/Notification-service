'use client'

import React, { useState, useEffect } from 'react'
import { PageHeader } from '@/components/shared/page-header'
import { Card, CardContent, CardDescription, CardHeader, CardTitle, CardFooter } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import { Badge } from '@/components/ui/badge'
import { getVendorConfigs, updateVendorConfig, VendorConfig } from '@/lib/api'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger, DialogFooter } from '@/components/ui/dialog'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Mail, MessageSquare, Bell, Save, Loader2, ShieldCheck, AlertCircle, Database, Plus, Upload, FileJson, CheckCircle2, Slack } from 'lucide-react'
import { cn } from '@/lib/utils'

const SOCIAL_VENDORS: { id: string; title: string; description: string; field: 'webhook' | 'api_key' }[] = [
  { id: 'slack', title: 'Slack', description: 'Incoming Webhook URL (used for Slack channel deliveries).', field: 'webhook' },
  { id: 'discord', title: 'Discord', description: 'Discord channel webhook URL.', field: 'webhook' },
  { id: 'teams', title: 'Microsoft Teams', description: 'Workflow or connector token / secret.', field: 'api_key' },
  { id: 'telegram', title: 'Telegram', description: 'Bot token or API credential.', field: 'api_key' },
]

export default function SettingsPage() {
  const [configs, setConfigs] = useState<VendorConfig[]>([])
  const [loading, setLoading] = useState(true)
  const [activeTab, setActiveTab] = useState<'sms' | 'email' | 'push' | 'social' | 'store'>('sms')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [newVendorType, setNewVendorType] = useState('')
  const [newVendorJson, setNewVendorJson] = useState('{\n  \n}')
  const [isDialogOpen, setIsDialogOpen] = useState(false)

  // Local state for forms
  const [smsConfig, setSmsConfig] = useState({
    primary: 'twilio',
    twilio: { account_sid: '', auth_token: '', from_number: '' },
    plivo: { auth_id: '', auth_token: '', from_number: '' },
    vonage: { api_key: '', api_secret: '', from: '' }
  })

  const [smsRouting, setSmsRouting] = useState({
    mode: 'backup' as 'backup' | 'round_robin' | 'publish_all' | 'only',
    prefer: 'twilio' as 'twilio' | 'plivo' | 'vonage',
    fallback: 'plivo' as 'twilio' | 'plivo' | 'vonage',
    only: 'twilio' as 'twilio' | 'plivo' | 'vonage',
    participants: ['twilio', 'plivo', 'vonage'] as Array<'twilio' | 'plivo' | 'vonage'>,
  })

  const [emailConfig, setEmailConfig] = useState({
    primary: 'smtp',
    ses: { region: '', access_key_id: '', secret_access_key: '', from_address: '', from_name: '', smtp_username: '', smtp_password: '' },
    smtp: { host: '', port: 587, username: '', password: '', from: '' }
  })

  const [emailRouting, setEmailRouting] = useState({
    mode: 'backup' as 'backup' | 'round_robin' | 'publish_all' | 'only',
    prefer: 'ses' as 'ses' | 'smtp' | 'mailgun',
    fallback: 'smtp' as 'ses' | 'smtp' | 'mailgun',
    only: 'ses' as 'ses' | 'smtp' | 'mailgun',
    participants: ['ses', 'smtp', 'mailgun'] as Array<'ses' | 'smtp' | 'mailgun'>,
  })

  const [pushRouting, setPushRouting] = useState({
    mode: 'backup' as 'backup' | 'round_robin' | 'publish_all' | 'only',
    prefer: 'fcm' as 'fcm',
    fallback: 'fcm' as 'fcm',
    only: 'fcm' as 'fcm',
    participants: ['fcm'] as Array<'fcm'>,
  })

  type FcmConfig = {
    server_key?: string
    service_account?: Record<string, any> | null
    [key: string]: any
  }

  const [fcmConfig, setFcmConfig] = useState<FcmConfig>({
    server_key: '',
    service_account: null,
  })

  const [socialConfigs, setSocialConfigs] = useState<Record<string, Record<string, any>>>({})

  useEffect(() => {
    loadConfigs()
  }, [])

  const loadConfigs = async () => {
    try {
      setLoading(true)
      const data = (await getVendorConfigs()) || []
      setConfigs(data)
      
      // Populate form state from DB configs
      data.forEach(cfg => {
        if (cfg.vendor_type === 'sms') setSmsConfig(prev => ({ ...prev, ...cfg.config_json }))
        if (cfg.vendor_type === 'twilio') setSmsConfig(prev => ({ ...prev, twilio: cfg.config_json }))
        if (cfg.vendor_type === 'plivo') setSmsConfig(prev => ({ ...prev, plivo: cfg.config_json }))
        if (cfg.vendor_type === 'vonage') setSmsConfig(prev => ({ ...prev, vonage: cfg.config_json }))
        if (cfg.vendor_type === 'sms_routing') setSmsRouting(prev => ({ ...prev, ...cfg.config_json }))
        
        if (cfg.vendor_type === 'email') setEmailConfig(prev => ({ ...prev, ...cfg.config_json }))
        if (cfg.vendor_type === 'ses') setEmailConfig(prev => ({ ...prev, ses: cfg.config_json }))

        if (cfg.vendor_type === 'email_routing') setEmailRouting(prev => ({ ...prev, ...cfg.config_json }))

        if (cfg.vendor_type === 'push_routing') setPushRouting(prev => ({ ...prev, ...cfg.config_json }))
        if (cfg.vendor_type === 'fcm') setFcmConfig(prev => ({ ...prev, ...cfg.config_json }))
        if (SOCIAL_VENDORS.some((s) => s.id === cfg.vendor_type)) {
          setSocialConfigs((prev) => ({
            ...prev,
            [cfg.vendor_type]: { ...(cfg.config_json as Record<string, any>) },
          }))
        }
      })
    } catch (err) {
      console.error(err)
      setError('Failed to load settings from server.')
    } finally {
      setLoading(false)
    }
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
      await updateVendorConfig(type, config)
      setSuccess(`${type.toUpperCase()} configuration updated successfully.`)
      setTimeout(() => setSuccess(null), 3000)
      if (activeTab === 'store') {
        loadConfigs()
        setIsDialogOpen(false)
        setNewVendorType('')
        setNewVendorJson('{\n  \n}')
      }
      if (activeTab === 'social' && SOCIAL_VENDORS.some((s) => s.id === type)) {
        const data = (await getVendorConfigs()) || []
        setConfigs(data)
        data.forEach((cfg) => {
          if (SOCIAL_VENDORS.some((s) => s.id === cfg.vendor_type)) {
            setSocialConfigs((prev) => ({
              ...prev,
              [cfg.vendor_type]: { ...(cfg.config_json as Record<string, any>) },
            }))
          }
        })
      }
    } catch (err) {
      setError(`Failed to update ${type} settings.`)
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <div className="flex flex-col gap-6 p-6">
        <PageHeader title="Settings" description="Manage notification providers and system configuration." />
        <div className="flex h-64 items-center justify-center">
          <Loader2 className="h-8 w-8 animate-spin text-primary" />
        </div>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-6 p-6 max-w-5xl mx-auto">
      <PageHeader 
        title="Settings" 
        description="Manage your notification delivery providers in real-time." 
      />

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
            <Slack className="h-4 w-4" />
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
        {activeTab === 'sms' && (
          <div className="grid gap-6">
            {(
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
                            <SelectItem value="twilio">Twilio</SelectItem>
                            <SelectItem value="plivo">Plivo</SelectItem>
                            <SelectItem value="vonage">Vonage</SelectItem>
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
                            <SelectItem value="twilio">Twilio</SelectItem>
                            <SelectItem value="plivo">Plivo</SelectItem>
                            <SelectItem value="vonage">Vonage</SelectItem>
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
                        {(['twilio', 'plivo', 'vonage'] as const).map((v) => {
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
                              {v}
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
                            <SelectItem value="twilio">Twilio</SelectItem>
                            <SelectItem value="plivo">Plivo</SelectItem>
                            <SelectItem value="vonage">Vonage</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                      <div />
                    </div>
                  )}
                </CardContent>
                <CardFooter className="bg-muted/50 py-3 flex justify-between">
                  <p className="text-xs text-muted-foreground italic">This controls vendor selection in the worker.</p>
                  <Button disabled={saving} onClick={() => handleSave('sms_routing', smsRouting)}>
                    {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                    Save Preference
                  </Button>
                </CardFooter>
              </Card>
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
                  <Button disabled={saving} onClick={() => handleSave('twilio', smsConfig.twilio)}>
                    {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                    Save Twilio Settings
                  </Button>
                </CardFooter>
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
                  <Button disabled={saving} onClick={() => handleSave('plivo', smsConfig.plivo)}>
                    {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                    Save Plivo Settings
                  </Button>
                </CardFooter>
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
                  <Button disabled={saving} onClick={() => handleSave('vonage', smsConfig.vonage)}>
                    {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                    Save Vonage Settings
                  </Button>
                </CardFooter>
              </Card>
            )}

            {!configs.some(c => ['twilio', 'plivo', 'vonage'].includes(c.vendor_type)) && (
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
            {(
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
                            <SelectItem value="ses">Amazon SES</SelectItem>
                            <SelectItem value="smtp">SMTP Relay</SelectItem>
                            <SelectItem value="mailgun">Mailgun</SelectItem>
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
                            <SelectItem value="ses">Amazon SES</SelectItem>
                            <SelectItem value="smtp">SMTP Relay</SelectItem>
                            <SelectItem value="mailgun">Mailgun</SelectItem>
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
                        {(['ses', 'smtp', 'mailgun'] as const).map((v) => {
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
                              {v}
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
                            <SelectItem value="ses">Amazon SES</SelectItem>
                            <SelectItem value="smtp">SMTP Relay</SelectItem>
                            <SelectItem value="mailgun">Mailgun</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                      <div />
                    </div>
                  )}
                </CardContent>
                <CardFooter className="bg-muted/50 py-3 flex justify-between">
                  <p className="text-xs text-muted-foreground italic">This controls vendor selection in the worker.</p>
                  <Button disabled={saving} onClick={() => handleSave('email_routing', emailRouting)}>
                    {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                    Save Preference
                  </Button>
                </CardFooter>
              </Card>
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
                </CardContent>
                <CardFooter className="bg-muted/50 py-3 flex justify-end">
                  <Button disabled={saving} onClick={() => handleSave('ses', emailConfig.ses)}>
                    {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                    Save SES Settings
                  </Button>
                </CardFooter>
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
                  <Button disabled={saving} onClick={() => handleSave('email', emailConfig)}>
                    {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                    Save SMTP Settings
                  </Button>
                </CardFooter>
              </Card>
            )}

            {!configs.some(c => ['ses', 'email', 'smtp'].includes(c.vendor_type)) && (
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
            {(
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
                        {(['fcm'] as const).map((v) => {
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
                          </SelectContent>
                        </Select>
                      </div>
                      <div />
                    </div>
                  )}
                </CardContent>
                <CardFooter className="bg-muted/50 py-3 flex justify-between">
                  <p className="text-xs text-muted-foreground italic">This controls vendor selection in the worker.</p>
                  <Button disabled={saving} onClick={() => handleSave('push_routing', pushRouting)}>
                    {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                    Save Preference
                  </Button>
                </CardFooter>
              </Card>
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
              </Card>
            )}

            {!configs.some(c => c.vendor_type === 'fcm') && (
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
                    )}
                  </CardContent>
                  <CardFooter className="bg-muted/50 py-3 flex justify-between">
                    <p className="text-xs text-muted-foreground italic">Overrides are persisted per vendor in the config store.</p>
                    <Button disabled={saving} onClick={() => handleSave(v.id, socialConfigs[v.id] || {})}>
                      {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
                      Save
                    </Button>
                  </CardFooter>
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
              <CardContent>
                {configs.filter(c => !['sms', 'email', 'push'].includes(c.vendor_type)).length === 0 ? (
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
                      {configs.filter(c => !['sms', 'email', 'push'].includes(c.vendor_type)).map((cfg) => (
                        <TableRow key={cfg.id}>
                          <TableCell className="font-mono text-xs">{cfg.vendor_type}</TableCell>
                          <TableCell className="font-mono text-xs text-muted-foreground truncate max-w-[200px]">
                            {JSON.stringify(cfg.config_json)}
                          </TableCell>
                          <TableCell className="text-right">
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
      </div>
    </div>
  )
}
