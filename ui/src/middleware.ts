// ── Clerk Auth Middleware ──────────────────────────────────────────────────
// When NEXT_PUBLIC_CLERK_ENABLED=false (local dev without Clerk), this
// middleware is bypassed — a mock auth provider handles auth on the client.
// See: components/auth/mock-clerk-provider.tsx

import { clerkMiddleware, createRouteMatcher } from '@clerk/nextjs/server'

const isClerkEnabled = (process.env.NEXT_PUBLIC_CLERK_ENABLED ?? 'true') === 'true'

const isPublicRoute = createRouteMatcher([
  '/login(.*)',
  '/sign-in(.*)',
  '/sign-up(.*)',
])

// When Clerk is disabled, allow all requests through (mock auth on client)
const clerkMiddlewareHandler = isClerkEnabled
  ? clerkMiddleware(async (auth, request) => {
      if (!isPublicRoute(request)) {
        await auth.protect()
      }
    })
  : () => {}

export default clerkMiddlewareHandler

export const config = {
  matcher: [
    // Skip Next.js internals and all static files
    '/((?!_next/static|_next/image|favicon.ico|.*\\.(?:svg|png|jpg|jpeg|gif|webp)$).*)',
  ],
}
