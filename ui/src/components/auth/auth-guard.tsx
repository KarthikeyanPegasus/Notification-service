'use client'

import { useEffect, useState } from 'react'
import { usePathname, useRouter } from 'next/navigation'
import { getAuthToken } from '@/lib/api'

const PUBLIC_ROUTES = new Set<string>(['/login'])

export function AuthGuard({ children }: { children: React.ReactNode }) {
  const router = useRouter()
  const pathname = usePathname() ?? '/'
  const [mounted, setMounted] = useState(false)

  useEffect(() => {
    setMounted(true)
  }, [])

  useEffect(() => {
    if (!mounted) return
    if (PUBLIC_ROUTES.has(pathname)) return
    const tok = getAuthToken()
    if (!tok) {
      router.replace('/login')
    }
  }, [mounted, pathname, router])

  if (PUBLIC_ROUTES.has(pathname)) return <>{children}</>
  // Avoid hydration mismatch: on the server we can't read localStorage, so we
  // wait until the client is mounted before deciding what to render.
  if (!mounted) return null
  const tok = getAuthToken()
  if (!tok) return null
  return <>{children}</>
}

