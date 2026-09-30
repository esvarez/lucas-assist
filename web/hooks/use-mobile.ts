import { useSyncExternalStore } from 'react'

// 1024px matches the `lg` breakpoint this app already uses to split
// mobile/desktop layouts (e.g. ProjectDetailPage's `lg:hidden` /
// `hidden lg:flex` pairs) — kept in sync with that rather than shadcn's
// usual 768px default, so "mobile" means the same thing everywhere.
const QUERY = '(max-width: 1023px)'

function subscribe(listener: () => void) {
  const mql = window.matchMedia(QUERY)
  mql.addEventListener('change', listener)
  return () => mql.removeEventListener('change', listener)
}

function getSnapshot() {
  return window.matchMedia(QUERY).matches
}

export function useIsMobile() {
  return useSyncExternalStore(subscribe, getSnapshot)
}
