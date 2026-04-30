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
import type { VendorRateLimit } from '@/types'
import { deleteVendorRateLimitScoped, getVendorRateLimitsScoped, upsertVendorRateLimitScoped } from '@/lib/api'

type RateLimitDraft = {
  rps: string
  per_minute: string
  per_10_min: string
  per_hour: string
  per_day: string
}

function toDraftValue(v: number | null | undefined): string {
  if (v === null || v === undefined) return ''
  if (typeof v === 'number') return String(v)
  return ''
}

function parseOptionalNumber(v: string): number | null {
  const trimmed = v.trim()
  if (!trimmed) return null
  const n = Number(trimmed)
  if (Number.isNaN(n)) return null
  return n
}

export function VendorRateLimits({
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
  const queryKey = useMemo(() => ['vendor-rate-limits', apiKeyId ?? 'global'], [apiKeyId])

  const { data, isLoading, error } = useQuery({
    queryKey,
    queryFn: () => getVendorRateLimitsScoped(apiKeyId),
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

  const limitsByVendor = useMemo(() => {
    const m = new Map<string, VendorRateLimit>()
    ;(data ?? []).forEach((l) => m.set(l.vendor_name, l))
    return m
  }, [data])

  const [draft, setDraft] = useState<Record<string, RateLimitDraft>>({})
  useEffect(() => {
    const next: Record<string, RateLimitDraft> = {}
    for (const vendorName of effectiveVendorIds) {
      const existing = limitsByVendor.get(vendorName)
      next[vendorName] = {
        rps: toDraftValue(existing?.rps ?? null),
        per_minute: toDraftValue(existing?.per_minute ?? null),
        per_10_min: toDraftValue(existing?.per_10_min ?? null),
        per_hour: toDraftValue(existing?.per_hour ?? null),
        per_day: toDraftValue(existing?.per_day ?? null),
      }
    }
    setDraft(next)
  }, [effectiveVendorIds, limitsByVendor])

  const [savingVendor, setSavingVendor] = useState<string | null>(null)

  const saveVendor = async (vendorName: string) => {
    if (!canEdit) return
    const d = draft[vendorName]
    if (!d) return

    setSavingVendor(vendorName)
    try {
      await upsertVendorRateLimitScoped(
        vendorName,
        {
          rps: parseOptionalNumber(d.rps),
          per_minute: parseOptionalNumber(d.per_minute),
          per_10_min: parseOptionalNumber(d.per_10_min),
          per_hour: parseOptionalNumber(d.per_hour),
          per_day: parseOptionalNumber(d.per_day),
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
    const ok = window.confirm(`Remove rate limit for ${vendorName}?`)
    if (!ok) return
    setSavingVendor(vendorName)
    try {
      await deleteVendorRateLimitScoped(vendorName, apiKeyId)
      await queryClient.invalidateQueries({ queryKey })
    } finally {
      setSavingVendor(null)
    }
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between gap-3">
          <div>
            <CardTitle className="text-lg">Vendor rate limits</CardTitle>
            <CardDescription>Control per-vendor throughput caps (optional windows). Changes apply immediately.</CardDescription>
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
                <TableHead className="w-[120px] text-right">RPS</TableHead>
                <TableHead className="w-[140px] text-right">Per minute</TableHead>
                <TableHead className="w-[140px] text-right">Per 10 min</TableHead>
                <TableHead className="w-[140px] text-right">Per hour</TableHead>
                <TableHead className="w-[140px] text-right">Per day</TableHead>
                <TableHead className="w-[160px] text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {effectiveVendorIds.map((vendorName) => {
                const existing = limitsByVendor.get(vendorName)
                const isSaving = savingVendor === vendorName
                const v = vendorById.get(vendorName)
                const d = draft[vendorName] ?? {
                  rps: '',
                  per_minute: '',
                  per_10_min: '',
                  per_hour: '',
                  per_day: '',
                }

                if (!v) return null

                return (
                  <TableRow key={vendorName}>
                    <TableCell className="font-medium">
                      <div className="flex items-center gap-2">
                        <v.icon className="h-4 w-4" />
                        <span className="capitalize">{v.name}</span>
                        {!existing && <Badge variant="outline" className="ml-2">none</Badge>}
                      </div>
                    </TableCell>
                    <TableCell>
                      <Input
                        value={d.rps}
                        disabled={!canEdit || isSaving}
                        onChange={(e) => setDraft((prev) => ({ ...prev, [vendorName]: { ...d, rps: e.target.value } }))}
                        inputMode="decimal"
                        step="0.01"
                      />
                    </TableCell>
                    <TableCell>
                      <Input
                        value={d.per_minute}
                        disabled={!canEdit || isSaving}
                        onChange={(e) => setDraft((prev) => ({ ...prev, [vendorName]: { ...d, per_minute: e.target.value } }))}
                        inputMode="numeric"
                      />
                    </TableCell>
                    <TableCell>
                      <Input
                        value={d.per_10_min}
                        disabled={!canEdit || isSaving}
                        onChange={(e) => setDraft((prev) => ({ ...prev, [vendorName]: { ...d, per_10_min: e.target.value } }))}
                        inputMode="numeric"
                      />
                    </TableCell>
                    <TableCell>
                      <Input
                        value={d.per_hour}
                        disabled={!canEdit || isSaving}
                        onChange={(e) => setDraft((prev) => ({ ...prev, [vendorName]: { ...d, per_hour: e.target.value } }))}
                        inputMode="numeric"
                      />
                    </TableCell>
                    <TableCell>
                      <Input
                        value={d.per_day}
                        disabled={!canEdit || isSaving}
                        onChange={(e) => setDraft((prev) => ({ ...prev, [vendorName]: { ...d, per_day: e.target.value } }))}
                        inputMode="numeric"
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

