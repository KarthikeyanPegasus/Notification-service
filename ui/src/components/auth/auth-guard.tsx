'use client'

import { useAuth, RedirectToSignIn } from '@clerk/nextjs'
import { usePathname } from 'next/navigation'
import { useMemo } from 'react'
import { useMockAuth } from './mock-clerk-provider'

// When Clerk is disabled (NEXT_PUBLIC_CLERK_ENABLED=false), fall back to mock auth.
function useMaybeAuth() {
  const isClerkEnabled =
    (typeof process !== 'undefined' && process.env?.NEXT_PUBLIC_CLERK_ENABLED !== 'false')

  // eslint-disable-next-line react-hooks/rules-of-hooks
  const clerkAuth = isClerkEnabled ? useAuth() : useMockAuth()

  return useMemo(() => ({
    isLoaded: clerkAuth.isLoaded,
    isSignedIn: clerkAuth.isSignedIn,
  }), [clerkAuth.isLoaded, clerkAuth.isSignedIn])
}

const PUBLIC_ROUTES = new Set<string>(['/login', '/sign-in', '/sign-up'])

export function AuthGuard({ children }: { children: React.ReactNode }) {
  const pathname = usePathname() ?? '/'
  const { isSignedIn, isLoaded } = useMaybeAuth()

  // Allow public routes without auth
  if (PUBLIC_ROUTES.has(pathname)) {
    return <>{children}</>
  }

  // Wait for auth to load
  if (!isLoaded) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-muted border-t-primary" />
      </div>
    )
  }

  if (!isSignedIn) {
    return <RedirectToSignIn />
  }

  return <>{children}</>
}
