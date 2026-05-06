'use client'

import { SignIn } from '@clerk/nextjs'
import { useRouter } from 'next/navigation'
import { useEffect } from 'react'
import { useMaybeAuth } from '@/components/auth/maybe-clerk'

export default function LoginPage() {
  const { isSignedIn } = useMaybeAuth()
  const router = useRouter()

  useEffect(() => {
    if (isSignedIn) {
      router.replace('/dashboard')
    }
  }, [isSignedIn, router])

  const isClerkEnabled =
    (typeof process !== 'undefined' && process.env?.NEXT_PUBLIC_CLERK_ENABLED !== 'false')

  return (
    <div className="min-h-[calc(100vh-8rem)] flex items-center justify-center">
      {isClerkEnabled ? (
        <SignIn routing="hash" />
      ) : (
        <div className="max-w-md rounded-xl border bg-card p-6 text-center">
          <h1 className="text-lg font-semibold">Mock auth enabled</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Clerk is disabled for this environment. You should be redirected automatically.
          </p>
        </div>
      )}
    </div>
  )
}