import { useCallback, useSyncExternalStore } from 'react'

export type Theme = 'light' | 'dark'

const STORAGE_KEY = 'theme'

function systemTheme(): Theme {
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function storedTheme(): Theme | null {
  const value = localStorage.getItem(STORAGE_KEY)
  return value === 'light' || value === 'dark' ? value : null
}

function applyTheme(theme: Theme) {
  document.documentElement.classList.toggle('dark', theme === 'dark')
}

// Module-level store shared by every useTheme() call site. Plain
// useState is per-component: with the toggle button and the toast
// Toaster each calling useTheme() independently, a toggle in one never
// reached the other. A single shared value plus subscribers fixes that
// without introducing a Context provider.
let currentTheme: Theme = storedTheme() ?? systemTheme()
const listeners = new Set<() => void>()

function setStoreTheme(next: Theme) {
  currentTheme = next
  applyTheme(next)
  for (const listener of listeners) listener()
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

function getSnapshot() {
  return currentTheme
}

// Follow the OS preference live as long as the user hasn't picked one
// explicitly. Registered once at module scope, not per component
// instance — re-checks localStorage on every OS change rather than once
// up front, so a toggle made after this listener is registered still wins.
window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
  if (storedTheme()) return
  setStoreTheme(systemTheme())
})

// Resolution order — stored preference first, OS preference otherwise —
// matches the blocking bootstrap script in index.html that sets the
// class before first paint.
export function useTheme() {
  const theme = useSyncExternalStore(subscribe, getSnapshot)

  const setTheme = useCallback((next: Theme) => {
    localStorage.setItem(STORAGE_KEY, next)
    setStoreTheme(next)
  }, [])

  const toggleTheme = useCallback(() => {
    setTheme(currentTheme === 'dark' ? 'light' : 'dark')
  }, [setTheme])

  return { theme, setTheme, toggleTheme }
}
