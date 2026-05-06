'use client'

import Link from 'next/link'
import { usePathname, useRouter } from 'next/navigation'
import {
  LayoutDashboard,
  Bell,
  BarChart3,
  Clock,
  Menu,
  X,
  Settings,
  ShoppingBag,
  ShieldAlert,
  Book,
  KeyRound,
  Users,
  UserCircle2,
  LogOut,
  Moon,
  Sun,
  ChevronDown,
} from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import Image from 'next/image'
import { useMaybeClerk, useMaybeUser } from '@/components/auth/maybe-clerk'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  clearAuthToken,
  getAuthUser,
} from '@/lib/api'
import { effectiveRole, type Role } from '@/lib/role'
import { getStoredTheme, toggleTheme, type ThemeMode } from '@/lib/theme'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

const STORAGE_KEY = 'notifyhub-sidebar-collapsed'

type NavItem = { href: string; label: string; icon: React.ElementType; roles: Role[]; children?: NavItem[]; exact?: boolean }
type NavSection = { id: string; label: string; icon: React.ElementType; items: NavItem[] }

const navSections: NavSection[] = [
  {
    id: 'monitoring',
    label: 'Monitor & analyze',
    icon: LayoutDashboard,
    items: [
      { href: '/dashboard', label: 'Dashboard', icon: LayoutDashboard, roles: ['admin', 'manager', 'dev', 'support'] },
      { href: '/notifications', label: 'Notifications', icon: Bell, roles: ['admin', 'manager', 'dev', 'support'] },
      { href: '/reports', label: 'Reports', icon: BarChart3, roles: ['admin', 'manager', 'dev'] },
    ],
  },
  {
    id: 'delivery',
    label: 'Create & deliver',
    icon: Bell,
    items: [
      { href: '/templates', label: 'Templates', icon: Bell, roles: ['admin', 'manager', 'dev'] },
      { href: '/scheduled', label: 'Scheduled', icon: Clock, roles: ['admin', 'manager', 'dev', 'support'] },
      { href: '/app-store', label: 'App Store', icon: ShoppingBag, roles: ['admin', 'manager', 'dev'] },
    ],
  },
  {
    id: 'governance',
    label: 'Governance & ops',
    icon: ShieldAlert,
    items: [
      { href: '/governance/suppressions', label: 'Suppressions', icon: ShieldAlert, roles: ['admin', 'manager', 'dev', 'support'] },
      { href: '/governance/opt-outs', label: 'Opt-outs', icon: ShieldAlert, roles: ['admin', 'manager', 'dev', 'support'] },
      { href: '/admin/dlq', label: 'Dead-Letter Queue', icon: ShieldAlert, roles: ['admin', 'manager', 'dev', 'support'] },
    ],
  },
  {
    id: 'configuration',
    label: 'Configure workspace',
    icon: Settings,
    items: [
      { href: '/settings', label: 'Settings', icon: Settings, roles: ['admin', 'manager', 'dev'] },
      { href: '/clients', label: 'Client management', icon: KeyRound, roles: ['admin', 'manager', 'dev'] },
      { href: '/people', label: 'People', icon: Users, roles: ['admin', 'dev'] },
      { href: '/admin', label: 'Admin Dashboard', icon: ShieldAlert, roles: ['admin'], exact: true },
      { href: '/admin/api-settings', label: 'API Settings', icon: KeyRound, roles: ['admin'] },
    ],
  },
  {
    id: 'developer',
    label: 'Developer resources',
    icon: Book,
    items: [
      {
        href: '/docs',
        label: 'Docs',
        icon: Book,
        roles: ['admin', 'dev'],
        children: [
          { href: '/docs', label: 'Overview', icon: Book, roles: ['admin', 'dev'], exact: true },
          { href: '/docs/api', label: 'API Reference', icon: Book, roles: ['admin', 'dev'] },
          { href: '/docs/sdk/go', label: 'SDK — Go', icon: Book, roles: ['admin', 'dev'] },
          { href: '/docs/sdk/dotnet', label: 'SDK — .NET', icon: Book, roles: ['admin', 'dev'] },
        ],
      },
    ],
  },
]

function NavLink({
  href,
  label,
  icon: Icon,
  active,
  compact,
  indent,
  onClick,
}: {
  href: string
  label: string
  icon: React.ElementType
  active: boolean
  compact: boolean
  indent?: boolean
  onClick?: () => void
}) {
  return (
    <Link
      href={href}
      title={label}
      onClick={onClick}
      className={cn(
        'flex items-center gap-3 rounded-md py-2 text-sm font-medium transition-colors',
        compact ? 'justify-center px-2' : indent ? 'pl-7 pr-3' : 'px-3',
        active
          ? 'bg-primary text-primary-foreground'
          : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
      )}
    >
      <Icon className="h-4 w-4 shrink-0" aria-hidden />
      <span className={cn(compact && 'sr-only')}>{label}</span>
    </Link>
  )
}

