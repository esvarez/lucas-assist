const TASK_STATUS_TEXT_CLASSES: Record<string, string> = {
  todo: 'text-muted-foreground',
  done: 'text-emerald-600 dark:text-emerald-400',
  'in-progress': 'text-amber-600 dark:text-amber-400',
  blocked: 'text-red-700 dark:text-red-300',
}

const TASK_STATUS_LABELS: Record<string, string> = {
  todo: 'To do',
  done: 'Done',
  'in-progress': 'In progress',
  blocked: 'Block',
}

export function taskStatusClassName(status: string): string {
  return TASK_STATUS_TEXT_CLASSES[status] ?? 'text-muted-foreground'
}

export function taskStatusLabel(status: string): string {
  return TASK_STATUS_LABELS[status] ?? status
}
