import { useId, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
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
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ApiError, ValidationError, createProject } from '../api/projects'

const STATUS_OPTIONS = [
  { value: 'on-track', label: 'On track' },
  { value: 'at-risk', label: 'At risk' },
  { value: 'blocked', label: 'Blocked' },
  { value: 'paused', label: 'Paused' },
]

// Fields this form renders — anything the server flags outside this set
// (e.g. user_id, which has no input here) surfaces as a general error
// instead of being silently dropped.
const KNOWN_FIELDS = new Set(['name', 'goal', 'deadline', 'status', 'constraints'])

function toISODeadline(dateInput: string): string | undefined {
  if (!dateInput) return undefined
  return new Date(`${dateInput}T00:00:00Z`).toISOString()
}

function NewProjectDialog() {
  const navigate = useNavigate()
  const nameId = useId()
  const goalId = useId()
  const deadlineId = useId()
  const statusId = useId()

  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [goal, setGoal] = useState('')
  const [deadline, setDeadline] = useState('')
  const [status, setStatus] = useState('on-track')
  const [constraints, setConstraints] = useState<string[]>([])
  const [submitting, setSubmitting] = useState(false)
  const [generalError, setGeneralError] = useState<string | null>(null)
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})

  const trimmedName = name.trim()

  function reset() {
    setName('')
    setGoal('')
    setDeadline('')
    setStatus('on-track')
    setConstraints([])
    setGeneralError(null)
    setFieldErrors({})
  }

  function updateConstraint(index: number, value: string) {
    setConstraints((prev) => prev.map((c, i) => (i === index ? value : c)))
  }

  function removeConstraint(index: number) {
    setConstraints((prev) => prev.filter((_, i) => i !== index))
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    // Client-side check on name only — everything else is left to the
    // server's validator (#52), surfaced below via fieldErrors.
    if (!trimmedName || submitting) return

    setSubmitting(true)
    setGeneralError(null)
    setFieldErrors({})
    try {
      const project = await createProject({
        name: trimmedName,
        goal: goal.trim(),
        deadline: toISODeadline(deadline),
        constraints: constraints.map((c) => c.trim()).filter(Boolean),
        status,
      })
      setOpen(false)
      reset()
      navigate(`/projects/${project.id}`)
    } catch (err) {
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
        if (err.status === 409) {
          setGeneralError('A project with this ID already exists — please try again.')
        } else {
          setGeneralError(err.message)
        }
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
      <DialogTrigger render={<Button />}>+ New project</DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={handleSubmit} className="flex max-h-[80vh] flex-col gap-4 overflow-y-auto">
          <DialogHeader>
            <DialogTitle>New project</DialogTitle>
          </DialogHeader>

          {generalError && <p className="text-xs text-destructive">{generalError}</p>}

          <div className="flex flex-col gap-2">
            <Label htmlFor={nameId}>Name</Label>
            <Input
              id={nameId}
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. Tidepool Sync"
              autoFocus
              disabled={submitting}
              aria-invalid={Boolean(fieldErrors.name)}
            />
            {fieldErrors.name && <p className="text-xs text-destructive">{fieldErrors.name}</p>}
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor={goalId}>Goal</Label>
            <Textarea
              id={goalId}
              value={goal}
              onChange={(e) => setGoal(e.target.value)}
              placeholder="What does shipping this look like?"
              disabled={submitting}
              aria-invalid={Boolean(fieldErrors.goal)}
            />
            {fieldErrors.goal && <p className="text-xs text-destructive">{fieldErrors.goal}</p>}
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor={deadlineId}>Deadline</Label>
            <Input
              id={deadlineId}
              type="date"
              value={deadline}
              onChange={(e) => setDeadline(e.target.value)}
              disabled={submitting}
              aria-invalid={Boolean(fieldErrors.deadline)}
            />
            {fieldErrors.deadline && (
              <p className="text-xs text-destructive">{fieldErrors.deadline}</p>
            )}
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor={statusId}>Status</Label>
            <Select value={status} onValueChange={(next) => setStatus(next as string)}>
              <SelectTrigger id={statusId} className="w-full" disabled={submitting}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {STATUS_OPTIONS.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {fieldErrors.status && (
              <p className="text-xs text-destructive">{fieldErrors.status}</p>
            )}
          </div>

          <div className="flex flex-col gap-2">
            <Label>Constraints</Label>
            {constraints.map((constraint, index) => (
              <div key={index} className="flex gap-2">
                <Input
                  value={constraint}
                  onChange={(e) => updateConstraint(index, e.target.value)}
                  placeholder="e.g. Must ship on Postgres"
                  disabled={submitting}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  onClick={() => removeConstraint(index)}
                  disabled={submitting}
                  aria-label="Remove constraint"
                >
                  <XIcon />
                </Button>
              </div>
            ))}
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setConstraints((prev) => [...prev, ''])}
              disabled={submitting}
              className="self-start"
            >
              <PlusIcon /> Add constraint
            </Button>
            {fieldErrors.constraints && (
              <p className="text-xs text-destructive">{fieldErrors.constraints}</p>
            )}
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setOpen(false)}
              disabled={submitting}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={!trimmedName || submitting}>
              {submitting ? 'Creating…' : 'Create'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export default NewProjectDialog
