'use client'

import { useEffect, useState, useCallback } from 'react'

const STORAGE_KEY = 'notifyhub-scope-api-key-id'

export function getStoredScope(key: string = STORAGE_KEY): string | null {
  if (typeof window === 'undefined') return null
  try {
    return window.localStorage.getItem(key)
  } catch {
    return null
  }
}

export function setStoredScope(v: string, key: string = STORAGE_KEY) {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(key, v)
  } catch {
    /* ignore */
  }
}

// Shared client scope across pages. Value is either:
// - 'all' (if includeAll enabled), 'global' (admin only), or an api key id
export function useClientScope(defaultValue = 'global', storageKey: string = STORAGE_KEY) {
  const [scopeApiKeyId, setRawScopeApiKeyId] = useState<string>(defaultValue)

  useEffect(() => {
    const stored = getStoredScope(storageKey)
    if (stored) setRawScopeApiKeyId(stored)
  }, [storageKey])

  const setScopeApiKeyId = useCallback((v: string) => {
    setRawScopeApiKeyId(v)
    setStoredScope(v, storageKey)
  }, [storageKey])

  return { scopeApiKeyId, setScopeApiKeyId }
}