function isItemActive(item: NavItem, pathname: string): boolean {
  const selfActive = item.exact
    ? pathname === item.href
    : pathname === item.href || pathname.startsWith(item.href + '/')
  if (selfActive) return true
  return item.children?.some((child) => isItemActive(child, pathname)) ?? false
}

function filterNavItemsByRole(items: NavItem[], role: Role): NavItem[] {
  return items.flatMap((item) => {
    const children = item.children ? filterNavItemsByRole(item.children, role) : undefined
    if (!item.roles.includes(role) && !children?.length) return []
    return [{ ...item, children }]
  })
}

function CompactNavMenuItem({
  item,
  pathname,
  onClick,
}: {
  item: NavItem
  pathname: string
  onClick?: () => void
}) {
  const Icon = item.icon
  const active = isItemActive(item, pathname)

  if (item.children?.length) {
    return (
      <DropdownMenuSub>
        <DropdownMenuSubTrigger className={cn(active && 'bg-accent text-accent-foreground')}>
          <Icon className="mr-2 h-4 w-4" aria-hidden />
          {item.label}
        </DropdownMenuSubTrigger>
        <DropdownMenuSubContent className="w-48">
          {item.children.map((child) => (
            <CompactNavMenuItem
              key={child.href}
              item={child}
              pathname={pathname}
              onClick={onClick}
            />
          ))}
        </DropdownMenuSubContent>
      </DropdownMenuSub>
    )
  }

  return (
    <DropdownMenuItem asChild className={cn(active && 'bg-accent text-accent-foreground')}>
      <Link href={item.href} onClick={onClick}>
        <Icon className="mr-2 h-4 w-4" aria-hidden />
        {item.label}
      </Link>
    </DropdownMenuItem>
  )
}

