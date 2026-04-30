'use client'

import React, { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Loader2, Users } from 'lucide-react'
import { getVendorConfigsScoped, updateVendorConfigScoped } from '@/lib/api'

type Channel = 'email' | 'sms' | 'push' | 'webhook' | 'websocket' | 'slack'

type ChannelWorkerPool = {
  min_workers: number
  max_workers: number
}

type WorkerPoolConfig = {
  email: ChannelWorkerPool
  sms: ChannelWorkerPool
  push: ChannelWorkerPool
  webhook: ChannelWorkerPool
  websocket: ChannelWorkerPool
  slack: ChannelWorkerPool
}

const DEFAULT_POOL: WorkerPoolConfig = {
  email: { min_workers: 1, max_workers: 3 },
  sms: { min_workers: 1, max_workers: 3 },
  push: { min_workers: 1, max_workers: 3 },
  webhook: { min_workers: 1, max_workers: 3 },
  websocket: { min_workers: 1, max_workers: 3 },
  slack: { min_workers: 1, max_workers: 3 },
}

function normalizeChannelPool(v: any, fallback: ChannelWorkerPool): ChannelWorkerPool {
  const min = Number(v?.min_workers ?? fallback.min_workers)
  const max = Number(v?.max_workers ?? fallback.max_workers)
  const minC = Number.isFinite(min) ? min : fallback.min_workers
  const maxC = Number.isFinite(max) ? max : fallback.max_workers
  if (maxC < minC) return { min_workers: maxC, max_workers: minC }
  return { min_workers: minC, max_workers: maxC }
}

export function WorkersPerChannelSettings({ apiKeyId }: { apiKeyId?: string }) {
  const [cfg, setCfg] = useState<WorkerPoolConfig>(DEFAULT_POOL)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['vendor-configs', apiKeyId ?? 'global'],
    queryFn: () => getVendorConfigsScoped(apiKeyId),
    retry: 1,
  })

  useEffect(() => {
    setError(null)
    setSuccess(null)

    const existing = (data ?? []).find((c) => c.vendor_type === 'worker_pool')
    if (!existing?.config_json) {
      setCfg(DEFAULT_POOL)
      return
    }

    try {
      const raw = existing.config_json as Partial<WorkerPoolConfig>
      setCfg({
        email: normalizeChannelPool(raw.email, DEFAULT_POOL.email),
        sms: normalizeChannelPool(raw.sms, DEFAULT_POOL.sms),
        push: normalizeChannelPool(raw.push, DEFAULT_POOL.push),
        webhook: normalizeChannelPool(raw.webhook, DEFAULT_POOL.webhook),
        websocket: normalizeChannelPool(raw.websocket, DEFAULT_POOL.websocket),
        slack: normalizeChannelPool(raw.slack, DEFAULT_POOL.slack),
      })
    } catch {
      setCfg(DEFAULT_POOL)
    }
  }, [data])

  const canSave = useMemo(() => {
    // Basic invariants: numeric and max >= min (enforced by normalizeChannelPool).
    return Boolean(cfg?.email && cfg?.sms && cfg?.push && cfg?.webhook && cfg?.websocket && cfg?.slack)
  }, [cfg])

  const save = async () => {
    setSaving(true)
    setError(null)
    setSuccess(null)
    try {
      await updateVendorConfigScoped('worker_pool', cfg, apiKeyId)
      setSuccess('Worker pool settings saved.')
      setTimeout(() => setSuccess(null), 2500)
    } catch (e: any) {
      setError(e?.message ? String(e.message) : 'Failed to save worker pool settings.')
      setTimeout(() => setError(null), 4000)
    } finally {
      setSaving(false)
    }
  }

  const channelRows: { id: Channel; label: string; desc: string }[] = [
    { id: 'sms', label: 'SMS workers', desc: 'Min/max worker pool size for the SMS channel.' },
    { id: 'email', label: 'Email workers', desc: 'Min/max worker pool size for the Email channel.' },
    { id: 'push', label: 'Push workers', desc: 'Min/max worker pool size for the Push channel.' },
    { id: 'webhook', label: 'Webhook workers', desc: 'Min/max worker pool size for outbound webhooks.' },
    { id: 'websocket', label: 'WebSocket workers', desc: 'Min/max worker pool size for in-app/websocket notifications.' },
    { id: 'slack', label: 'Slack workers', desc: 'Min/max worker pool size for Slack channel notifications.' },
  ]

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <Users className="h-5 w-5 text-primary" aria-hidden />
          <div>
            <CardTitle className="text-lg">Workers per channel</CardTitle>
            <CardDescription>
              Set min/max worker tiers per channel. (In this system, worker tiers map to the `high/medium/low` priority
              task queues.)
            </CardDescription>
          </div>
        </div>
      </CardHeader>

      <CardContent className="space-y-4">
        {error && <p className="text-sm text-destructive rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3">{error}</p>}
        {success && <p className="text-sm text-green-700 rounded-lg border border-green-500/30 bg-green-500/10 px-4 py-3">{success}</p>}

        {isLoading ? (
          <div className="flex items-center justify-center py-10 text-muted-foreground">
            <Loader2 className="h-6 w-6 animate-spin" />
          </div>
        ) : (
          <div className="space-y-5">
            {channelRows.map((row) => {
              const v = cfg[row.id]
              return (
                <div key={row.id} className="rounded-lg border p-4 space-y-3">
                  <div className="flex items-start justify-between gap-4">
                    <div>
                      <p className="text-sm font-medium">{row.label}</p>
                      <p className="text-xs text-muted-foreground">{row.desc}</p>
                    </div>
                  </div>

                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Min workers</label>
                      <Input
                        type="number"
                        min={0}
                        step={1}
                        value={String(v.min_workers ?? 0)}
                        onChange={(e) => {
                          const n = Number(e.target.value)
                          setCfg((p) => ({ ...p, [row.id]: { ...p[row.id], min_workers: n } }))
                        }}
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-xs font-semibold text-muted-foreground uppercase">Max workers</label>
                      <Input
                        type="number"
                        min={0}
                        step={1}
                        value={String(v.max_workers ?? 0)}
                        onChange={(e) => {
                          const n = Number(e.target.value)
                          setCfg((p) => ({ ...p, [row.id]: { ...p[row.id], max_workers: n } }))
                        }}
                      />
                    </div>
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </CardContent>

      <CardFooter className="bg-muted/50 py-3 flex justify-end">
        <Button disabled={saving || !canSave} onClick={save}>
          {saving ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : null}
          Save worker pool settings
        </Button>
      </CardFooter>
    </Card>
  )
}

