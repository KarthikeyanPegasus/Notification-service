'use client'

import React, { useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Loader2, Trash2 } from 'lucide-react'
import { VENDORS } from '@/lib/vendors'
import type { VendorRetryConfig } from '@/types'
import { deleteVendorRetryConfigScoped, getVendorRetryConfigsScoped, upsertVendorRetryConfigScoped } from '@/lib/api'

type RetryDraft = {
  retry_initial_interval_ms: string
  retry_max_interval_ms: string
  retry_max_attempts: string
  retry_backoff_coefficient: string
  sla_seconds: string
}

const DEFAULTS: RetryDraft = {
  retry_initial_interval_ms: '100',
  retry_max_interval_ms: '30000',
  retry_max_attempts: '5',
  retry_backoff_coefficient: '2.0',
  sla_seconds: '30',
}

function toDraftValue(v: number | null | undefined, fallback: string): string {
  if (v === null || v === undefined) return fallback
  return String(v)
}

function parseOptionalInt(v: string): number | null {
  const trimmed = v.trim()
  if (!trimmed) return null
  const n = parseInt(trimmed, 10)
  if (Number.isNaN(n)) return null
  return n
}

function parseOptionalFloat(v: string): number | null {
  const trimmed = v.trim()
  if (!trimmed) return null
  const n = parseFloat(trimmed)
  if (Number.isNaN(n)) return null
  return n
}

