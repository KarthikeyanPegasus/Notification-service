/* eslint-disable react-hooks/exhaustive-deps */
'use client'

import React, { useEffect, useMemo, useState } from 'react'
import { Card, CardContent, CardDescription, CardFooter, CardHeader } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Loader2, Save, Radio } from 'lucide-react'
import { getVendorConfigsScoped, updateVendorConfigScoped } from '@/lib/api'

type IngressChannel = 'sms' | 'email' | 'push' | 'webhook' | 'websocket' | 'slack'

type IngressConfig = {
  api_enabled?: boolean
  pubsub?: {
    enabled?: boolean
    project_id?: string
    service_account_json?: Record<string, any> | null
    channels?: Partial<Record<IngressChannel, { topic?: string; subscription?: string }>>
  }
}

const CHANNELS: IngressChannel[] = ['sms', 'email', 'push', 'webhook', 'websocket', 'slack']

export function IngressSettings({ apiKeyId }: { apiKeyId?: string }) {
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)

  const [cfg, setCfg] = useState<IngressConfig>({
    api_enabled: true,
    pubsub: {
      enabled: false,
      project_id: '',
      service_account_json: null,
      channels: {},
    },
  })

  const load = async () => {
    setLoading(true)
    setError(null)
    setSuccess(null)
    try {
      const configs = await getVendorConfigsScoped(apiKeyId)
      const ingress = configs.find((c) => c.vendor_type === 'ingress')
      if (ingress?.config_json) {
        setCfg((prev) => ({
          ...prev,
          ...(ingress.config_json as IngressConfig),
          pubsub: {
            ...prev.pubsub,
            ...(ingress.config_json?.pubsub as any),
            channels: {
              ...(prev.pubsub?.channels ?? {}),
              ...(ingress.config_json?.pubsub?.channels ?? {}),
            },
          },
        }))
      } else {
        setCfg({
          api_enabled: true,
          pubsub: { enabled: false, project_id: '', service_account_json: null, channels: {} },
        })
      }
    } catch (e) {
      console.error(e)
      setError('Failed to load ingress settings.')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [apiKeyId])

  const pubsubEnabled = Boolean(cfg.pubsub?.enabled)
  const apiEnabled = cfg.api_enabled !== false

  const channelRows = useMemo(() => {
    const m = cfg.pubsub?.channels ?? {}
    return CHANNELS.map((ch) => ({
      ch,
      topic: m[ch]?.topic ?? '',
      subscription: m[ch]?.subscription ?? '',
    }))
  }, [cfg.pubsub?.channels])

  const handleServiceAccountUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = (evt) => {
      try {
        const json = JSON.parse(String(evt.target?.result ?? 'null'))
        setCfg((prev) => ({
          ...prev,
          pubsub: { ...(prev.pubsub ?? {}), service_account_json: json },
        }))
      } catch {
        setError('Invalid JSON file.')
        setTimeout(() => setError(null), 3000)
      } finally {
        e.target.value = ''
      }
    }
    reader.readAsText(file)
  }

  const save = async () => {
    setSaving(true)
    setError(null)
    setSuccess(null)
    try {
      await updateVendorConfigScoped('ingress', cfg, apiKeyId)
      setSuccess('Ingress settings saved.')
      setTimeout(() => setSuccess(null), 2500)
    } catch (e) {
      console.error(e)
      setError('Failed to save ingress settings.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-4">
      {error && (
        <p className="text-sm text-destructive rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3">
          {error}
        </p>
      )}
      {success && (
        <p className="text-sm text-green-700 rounded-lg border border-green-500/30 bg-green-500/10 px-4 py-3">
          {success}
        </p>
      )}

      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <Radio className="h-5 w-5 text-primary" aria-hidden />
            <div>
              <div className="text-base font-semibold">Ingress settings</div>
              <CardDescription>Configure how this client can ingest notifications (API and Pub/Sub).</CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-5">
          {loading ? (
            <div className="flex items-center justify-center py-10 text-muted-foreground">
              <Loader2 className="h-6 w-6 animate-spin" />
            </div>
          ) : (
            <>
              <div className="flex items-center justify-between gap-4 rounded-lg border p-4">
                <div>
                  <p className="text-sm font-medium">API ingress</p>
                  <p className="text-xs text-muted-foreground">Allow this client to call HTTP endpoints like POST /v1/notifications.</p>
                </div>
                <Button
                  variant={apiEnabled ? 'default' : 'outline'}
                  onClick={() => setCfg((p) => ({ ...p, api_enabled: !apiEnabled }))}
                >
                  {apiEnabled ? 'Enabled' : 'Disabled'}
                </Button>
              </div>

              <div className="flex items-center justify-between gap-4 rounded-lg border p-4">
                <div>
                  <p className="text-sm font-medium">Pub/Sub ingress</p>
                  <p className="text-xs text-muted-foreground">Allow this client to ingest via Pub/Sub subscriptions.</p>
                </div>
                <Button
                  variant={pubsubEnabled ? 'default' : 'outline'}
                  onClick={() => setCfg((p) => ({ ...p, pubsub: { ...(p.pubsub ?? {}), enabled: !pubsubEnabled } }))}
                >
                  {pubsubEnabled ? 'Enabled' : 'Disabled'}
                </Button>
              </div>

              {pubsubEnabled && (
                <div className="space-y-4 rounded-lg border p-4">
                  <div className="flex items-center justify-between">
                    <p className="text-sm font-medium">GCP Pub/Sub configuration</p>
                    {cfg.pubsub?.service_account_json ? (
                      <Badge variant="secondary">Service account loaded</Badge>
                    ) : (
                      <Badge variant="outline">No credentials</Badge>
                    )}
                  </div>

                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Project ID</label>
                      <Input
                        value={cfg.pubsub?.project_id ?? ''}
                        onChange={(e) =>
                          setCfg((p) => ({ ...p, pubsub: { ...(p.pubsub ?? {}), project_id: e.target.value } }))
                        }
                        placeholder="my-gcp-project"
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Service account JSON</label>
                      <Input type="file" accept="application/json" onChange={handleServiceAccountUpload} />
                    </div>
                  </div>

                  <div className="space-y-3">
                    <p className="text-xs font-semibold text-muted-foreground uppercase">Per-channel topics/subscriptions</p>
                    <div className="grid grid-cols-1 gap-3">
                      {channelRows.map((r) => (
                        <div key={r.ch} className="grid grid-cols-1 md:grid-cols-3 gap-3 items-center">
                          <div className="text-sm font-medium capitalize">{r.ch}</div>
                          <Input
                            placeholder="topic (optional)"
                            value={r.topic}
                            onChange={(e) =>
                              setCfg((p) => ({
                                ...p,
                                pubsub: {
                                  ...(p.pubsub ?? {}),
                                  channels: {
                                    ...(p.pubsub?.channels ?? {}),
                                    [r.ch]: { ...(p.pubsub?.channels?.[r.ch] ?? {}), topic: e.target.value },
                                  },
                                },
                              }))
                            }
                          />
                          <Input
                            placeholder="subscription"
                            value={r.subscription}
                            onChange={(e) =>
                              setCfg((p) => ({
                                ...p,
                                pubsub: {
                                  ...(p.pubsub ?? {}),
                                  channels: {
                                    ...(p.pubsub?.channels ?? {}),
                                    [r.ch]: { ...(p.pubsub?.channels?.[r.ch] ?? {}), subscription: e.target.value },
                                  },
                                },
                              }))
                            }
                          />
                        </div>
                      ))}
                    </div>
                  </div>
                </div>
              )}
            </>
          )}
        </CardContent>
        <CardFooter className="bg-muted/50 py-3 flex justify-end">
          <Button disabled={saving || loading} onClick={save}>
            {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
            Save ingress settings
          </Button>
        </CardFooter>
      </Card>
    </div>
  )
}

