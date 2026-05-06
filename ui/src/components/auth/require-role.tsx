'use client'

import { useRouter } from 'next/navigation'
import { useEffect } from 'react'
import { useUserRole, type UserRole } from '@/hooks/use-user-role'
import { useMaybeAuth, useMaybeUser } from './maybe-clerk'

interface RequireRoleProps {
  roles: UserRole[]
  children: React.ReactNode
  redirectTo?: string
}

/**
 * Guards a page/section so only users with one of the given roles can see it.
 * Non-matching authenticated users are redirected to /dashboard.
 */
export function RequireRole({ roles, children, redirectTo = '/dashboard' }: RequireRoleProps) {
  const { isLoaded: authLoaded, isSignedIn } = useMaybeAuth()
  const { isLoaded: userLoaded } = useMaybeUser()
  const role = useUserRole()
  const router = useRouter()

  const isLoaded = authLoaded && userLoaded

  useEffect(() => {
    if (!isLoaded) return
    if (!isSignedIn) return
    if (!roles.includes(role)) {
      router.replace(redirectTo)
    }
  }, [isLoaded, isSignedIn, role, roles, router, redirectTo])

  if (!isLoaded || !isSignedIn) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-muted border-t-primary" />
      </div>
    )
  }
  if (!roles.includes(role)) return null

  return <>{children}</>
}
