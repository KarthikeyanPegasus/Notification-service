'use client'

import { useMemo } from 'react'
import { useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { clearAuthToken, getAuthUser } from '@/lib/api'

export default function ProfilePage() {
  const router = useRouter()
  const user = useMemo(() => getAuthUser(), [])

  const logout = () => {
    clearAuthToken()
    router.replace('/login')
  }

  return (
    <div className="max-w-2xl">
      <h1 className="text-2xl font-semibold tracking-tight">Profile</h1>
      <p className="mt-1 text-sm text-muted-foreground">Your current session details.</p>

      <div className="mt-6 rounded-lg border bg-card p-5">
        <div className="grid gap-4">
          <div>
            <p className="text-xs text-muted-foreground">Name</p>
            <p className="text-sm font-medium">{user?.name ?? '-'}</p>
          </div>
          <div>
            <p className="text-xs text-muted-foreground">Email</p>
            <p className="text-sm font-medium">{user?.email ?? '-'}</p>
          </div>
          <div>
            <p className="text-xs text-muted-foreground">Role</p>
            <p className="text-sm font-medium">{user?.role ?? '-'}</p>
          </div>
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

