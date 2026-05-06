export type Role = 'admin' | 'manager' | 'dev' | 'support'

export function normalizeRole(value: unknown): Role | null {
  const v = typeof value === 'string' ? value.trim().toLowerCase() : ''
  if (v === 'admin' || v === 'manager' || v === 'dev' || v === 'support') return v
  return null
}

// Clerk: role is typically stored in `publicMetadata.role`.
// We keep this helper loose-typed to avoid coupling to Clerk SDK types in the UI.
export function roleFromClerkUser(user: any): Role | null {
  if (!user) return null
  return (
    normalizeRole(user?.publicMetadata?.role) ??
    normalizeRole(user?.unsafeMetadata?.role) ??
    normalizeRole(user?.publicMetadata?.Role) ??
    null
  )
}

export function effectiveRole(opts: { clerkUser?: any; legacyRole?: unknown; fallback?: Role }): Role {
  return (
    roleFromClerkUser(opts.clerkUser) ??
    normalizeRole(opts.legacyRole) ??
    opts.fallback ??
    'support'
  )
}

