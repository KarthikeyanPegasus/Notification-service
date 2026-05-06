'use client'

import React, { createContext, useContext, useMemo } from 'react'

/**
 * Mock Clerk provider — used when NEXT_PUBLIC_CLERK_ENABLED=false.
 * Provides a fake authenticated session so the UI can be developed
 * and tested without a Clerk account.
 */

interface MockUser {
  id: string
  fullName: string
  username: string
  primaryEmailAddress: { emailAddress: string }
  publicMetadata: { role: string }
}

interface MockSession {
  getToken(): Promise<string>
}

interface MockClerkContextValue {
  user: MockUser | null
  isLoaded: boolean
  isSignedIn: boolean
  session: MockSession | null
}

const MockClerkContext = createContext<MockClerkContextValue>({
  user: null,
  isLoaded: true,
  isSignedIn: true,
  session: null,
})

export function MockClerkProvider({ children }: { children: React.ReactNode }) {
  const value = useMemo<MockClerkContextValue>(() => ({
    user: {
      id: 'mock-user-id',
      fullName: 'Mock Admin',
      username: 'mock-admin',
      primaryEmailAddress: { emailAddress: 'admin@mock.local' },
      publicMetadata: { role: 'admin' },
    },
    isLoaded: true,
    isSignedIn: true,
    session: {
      async getToken() {
        return 'mock-clerk-token'
      },
    },
  }), [])

  return (
    <MockClerkContext.Provider value={value}>
      <div id="mock-clerk-root">
        {/* Expose Clerk on window so api.ts can find it */}
        <MockClerkScript />
        {children}
      </div>
    </MockClerkContext.Provider>
  )
}

function MockClerkScript() {
  if (typeof window !== 'undefined') {
    (window as any).Clerk = {
      session: {
        getToken: async () => 'mock-clerk-token',
      },
    }
  }
  return null
}

// ── Mock hooks matching Clerk's useAuth, useUser, useClerk ──────────────────

export function useMockAuth() {
  const ctx = useContext(MockClerkContext)
  return {
    isLoaded: ctx.isLoaded,
    isSignedIn: ctx.isSignedIn,
    sessionId: 'mock-session',
    sessionClaims: {
      publicMetadata: { role: 'admin' },
    } as any,
    getToken: ctx.session?.getToken ?? (async () => 'mock-clerk-token'),
    signOut: async () => { /* no-op */ },
  }
}

export function useMockUser() {
  const ctx = useContext(MockClerkContext)
  return {
    isLoaded: ctx.isLoaded,
    isSignedIn: ctx.isSignedIn,
    user: ctx.user,
  }
}

export function useMockClerk() {
  return {
    signOut: async () => { /* no-op */ },
    openSignIn: async () => { /* no-op */ },
  }
}
