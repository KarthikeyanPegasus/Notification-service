'use client'

import { useMaybeAuth, useMaybeUser } from '@/components/auth/maybe-clerk'

export type UserRole = 'admin' | 'manager' | 'dev' | 'support' | ''

export function useUserRole(): UserRole {
  const { user } = useMaybeUser()
  const { sessionClaims } = useMaybeAuth()

  // Primary source: publicMetadata on the Clerk user object.
  // useUser() always has the up-to-date metadata even without a JWT Template.
  const metaRole = user?.publicMetadata?.role as UserRole | undefined
  if (metaRole) return metaRole

  // Fallback: sessionClaims (works only when a Clerk JWT Template injects publicMetadata).
  const claims = sessionClaims as any
  const claimsRole = (
    claims?.publicMetadata?.role ??
    claims?.metadata?.role ??
    claims?.role
  ) as UserRole | undefined
  return claimsRole ?? ''
}

export function useIsAdmin(): boolean {
  return useUserRole() === 'admin'
}
