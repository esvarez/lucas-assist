import { useId, useState, type FormEvent } from 'react'
import { PlusIcon, XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { ApiError, ValidationError } from '@/src/api/projects'
import { createTask, type Task } from '@/src/api/tasks'
import { notify } from '@/src/lib/notify'

// Fields this form renders; anything else the server flags surfaces as a
// general error instead of being silently dropped (same as
// EditProjectDialog's KNOWN_FIELDS).
const KNOWN_FIELDS = new Set(['title', 'description', 'acceptance_criteria', 'parent_id'])

// parentOptions flattens the task tree depth-first, keeping each task's
// depth so the parent picker can indent subtasks under their parent.
function parentOptions(tasks: Task[], depth = 0): { task: Task; depth: number }[] {
  return tasks.flatMap((task) => [{ task, depth }, ...parentOptions(task.subtasks, depth + 1)])
}

// AddTaskDialog adds a single task by hand (#175) — for a task the user
// already knows they want, without a decompose_task round-trip.
// defaultParentId preselects a parent, for opening it from a task's own
// row; the user can still change it.
function AddTaskDialog({
  projectId,
  tasks,
  open,
  onOpenChange,
  onCreated,
  defaultParentId = null,
}: {
  projectId: string
  tasks: Task[]
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: () => void
  defaultParentId?: string | null
}) {
  const titleId = useId()
  const descriptionId = useId()
  const parentId = useId()

  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [criteria, setCriteria] = useState<string[]>([])
  const [parent, setParent] = useState<string | null>(defaultParentId)
  const [submitting, setSubmitting] = useState(false)
  const [generalError, setGeneralError] = useState<string | null>(null)
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})

  const options = parentOptions(tasks)
  const titleById = new Map(options.map(({ task }) => [task.id, task.title]))

  // Reset on every open, including one driven by the `open` prop alone —
  // onOpenChange only fires for the dialog's own open/close gestures, not
  // for a parent flipping `open` to true, so it can't carry the reset.
  // Adjusting state during render (rather than in an effect) avoids a
  // frame showing the previous open's values.
  const [wasOpen, setWasOpen] = useState(open)
  if (open !== wasOpen) {
    setWasOpen(open)
    if (open) {
      setTitle('')
      setDescription('')
      setCriteria([])
      setParent(defaultParentId)
      setGeneralError(null)
      setFieldErrors({})
    }
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (submitting) return

    if (!title.trim()) {
      setFieldErrors({ title: 'Title is required' })
      return
    }

    setSubmitting(true)
    setGeneralError(null)
    setFieldErrors({})
    try {
      await createTask(projectId, {
        title: title.trim(),
        description: description.trim(),
        acceptance_criteria: criteria.map((c) => c.trim()).filter(Boolean),
        parent_id: parent ?? undefined,
      })
      onCreated()
      onOpenChange(false)
      notify.success('Task added')
    } catch (err) {
      notify.error('Failed to add task')
      if (err instanceof ValidationError) {
        const known: Record<string, string> = {}
        const unknown: string[] = []
        for (const [field, message] of Object.entries(err.fields)) {
          if (KNOWN_FIELDS.has(field)) known[field] = message
          else unknown.push(message)
        }
        setFieldErrors(known)
        if (unknown.length > 0) setGeneralError(unknown.join(', '))
      } else if (err instanceof ApiError) {
        setGeneralError(err.message)
      } else {
        setGeneralError(err instanceof Error ? err.message : 'Something went wrong')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={handleSubmit} className="flex max-h-[80vh] flex-col gap-4 overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{parent ? 'Add a subtask' : 'Add a task'}</DialogTitle>
          </DialogHeader>

          {generalError && <p className="text-xs text-destructive">{generalError}</p>}

          <div className="flex flex-col gap-2">
            <Label htmlFor={titleId}>Title</Label>
            <Input
              id={titleId}
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="e.g. Write the README"
              disabled={submitting}
              aria-invalid={Boolean(fieldErrors.title)}
              autoFocus
            />
            {fieldErrors.title && <p className="text-xs text-destructive">{fieldErrors.title}</p>}
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor={descriptionId}>Description</Label>
            <Textarea
              id={descriptionId}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="What does done look like?"
              disabled={submitting}
              aria-invalid={Boolean(fieldErrors.description)}
            />
            {fieldErrors.description && <p className="text-xs text-destructive">{fieldErrors.description}</p>}
          </div>

          <div className="flex flex-col gap-2">
            <Label>Acceptance criteria</Label>
            {criteria.map((criterion, index) => (
              <div key={index} className="flex gap-2">
                <Input
                  value={criterion}
                  onChange={(e) => setCriteria((prev) => prev.map((c, i) => (i === index ? e.target.value : c)))}
                  placeholder="e.g. Covers local setup"
                  disabled={submitting}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  onClick={() => setCriteria((prev) => prev.filter((_, i) => i !== index))}
                  disabled={submitting}
                  aria-label="Remove criterion"
                >
                  <XIcon />
                </Button>
              </div>
            ))}
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setCriteria((prev) => [...prev, ''])}
              disabled={submitting}
              className="self-start"
            >
              <PlusIcon /> Add criterion
            </Button>
            {fieldErrors.acceptance_criteria && (
              <p className="text-xs text-destructive">{fieldErrors.acceptance_criteria}</p>
            )}
          </div>

          {options.length > 0 && (
            <div className="flex flex-col gap-2">
              <Label htmlFor={parentId}>Parent task</Label>
              <Select value={parent} onValueChange={(next) => setParent((next as string | null) ?? null)}>
                <SelectTrigger
                  id={parentId}
                  className="w-full"
                  disabled={submitting}
                  aria-invalid={Boolean(fieldErrors.parent_id)}
                >
                  <SelectValue>
                    {(value: string | null) => (value ? titleById.get(value) ?? value : 'None — top-level task')}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={null}>None — top-level task</SelectItem>
                  {options.map(({ task, depth }) => (
                    <SelectItem key={task.id} value={task.id}>
                      <span style={{ paddingLeft: `${depth * 0.75}rem` }}>{task.title}</span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {fieldErrors.parent_id && <p className="text-xs text-destructive">{fieldErrors.parent_id}</p>}
            </div>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={submitting}>
              Cancel
            </Button>
            <Button type="submit" disabled={submitting}>
              {submitting ? <Spinner /> : 'Add task'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export default AddTaskDialog
