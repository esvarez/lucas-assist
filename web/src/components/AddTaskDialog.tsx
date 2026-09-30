import { useId, useState, type FormEvent, type ReactElement } from 'react'
import { PlusIcon, XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { ApiError, ValidationError } from '@/src/api/projects'
import { createTask } from '@/src/api/tasks'
import { notify } from '@/src/lib/notify'

// Fields this form renders — anything the server flags outside this set
// surfaces as a general error instead of being silently dropped (same
// pattern as NewProjectDialog's KNOWN_FIELDS).
const KNOWN_FIELDS = new Set(['title', 'description', 'acceptance_criteria', 'parent_id'])

// AddTaskDialog is the manual counterpart to decompose_task's
// propose/review/accept flow (#175/#185) — a single task typed by hand
// instead of proposed by the model. parentId, when set, attaches the new
// task as a subtask and adjusts the dialog's copy accordingly; omit it to
// add a root-level task.
function AddTaskDialog({
  projectId,
  parentId,
  trigger,
  onCreated,
}: {
  projectId: string
  parentId?: string
  trigger: ReactElement
  onCreated: () => void
}) {
  const titleId = useId()
  const descriptionId = useId()

  const [open, setOpen] = useState(false)
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [criteria, setCriteria] = useState<string[]>([])
  const [submitting, setSubmitting] = useState(false)
  const [generalError, setGeneralError] = useState<string | null>(null)
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})

  const trimmedTitle = title.trim()

  function reset() {
    setTitle('')
    setDescription('')
    setCriteria([])
    setGeneralError(null)
    setFieldErrors({})
  }

  function updateCriterion(index: number, value: string) {
    setCriteria((prev) => prev.map((c, i) => (i === index ? value : c)))
  }

  function removeCriterion(index: number) {
    setCriteria((prev) => prev.filter((_, i) => i !== index))
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!trimmedTitle || submitting) return

    setSubmitting(true)
    setGeneralError(null)
    setFieldErrors({})
    try {
      await createTask(projectId, {
        title: trimmedTitle,
        description: description.trim(),
        acceptance_criteria: criteria.map((c) => c.trim()).filter(Boolean),
        ...(parentId ? { parent_id: parentId } : {}),
      })
      setOpen(false)
      reset()
      notify.success(parentId ? 'Subtask added' : 'Task added')
      onCreated()
    } catch (err) {
      notify.error(parentId ? 'Failed to add subtask' : 'Failed to add task')
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
    <Dialog
      open={open}
      onOpenChange={(next: boolean) => {
        setOpen(next)
        if (!next) reset()
      }}
    >
      <DialogTrigger render={trigger} />
      <DialogContent className="sm:max-w-md">
        <form onSubmit={handleSubmit} className="flex max-h-[80vh] flex-col gap-4 overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{parentId ? 'Add subtask' : 'Add task'}</DialogTitle>
          </DialogHeader>

          {generalError && <p className="text-xs text-destructive">{generalError}</p>}

          <div className="flex flex-col gap-2">
            <Label htmlFor={titleId}>Title</Label>
            <Input
              id={titleId}
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="e.g. Wire up the login form"
              autoFocus
              disabled={submitting}
              aria-invalid={Boolean(fieldErrors.title)}
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
                  onChange={(e) => updateCriterion(index, e.target.value)}
                  placeholder="e.g. Shows an error on bad credentials"
                  disabled={submitting}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  onClick={() => removeCriterion(index)}
                  disabled={submitting}
                  aria-label="Remove acceptance criterion"
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

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setOpen(false)} disabled={submitting}>
              Cancel
            </Button>
            <Button type="submit" disabled={!trimmedTitle || submitting}>
              {submitting ? <Spinner /> : parentId ? 'Add subtask' : 'Add task'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export default AddTaskDialog
