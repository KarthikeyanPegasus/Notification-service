'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import {
  LayoutDashboard,
  Bell,
  BarChart3,
  Clock,
  Menu,
  X,
  Zap,
  Settings,
  ShoppingBag,
  ShieldAlert,
  Book,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'

const STORAGE_KEY = 'notifyhub-sidebar-collapsed'

const navItems = [
  { href: '/dashboard', label: 'Dashboard', icon: LayoutDashboard },
  { href: '/app-store', label: 'App Store', icon: ShoppingBag },
  { href: '/notifications', label: 'Notifications', icon: Bell },
  { href: '/reports', label: 'Reports', icon: BarChart3 },
  { href: '/scheduled', label: 'Scheduled', icon: Clock },
  { href: '/templates', label: 'Templates', icon: Bell },
  { href: '/governance/suppressions', label: 'Suppressions', icon: ShieldAlert },
  { href: '/governance/opt-outs', label: 'Opt-outs', icon: ShieldAlert },
  { href: '/docs', label: 'API Documentation', icon: Book },
  { href: '/settings', label: 'Settings', icon: Settings },
]

function NavLink({
  href,
  label,
  icon: Icon,
  active,
  compact,
  onClick,
}: {
  href: string
  label: string
  icon: React.ElementType
  active: boolean
  compact: boolean
  onClick?: () => void
}) {
  return (
    <Link
      href={href}
      title={label}
      onClick={onClick}
      className={cn(
        'flex items-center gap-3 rounded-md py-2 text-sm font-medium transition-colors',
        compact ? 'justify-center px-2' : 'px-3',
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

export function Sidebar() {
  const pathname = usePathname()
  const [mobileOpen, setMobileOpen] = useState(false)
  /** Persisted: true = icon rail is the resting state (user chose collapse via logo). */
  const [collapsed, setCollapsed] = useState(false)
  /** While resting collapsed, hover temporarily expands until mouse leaves. */
  const [hoverExpanded, setHoverExpanded] = useState(false)

  useEffect(() => {
    try {
      if (localStorage.getItem(STORAGE_KEY) === '1') {
        setCollapsed(true)
      }
    } catch {
      /* ignore */
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

  const navContent = (opts: { compact: boolean }) => (
    <div className="flex h-full flex-col">
      <div
        className={cn(
          'flex items-center gap-2 border-b py-5',
          opts.compact ? 'justify-center px-2' : 'px-4',
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
          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-primary">
            <Zap className="h-4 w-4 text-primary-foreground" aria-hidden />
          </span>
          {!opts.compact && <span className="font-semibold text-foreground truncate">NotifyHub</span>}
        </button>
      </div>

      <nav className="flex-1 space-y-1 overflow-y-auto overflow-x-hidden p-3">
        {navItems.map((item) => (
          <NavLink
            key={item.href}
            href={item.href}
            label={item.label}
            icon={item.icon}
            compact={opts.compact}
            active={pathname === item.href || pathname.startsWith(item.href + '/')}
            onClick={() => setMobileOpen(false)}
          />
        ))}
      </nav>

      {!opts.compact && (
        <div className="mt-auto border-t px-4 py-3">
          <p className="text-xs text-muted-foreground">Notification Service v0.1</p>
        </div>
      )}
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
          if (collapsed) setHoverExpanded(true)
        }}
        onMouseLeave={() => {
          if (collapsed) setHoverExpanded(false)
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