export function VendorRetryConfig({
  apiKeyId,
  vendorIds,
  canEdit,
  canDelete,
}: {
  apiKeyId?: string
  vendorIds: string[]
  canEdit: boolean
  canDelete: boolean
}) {
  const queryClient = useQueryClient()
  const queryKey = useMemo(() => ['vendor-retry-configs', apiKeyId ?? 'global'], [apiKeyId])

  const { data, isLoading, error } = useQuery({
    queryKey,
    queryFn: () => getVendorRetryConfigsScoped(apiKeyId),
    retry: 1,
  })

  const vendorById = useMemo(() => new Map(VENDORS.map((v) => [v.id, v])), [])

  const effectiveVendorIds = useMemo(() => {
    const out: string[] = []
    for (const id of vendorIds) {
      if (vendorById.has(id)) out.push(id)
    }
    return out
  }, [vendorById, vendorIds])

  const configsByVendor = useMemo(() => {
    const m = new Map<string, VendorRetryConfig>()
    ;(data ?? []).forEach((c) => m.set(c.vendor_name, c))
    return m
  }, [data])

  const [draft, setDraft] = useState<Record<string, RetryDraft>>({})
  useEffect(() => {
    const next: Record<string, RetryDraft> = {}
    for (const vendorName of effectiveVendorIds) {
      const existing = configsByVendor.get(vendorName)
      next[vendorName] = {
        retry_initial_interval_ms: toDraftValue(existing?.retry_initial_interval_ms ?? null, DEFAULTS.retry_initial_interval_ms),
        retry_max_interval_ms: toDraftValue(existing?.retry_max_interval_ms ?? null, DEFAULTS.retry_max_interval_ms),
        retry_max_attempts: toDraftValue(existing?.retry_max_attempts ?? null, DEFAULTS.retry_max_attempts),
        retry_backoff_coefficient: toDraftValue(existing?.retry_backoff_coefficient ?? null, DEFAULTS.retry_backoff_coefficient),
        sla_seconds: toDraftValue(existing?.sla_seconds ?? null, DEFAULTS.sla_seconds),
      }
    }
    setDraft(next)
  }, [effectiveVendorIds, configsByVendor])

  const [savingVendor, setSavingVendor] = useState<string | null>(null)

  const saveVendor = async (vendorName: string) => {
    if (!canEdit) return
    const d = draft[vendorName]
    if (!d) return

    setSavingVendor(vendorName)
    try {
      await upsertVendorRetryConfigScoped(
        vendorName,
        {
          retry_initial_interval_ms: parseOptionalInt(d.retry_initial_interval_ms),
          retry_max_interval_ms: parseOptionalInt(d.retry_max_interval_ms),
          retry_max_attempts: parseOptionalInt(d.retry_max_attempts),
          retry_backoff_coefficient: parseOptionalFloat(d.retry_backoff_coefficient),
          sla_seconds: parseOptionalInt(d.sla_seconds),
        },
        apiKeyId,
      )

      await queryClient.invalidateQueries({ queryKey })
    } finally {
      setSavingVendor(null)
    }
  }

  const removeVendor = async (vendorName: string) => {
    if (!canDelete) return
    const ok = window.confirm(`Remove retry configuration for ${vendorName}? This will revert to defaults.`)
    if (!ok) return
    setSavingVendor(vendorName)
    try {
      await deleteVendorRetryConfigScoped(vendorName, apiKeyId)
      await queryClient.invalidateQueries({ queryKey })
    } finally {
      setSavingVendor(null)
    }
  }

  const tooltipLabels: Record<string, string> = {
    retry_initial_interval_ms: 'Initial backoff delay in milliseconds (e.g. 100 = 100ms)',
    retry_max_interval_ms: 'Maximum backoff delay in milliseconds (e.g. 30000 = 30s)',
    retry_max_attempts: 'Maximum delivery attempts before sending to dead-letter queue',
    retry_backoff_coefficient: 'Exponential backoff multiplier (e.g. 2.0 = doubles each attempt)',
    sla_seconds: 'SLA deadline in seconds per vendor (e.g. 30 = 30 seconds)',
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between gap-3">
          <div>
            <CardTitle className="text-lg">Vendor retry & backoff</CardTitle>
            <CardDescription>
              Control per-vendor retry behaviour (exponential backoff). These apply when the workflow engine
              is set to &quot;go_routines&quot;. Defaults apply if not configured.
            </CardDescription>
          </div>
          {!canEdit && <Badge variant="secondary">Read-only</Badge>}
        </div>
      </CardHeader>
      <CardContent>
        {error && <p className="text-sm text-destructive">{String((error as any)?.message ?? 'Failed to load')}</p>}

        {isLoading && (
          <div className="flex items-center justify-center py-10 text-muted-foreground">
            <Loader2 className="h-6 w-6 animate-spin" />
          </div>
        )}

        {!isLoading && effectiveVendorIds.length === 0 && (
          <div className="text-sm text-muted-foreground">No configured vendors.</div>
        )}

        {!isLoading && effectiveVendorIds.length > 0 && (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Vendor</TableHead>
                <TableHead className="w-[110px] text-right" title={tooltipLabels.retry_initial_interval_ms}>
                  Init ms
                </TableHead>
                <TableHead className="w-[110px] text-right" title={tooltipLabels.retry_max_interval_ms}>
                  Max ms
                </TableHead>
                <TableHead className="w-[100px] text-right" title={tooltipLabels.retry_max_attempts}>
                  Attempts
                </TableHead>
                <TableHead className="w-[100px] text-right" title={tooltipLabels.retry_backoff_coefficient}>
                  Coeff
                </TableHead>
                <TableHead className="w-[110px] text-right" title={tooltipLabels.sla_seconds}>
                  SLA (s)
                </TableHead>
                <TableHead className="w-[160px] text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {effectiveVendorIds.map((vendorName) => {
                const existing = configsByVendor.get(vendorName)
                const isSaving = savingVendor === vendorName
                const v = vendorById.get(vendorName)
                const d = draft[vendorName] ?? DEFAULTS

                if (!v) return null

                return (
                  <TableRow key={vendorName}>
                    <TableCell className="font-medium">
                      <div className="flex items-center gap-2">
                        <v.icon className="h-4 w-4" />
                        <span className="capitalize">{v.name}</span>
                        {!existing && <Badge variant="outline" className="ml-2">defaults</Badge>}
                      </div>
                    </TableCell>
                    <TableCell>
                      <Input
                        value={d.retry_initial_interval_ms}
                        disabled={!canEdit || isSaving}
                        onChange={(e) => setDraft((prev) => ({ ...prev, [vendorName]: { ...d, retry_initial_interval_ms: e.target.value } }))}
                        inputMode="numeric"
                        title={tooltipLabels.retry_initial_interval_ms}
                      />
                    </TableCell>
                    <TableCell>
                      <Input
                        value={d.retry_max_interval_ms}
                        disabled={!canEdit || isSaving}
                        onChange={(e) => setDraft((prev) => ({ ...prev, [vendorName]: { ...d, retry_max_interval_ms: e.target.value } }))}
                        inputMode="numeric"
                        title={tooltipLabels.retry_max_interval_ms}
                      />
                    </TableCell>
                    <TableCell>
                      <Input
                        value={d.retry_max_attempts}
                        disabled={!canEdit || isSaving}
                        onChange={(e) => setDraft((prev) => ({ ...prev, [vendorName]: { ...d, retry_max_attempts: e.target.value } }))}
                        inputMode="numeric"
                        title={tooltipLabels.retry_max_attempts}
                      />
                    </TableCell>
                    <TableCell>
                      <Input
                        value={d.retry_backoff_coefficient}
                        disabled={!canEdit || isSaving}
                        onChange={(e) => setDraft((prev) => ({ ...prev, [vendorName]: { ...d, retry_backoff_coefficient: e.target.value } }))}
                        inputMode="decimal"
                        step="0.1"
                        title={tooltipLabels.retry_backoff_coefficient}
                      />
                    </TableCell>
                    <TableCell>
                      <Input
                        value={d.sla_seconds}
                        disabled={!canEdit || isSaving}
                        onChange={(e) => setDraft((prev) => ({ ...prev, [vendorName]: { ...d, sla_seconds: e.target.value } }))}
                        inputMode="numeric"
                        title={tooltipLabels.sla_seconds}
                      />
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex items-center justify-end gap-2">
                        <Button size="sm" disabled={!canEdit || isSaving} onClick={() => saveVendor(vendorName)}>
                          {isSaving ? <Loader2 className="h-4 w-4 animate-spin" /> : 'Save'}
                        </Button>
                        {existing && canDelete && (
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={isSaving}
                            onClick={() => removeVendor(vendorName)}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  )
}
