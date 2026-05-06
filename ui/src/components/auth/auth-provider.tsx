'use client'

import dynamic from 'next/dynamic'

const isClerkEnabled =
  (typeof process !== 'undefined' && process.env?.NEXT_PUBLIC_CLERK_ENABLED !== 'false') ||
  (typeof window !== 'undefined' &&
    (window as any).__NEXT_DATA__?.props?.pageProps?.__CLERK_ENABLED !== false)

// Dynamically import only the provider we need at runtime.
const RealClerkProvider = dynamic(
  () => import('@clerk/nextjs').then((m) => {
    const Provider = ({ children }: { children: React.ReactNode }) => (
      <m.ClerkProvider>{children}</m.ClerkProvider>
    )
    Provider.displayName = 'ClerkProvider'
    return Provider
  }),
  { ssr: true },
)

const MockClerkProvider = dynamic(
  () => import('./mock-clerk-provider').then((m) => {
    const Provider = ({ children }: { children: React.ReactNode }) => (
      <m.MockClerkProvider>{children}</m.MockClerkProvider>
    )
    Provider.displayName = 'MockClerkProvider'
    return Provider
  }),
  { ssr: true },
)

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const Provider = isClerkEnabled ? RealClerkProvider : MockClerkProvider
  return <Provider>{children}</Provider>
}
