'use client'

import { useEffect, useMemo, useState, useRef } from 'react'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { getAuthUser, listMyClients, type ApiClientKey } from '@/lib/api'
import { useClientScope } from '@/lib/client-scope'

export function ClientScopeSelect({
  className,
  onScopeChange,
  includeAll,
}: {
  className?: string
  onScopeChange?: (apiKeyId?: string) => void
  includeAll?: boolean
}) {
  const authUser = getAuthUser()
  const role = authUser?.role ?? 'support'

  // Avoid state fights between selectors that allow "all" and selectors that don't.
  // (They share localStorage otherwise, which causes values like "all" to be normalized away on other pages.)
  const storageKey = includeAll ? 'notifyhub-scope-api-key-id:all' : 'notifyhub-scope-api-key-id:single'
  const { scopeApiKeyId, setScopeApiKeyId } = useClientScope(
    includeAll ? 'all' : role === 'admin' ? 'global' : '',
    storageKey,
  )
  const [clients, setClients] = useState<ApiClientKey[]>([])

  useEffect(() => {
    ;(async () => {
      try {
        const ks = await listMyClients()
        setClients(ks ?? [])
      } catch {
        setClients([])
      }
    })()
  }, [])

  const allowGlobal = role === 'admin'
  const allowAll = Boolean(includeAll)

  // Ensure scope is valid for current role/client list
  useEffect(() => {
    // If a different page stored 'all' but this selector doesn't allow it, normalize.
    if (!allowAll && scopeApiKeyId === 'all') {
      if (allowGlobal) {
        setScopeApiKeyId('global')
      } else if (clients.length > 0) {
        setScopeApiKeyId(clients[0].id)
      }
      return
    }

    if (allowGlobal) {
      if (!scopeApiKeyId) {
        setScopeApiKeyId(allowAll ? 'all' : 'global')
        return
      }
      if (scopeApiKeyId === 'global') return
      if (allowAll && scopeApiKeyId === 'all') return
      if (clients.some((c) => c.id === scopeApiKeyId)) return
      // Unknown/removed client id: fall back to first available client, else global.
      setScopeApiKeyId(clients[0]?.id ?? 'global')
      return
    }

    if (scopeApiKeyId === 'global') {
      if (allowAll) {
        setScopeApiKeyId('all')
      } else if (clients.length > 0) {
        setScopeApiKeyId(clients[0].id)
      }
      return
    }
    if (scopeApiKeyId && clients.some((c) => c.id === scopeApiKeyId)) return
    if (allowAll && scopeApiKeyId === 'all') return
    if (clients.length > 0) setScopeApiKeyId(clients[0].id)
  }, [allowAll, allowGlobal, clients, scopeApiKeyId, setScopeApiKeyId])

  const apiKeyId = useMemo(() => {
    if (scopeApiKeyId === 'all') return undefined
    if (allowGlobal && scopeApiKeyId === 'global') return undefined
    return scopeApiKeyId || undefined
  }, [allowGlobal, scopeApiKeyId])

  const lastReportedId = useRef<string | undefined>(undefined)

  useEffect(() => {
    if (apiKeyId !== lastReportedId.current) {
      lastReportedId.current = apiKeyId
      onScopeChange?.(apiKeyId)
    }
  }, [apiKeyId, onScopeChange])

  return (
    <div className={className}>
      <Select value={scopeApiKeyId} onValueChange={setScopeApiKeyId}>
        <SelectTrigger className="bg-card/50">
          <SelectValue placeholder="Scope" />
        </SelectTrigger>
        <SelectContent>
          {allowAll && <SelectItem value="all">All (my clients)</SelectItem>}
          {allowGlobal && !allowAll && <SelectItem value="global">Global (default)</SelectItem>}
          {clients.map((c) => (
            <SelectItem key={c.id} value={c.id}>
              {c.name} ({c.prefix})
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}

