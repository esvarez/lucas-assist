// Status color mapping, matching the design canvas's statusMeta (on-track
// green, at-risk amber, blocked red, done violet, paused neutral). The
// design theme's own dark-palette hexes aren't ported here — this reuses
// the app's existing Tailwind color scale instead of introducing new
// design tokens for a single chip.
const STATUS_BADGE_CLASSES: Record<string, string> = {
  'on-track': 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400',
  'at-risk': 'bg-amber-500/15 text-amber-600 dark:text-amber-400',
  blocked: 'bg-red-50 text-red-700 dark:bg-red-950 dark:text-red-300',
  done: 'bg-violet-500/15 text-violet-600 dark:text-violet-400',
  paused: 'bg-muted text-muted-foreground',
}

export function statusBadgeClassName(status: string): string {
  return STATUS_BADGE_CLASSES[status] ?? 'bg-muted text-muted-foreground'
}

// Editable statuses offered by the edit form (#67) — same set the badge
// colors above know how to render.
export const PROJECT_STATUS_OPTIONS = [
  { value: 'on-track', label: 'On track' },
  { value: 'at-risk', label: 'At risk' },
  { value: 'blocked', label: 'Blocked' },
  { value: 'done', label: 'Done' },
  { value: 'paused', label: 'Paused' },
]