function NavGroup({
  item,
  pathname,
  compact,
  onClick,
}: {
  item: NavItem
  pathname: string
  compact: boolean
  onClick?: () => void
}) {
  const isParentActive = isItemActive(item, pathname)
  const [open, setOpen] = useState(isParentActive)

  useEffect(() => {
    if (isParentActive) setOpen(true)
  }, [isParentActive])

  if (!item.children?.length) {
    return (
      <NavLink
        href={item.href}
        label={item.label}
        icon={item.icon}
        active={isParentActive}
        compact={compact}
        onClick={onClick}
      />
    )
  }

  const Icon = item.icon

  return (
    <div>
      <button
        type="button"
        title={item.label}
        onClick={() => setOpen((value) => !value)}
        className={cn(
          'flex w-full items-center gap-3 rounded-md py-2 text-sm font-medium transition-colors',
          compact ? 'justify-center px-2' : 'px-3',
          isParentActive
            ? 'bg-accent text-accent-foreground'
            : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
        )}
        aria-expanded={open}
      >
        <Icon className="h-4 w-4 shrink-0" aria-hidden />
        <span className={cn('flex-1 text-left', compact && 'sr-only')}>{item.label}</span>
        {!compact && (
          <ChevronDown
            className={cn('h-4 w-4 shrink-0 transition-transform', open && 'rotate-180')}
            aria-hidden
          />
        )}
      </button>
      {open && !compact && (
        <div className="mt-0.5 space-y-0.5">
          {item.children.map((child) => (
            <NavLink
              key={child.href}
              href={child.href}
              label={child.label}
              icon={child.icon}
              active={isItemActive(child, pathname)}
              compact={compact}
              indent
              onClick={onClick}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function NavSectionGroup({
  section,
  pathname,
  compact,
  onClick,
}: {
  section: NavSection
  pathname: string
  compact: boolean
  onClick?: () => void
}) {
  const isSectionActive = section.items.some((item) => isItemActive(item, pathname))
  const [open, setOpen] = useState(isSectionActive)
  const Icon = section.icon

  useEffect(() => {
    if (isSectionActive) setOpen(true)
  }, [isSectionActive])

  if (compact) {
    return (
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            title={section.label}
            className={cn(
              'flex w-full items-center justify-center rounded-md px-2 py-2 text-sm font-medium transition-colors outline-none ring-offset-background focus-visible:ring-2 focus-visible:ring-ring',
              isSectionActive
                ? 'bg-primary text-primary-foreground'
                : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
            )}
            aria-label={section.label}
          >
            <Icon className="h-4 w-4 shrink-0" aria-hidden />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent side="right" align="start" className="w-56">
          <DropdownMenuLabel>{section.label}</DropdownMenuLabel>
          <DropdownMenuSeparator />
          {section.items.map((item) => (
            <CompactNavMenuItem
              key={item.href}
              item={item}
              pathname={pathname}
              onClick={onClick}
            />
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    )
  }

  return (
    <div className="space-y-1">
      <button
        type="button"
        onClick={() => setOpen((value) => !value)}
        className={cn(
          'flex w-full items-center gap-2 rounded-md px-3 py-2 text-xs font-semibold uppercase tracking-wide transition-colors',
          isSectionActive
            ? 'text-foreground hover:bg-accent'
            : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
        )}
        aria-expanded={open}
      >
        <Icon className="h-3.5 w-3.5 shrink-0" aria-hidden />
        <span className="flex-1 text-left">{section.label}</span>
        <ChevronDown
          className={cn('h-4 w-4 shrink-0 transition-transform', open && 'rotate-180')}
          aria-hidden
        />
      </button>
      {open && (
        <div className="space-y-1">
          {section.items.map((item) => (
            <NavGroup
              key={item.href}
              item={item}
              pathname={pathname}
              compact={compact}
              onClick={onClick}
            />
          ))}
        </div>
      )}
    </div>
  )
}

export function Sidebar() {
  const pathname = usePathname() ?? ''
  const router = useRouter()
  const [mobileOpen, setMobileOpen] = useState(false)
  /** Persisted: true = icon rail is the resting state (user chose collapse via logo). */
  const [collapsed, setCollapsed] = useState(false)
  /** While resting collapsed, hover temporarily expands until mouse leaves. */
  const [hoverExpanded, setHoverExpanded] = useState(false)
  const hoverTimeoutRef = useRef<NodeJS.Timeout | null>(null)
  const [theme, setTheme] = useState<ThemeMode>(() => {
    if (typeof document !== 'undefined' && document.documentElement.classList.contains('dark')) return 'dark'
    return 'light'
  })

  useEffect(() => {
    try {
      if (localStorage.getItem(STORAGE_KEY) === '1') {
        setCollapsed(true)
      }
    } catch {
      /* ignore */
    }
  }, [])

  useEffect(() => {
    const stored = getStoredTheme()
    if (stored) setTheme(stored)

    const onChanged = (e: Event) => {
      const next = (e as CustomEvent).detail as ThemeMode
      if (next === 'dark' || next === 'light') setTheme(next)
    }
    window.addEventListener('notifyhub-theme-changed', onChanged as any)
    return () => window.removeEventListener('notifyhub-theme-changed', onChanged as any)
  }, [])

  // Cleanup timeout on unmount
  useEffect(() => {
    return () => {
      if (hoverTimeoutRef.current) {
        clearTimeout(hoverTimeoutRef.current)
      }
    }
  }, [])

  const setCollapsedPersist = (next: boolean) => {
    setCollapsed(next)
    setHoverExpanded(false)
    try {
      localStorage.setItem(STORAGE_KEY, next ? '1' : '0')
    } catch {
      /* ignore */
    }
  }

  const compactNav = collapsed && !hoverExpanded
  const { user: clerkUser } = useMaybeUser()
  const clerk = useMaybeClerk()
  const authUser = getAuthUser()
  const role = effectiveRole({ clerkUser, legacyRole: authUser?.role, fallback: 'support' })

  // Use Clerk user info if available, fall back to legacy localStorage
  const effectiveName = clerkUser?.fullName ?? clerkUser?.username ?? authUser?.name ?? 'Account'
  const effectiveEmail = clerkUser?.primaryEmailAddress?.emailAddress ?? authUser?.email ?? ''

  const logout = () => {
    clearAuthToken()
    if (clerkUser) {
      clerk.signOut()
    } else {
      router.replace('/login')
    }
  }

  const visibleNavSections = useMemo(() => {
    return navSections
      .map((section) => ({
        ...section,
        items: filterNavItemsByRole(section.items, role),
      }))
      .filter((section) => section.items.length > 0)
  }, [role])

  // Never return early before hooks run; keep this after hooks to avoid hook-order issues.
  if (pathname === '/login') return null

  const navContent = (opts: { compact: boolean }) => (
    <div className="flex h-full flex-col">
      <div
        className={cn(
          'flex items-center gap-2 border-b py-5',
          opts.compact ? 'justify-center px-2' : '',
        )}
      >
        <button
          type="button"
          className={cn(
            'flex items-center gap-2 rounded-md outline-none ring-offset-background focus-visible:ring-2 focus-visible:ring-ring',
            opts.compact ? 'justify-center' : 'w-full text-left',
          )}
          onClick={() => setCollapsedPersist(!collapsed)}
          aria-label={collapsed ? 'Expand sidebar (stay open)' : 'Collapse sidebar to icon rail'}
          aria-expanded={!collapsed}
        >
          {opts.compact ? (
            <span className="relative shrink-0 h-10 w-10">
              <Image
                src="/assets/logo_small.png"
                alt="NotifyHub"
                fill
                className="object-contain"
                priority
              />
            </span>
          ) : (
            <span className="relative w-full h-10">
              <Image
                src="/assets/logo_large.png"
                alt="NotifyHub"
                fill
                className="object-cover"
                priority
              />
            </span>
          )}
        </button>
      </div>

      <nav className="flex-1 space-y-4 overflow-y-auto overflow-x-hidden p-3">
        {visibleNavSections.map((section) => (
          <NavSectionGroup
            key={section.id}
            section={section}
            pathname={pathname}
            compact={opts.compact}
            onClick={() => setMobileOpen(false)}
          />
        ))}
      </nav>

      <div className="mt-auto border-t p-3">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              className={cn(
                'w-full rounded-md px-2 py-2 hover:bg-accent transition-colors outline-none ring-offset-background focus-visible:ring-2 focus-visible:ring-ring',
                opts.compact ? 'flex items-center justify-center' : 'flex items-center gap-2',
              )}
              aria-label="Open profile menu"
            >
              <UserCircle2 className="h-5 w-5 shrink-0 text-muted-foreground" aria-hidden />
              {!opts.compact && (
                <div className="min-w-0 flex-1 text-left">
                  <p className="text-sm font-medium truncate">{effectiveName}</p>
                  <p className="text-xs text-muted-foreground truncate">{effectiveEmail ?? role}</p>
                </div>
              )}
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align={opts.compact ? 'center' : 'start'} side="top" className="w-56">
            <DropdownMenuLabel>
              <div className="min-w-0">
                <p className="text-sm font-medium truncate">{effectiveName}</p>
                <p className="text-xs text-muted-foreground truncate">
                  {effectiveEmail}
                  {effectiveEmail ? ' • ' : ''}
                  {role}
                </p>
              </div>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild>
              <Link href="/profile" onClick={() => setMobileOpen(false)}>
                <UserCircle2 className="mr-2 h-4 w-4" aria-hidden />
                Profile
              </Link>
            </DropdownMenuItem>
            <DropdownMenuItem
              onSelect={(e) => {
                e.preventDefault()
                const next = toggleTheme()
                setTheme(next)
              }}
            >
              {theme === 'dark' ? <Moon className="mr-2 h-4 w-4" aria-hidden /> : <Sun className="mr-2 h-4 w-4" aria-hidden />}
              Theme: {theme === 'dark' ? 'Dark' : 'Light'}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onSelect={(e) => {
                e.preventDefault()
                logout()
              }}
            >
              <LogOut className="mr-2 h-4 w-4" aria-hidden />
              Log out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        {!opts.compact && (
          <p className="mt-3 text-xs text-muted-foreground">Notification Service v0.1</p>
        )}
      </div>
    </div>
  )

  return (
    <>
      <aside
        className={cn(
          'hidden lg:flex shrink-0 flex-col border-r bg-card sticky top-0 h-screen transition-[width,box-shadow] duration-200 ease-out',
          compactNav ? 'w-[4.5rem]' : 'w-60',
          collapsed && hoverExpanded && 'shadow-xl z-40',
        )}
        onMouseEnter={() => {
          // Clear any pending collapse timeout
          if (hoverTimeoutRef.current) {
            clearTimeout(hoverTimeoutRef.current)
            hoverTimeoutRef.current = null
          }
          if (collapsed) setHoverExpanded(true)
        }}
        onMouseLeave={() => {
          // Add a small delay before collapsing to prevent flickering when dropdown opens
          if (collapsed) {
            hoverTimeoutRef.current = setTimeout(() => {
              setHoverExpanded(false)
            }, 150)
          }
        }}
      >
        {navContent({ compact: compactNav })}
      </aside>

      <div className="lg:hidden fixed top-4 left-4 z-50">
        <Button
          variant="outline"
          size="icon"
          onClick={() => setMobileOpen(true)}
          className="bg-background shadow-md"
          aria-label="Open menu"
        >
          <Menu className="h-4 w-4" />
        </Button>
      </div>

      {mobileOpen && (
        <div className="lg:hidden fixed inset-0 z-50 flex">
          <div
            className="fixed inset-0 bg-black/50"
            aria-hidden
            onClick={() => setMobileOpen(false)}
          />
          <aside className="relative z-50 w-60 bg-card h-full shadow-xl">
            <Button
              variant="ghost"
              size="icon"
              onClick={() => setMobileOpen(false)}
              className="absolute right-3 top-3"
              aria-label="Close menu"
            >
              <X className="h-4 w-4" />
            </Button>
            {navContent({ compact: false })}
          </aside>
        </div>
      )}
    </>
  )
}
