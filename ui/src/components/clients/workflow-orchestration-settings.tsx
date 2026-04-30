'use client'

import React, { useEffect, useState } from 'react'
import { Card, CardContent, CardDescription, CardFooter, CardHeader } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Loader2, Save, Workflow } from 'lucide-react'
import { getVendorConfigsScoped, updateVendorConfigScoped } from '@/lib/api'

type Provider = 'temporal' | 'cadence'

type WorkflowOrchestrationConfig = {
  provider?: Provider
  temporal?: {
    host_port?: string
    namespace?: string
    frontend_url?: string
  }
  cadence?: {
    host_port?: string
    domain?: string
    frontend_url?: string
  }
}

const DEFAULT_CFG: WorkflowOrchestrationConfig = {
  provider: 'temporal',
  temporal: { host_port: 'localhost:7233', namespace: 'default', frontend_url: '' },
  cadence: { host_port: 'localhost:7233', domain: 'default', frontend_url: '' },
}

export function WorkflowOrchestrationSettings({ apiKeyId }: { apiKeyId?: string }) {
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [cfg, setCfg] = useState<WorkflowOrchestrationConfig>(DEFAULT_CFG)

  const load = async () => {
    setLoading(true)
    setError(null)
    setSuccess(null)
    try {
      const configs = await getVendorConfigsScoped(apiKeyId)
      const existing = configs.find((c) => c.vendor_type === 'workflow_orchestration')
      if (existing?.config_json) {
        setCfg((prev) => ({
          ...prev,
          ...(existing.config_json as WorkflowOrchestrationConfig),
          temporal: { ...(prev.temporal ?? {}), ...((existing.config_json as any)?.temporal ?? {}) },
          cadence: { ...(prev.cadence ?? {}), ...((existing.config_json as any)?.cadence ?? {}) },
        }))
      } else {
        setCfg(DEFAULT_CFG)
      }
    } catch (e) {
      console.error(e)
      setError('Failed to load workflow orchestration settings.')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiKeyId])

  const save = async () => {
    setSaving(true)
    setError(null)
    setSuccess(null)
    try {
      await updateVendorConfigScoped('workflow_orchestration', cfg, apiKeyId)
      setSuccess('Workflow orchestration settings saved.')
      setTimeout(() => setSuccess(null), 2500)
    } catch (e) {
      console.error(e)
      setError('Failed to save workflow orchestration settings.')
    } finally {
      setSaving(false)
    }
  }

  const provider = (cfg.provider ?? 'temporal') as Provider

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
            <Workflow className="h-5 w-5 text-primary" aria-hidden />
            <div>
              <div className="text-base font-semibold">Workflow orchestration client</div>
              <CardDescription>Choose Temporal or Cadence settings for this client scope.</CardDescription>
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
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                <Button
                  type="button"
                  variant={provider === 'temporal' ? 'default' : 'outline'}
                  onClick={() => setCfg((p) => ({ ...p, provider: 'temporal' }))}
                >
                  Temporal
                </Button>
                <Button
                  type="button"
                  variant={provider === 'cadence' ? 'default' : 'outline'}
                  onClick={() => setCfg((p) => ({ ...p, provider: 'cadence' }))}
                >
                  Cadence
                </Button>
              </div>

              {provider === 'temporal' ? (
                <div className="space-y-4 rounded-lg border p-4">
                  <p className="text-sm font-medium">Temporal</p>
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Frontend host:port (gRPC)</label>
                      <Input
                        value={cfg.temporal?.host_port ?? ''}
                        onChange={(e) =>
                          setCfg((p) => ({ ...p, temporal: { ...(p.temporal ?? {}), host_port: e.target.value } }))
                        }
                        placeholder="temporal.example.com:7233"
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Namespace</label>
                      <Input
                        value={cfg.temporal?.namespace ?? ''}
                        onChange={(e) =>
                          setCfg((p) => ({ ...p, temporal: { ...(p.temporal ?? {}), namespace: e.target.value } }))
                        }
                        placeholder="default"
                      />
                    </div>
                    <div className="space-y-2 md:col-span-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Frontend URL (UI)</label>
                      <Input
                        value={cfg.temporal?.frontend_url ?? ''}
                        onChange={(e) =>
                          setCfg((p) => ({ ...p, temporal: { ...(p.temporal ?? {}), frontend_url: e.target.value } }))
                        }
                        placeholder="https://temporal-ui.example.com"
                      />
                    </div>
                  </div>
                </div>
              ) : (
                <div className="space-y-4 rounded-lg border p-4">
                  <p className="text-sm font-medium">Cadence</p>
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Frontend host:port</label>
                      <Input
                        value={cfg.cadence?.host_port ?? ''}
                        onChange={(e) =>
                          setCfg((p) => ({ ...p, cadence: { ...(p.cadence ?? {}), host_port: e.target.value } }))
                        }
                        placeholder="cadence.example.com:7233"
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Domain</label>
                      <Input
                        value={cfg.cadence?.domain ?? ''}
                        onChange={(e) =>
                          setCfg((p) => ({ ...p, cadence: { ...(p.cadence ?? {}), domain: e.target.value } }))
                        }
                        placeholder="default"
                      />
                    </div>
                    <div className="space-y-2 md:col-span-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Frontend URL (UI)</label>
                      <Input
                        value={cfg.cadence?.frontend_url ?? ''}
                        onChange={(e) =>
                          setCfg((p) => ({ ...p, cadence: { ...(p.cadence ?? {}), frontend_url: e.target.value } }))
                        }
                        placeholder="https://cadence-web.example.com"
                      />
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
            Save workflow settings
          </Button>
        </CardFooter>
      </Card>
    </div>
  )
}

