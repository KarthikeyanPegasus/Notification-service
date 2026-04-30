export type ThemeMode = 'light' | 'dark'

const THEME_KEY = 'notifyhub-theme'

export function getStoredTheme(): ThemeMode | null {
  try {
    const v = localStorage.getItem(THEME_KEY)
    if (v === 'light' || v === 'dark') return v
    return null
  } catch {
    return null
  }
}

export function setStoredTheme(mode: ThemeMode) {
  try {
    localStorage.setItem(THEME_KEY, mode)
  } catch {
    /* ignore */
  }
}

export function applyTheme(mode: ThemeMode) {
  if (typeof document === 'undefined') return
  const root = document.documentElement
  if (mode === 'dark') root.classList.add('dark')
  else root.classList.remove('dark')
}

export function toggleTheme(): ThemeMode {
  const current = getStoredTheme() ?? (typeof document !== 'undefined' && document.documentElement.classList.contains('dark') ? 'dark' : 'light')
  const next: ThemeMode = current === 'dark' ? 'light' : 'dark'
  setStoredTheme(next)
  applyTheme(next)
  try {
    window.dispatchEvent(new CustomEvent('notifyhub-theme-changed', { detail: next }))
  } catch {
    /* ignore */
  }
  return next
}

