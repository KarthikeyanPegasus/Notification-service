'use client'

import { useRouter } from 'next/navigation'
import { useMaybeClerk, useMaybeUser } from '@/components/auth/maybe-clerk'
import { Button } from '@/components/ui/button'
import { clearAuthToken, getAuthUser } from '@/lib/api'
import { effectiveRole } from '@/lib/role'

export default function ProfilePage() {
  const router = useRouter()
  const { user: clerkUser, isLoaded } = useMaybeUser()
  const clerk = useMaybeClerk()

  // Use Clerk user info if available, fall back to legacy localStorage
  const legacyUser = getAuthUser()

  const name = clerkUser?.fullName ?? clerkUser?.username ?? legacyUser?.name ?? '-'
  const email = clerkUser?.primaryEmailAddress?.emailAddress ?? legacyUser?.email ?? '-'
  const role = effectiveRole({ clerkUser, legacyRole: legacyUser?.role, fallback: 'support' })

  const logout = () => {
    clearAuthToken()
    if (clerkUser) {
      clerk.signOut()
    } else {
      router.replace('/login')
    }
  }

  if (!isLoaded) {
    return <div className="max-w-2xl"><h1 className="text-2xl font-semibold tracking-tight">Profile</h1><p className="mt-4 text-sm text-muted-foreground">Loading...</p></div>
  }

  return (
    <div className="max-w-2xl">
      <h1 className="text-2xl font-semibold tracking-tight">Profile</h1>
      <p className="mt-1 text-sm text-muted-foreground">Your current session details.</p>

      <div className="mt-6 rounded-lg border bg-card p-5">
        <div className="grid gap-4">
          <div>
            <p className="text-xs text-muted-foreground">Name</p>
            <p className="text-sm font-medium">{name}</p>
          </div>
          <div>
            <p className="text-xs text-muted-foreground">Email</p>
            <p className="text-sm font-medium">{email}</p>
          </div>
          <div>
            <p className="text-xs text-muted-foreground">Role</p>
            <p className="text-sm font-medium capitalize">{role}</p>
          </div>
          {clerkUser && (
            <div>
              <p className="text-xs text-muted-foreground">Clerk ID</p>
              <p className="text-sm font-medium font-mono">{clerkUser.id}</p>
            </div>
          )}
        </div>

        <div className="mt-6 flex gap-2">
          <Button variant="outline" onClick={() => router.back()}>
            Back
          </Button>
          <Button variant="destructive" onClick={logout}>
            Log out
          </Button>
        </div>
      </div>
    </div>
  )
}
