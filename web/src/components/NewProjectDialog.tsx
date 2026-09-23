import { useId, useState, type FormEvent, type ReactElement } from 'react'
import { useNavigate } from 'react-router-dom'
import { format } from 'date-fns'
import { CalendarIcon, PlusIcon, XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Calendar } from '@/components/ui/calendar'
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
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { ApiError, ValidationError, createProject, type ProjectDomain } from '@/src/api/projects'
import { notify } from '@/src/lib/notify'
import { cn } from '@/lib/utils'

// Fields this form renders — anything the server flags outside this set
// (e.g. user_id or status, which have no input here) surfaces as a
// general error instead of being silently dropped.
const KNOWN_FIELDS = new Set(['name', 'goal', 'deadline', 'constraints', 'domain'])

// Calendar hands back a Date at local midnight for the picked day.
// date.toISOString() converts that to UTC, which shifts the day backward
// in any timezone ahead of UTC — normalize to UTC midnight for the same
// calendar day instead of the instant the local midnight represents.
function toUTCMidnightISO(date: Date): string {
  return new Date(Date.UTC(date.getFullYear(), date.getMonth(), date.getDate())).toISOString()
}

// trigger lets callers swap in a more prominent CTA (e.g. the empty
// state, #93) without duplicating the dialog/form itself.
function NewProjectDialog({ trigger = <Button>+ New project</Button> }: { trigger?: ReactElement } = {}) {
  const navigate = useNavigate()
  const nameId = useId()
  const goalId = useId()
  const deadlineId = useId()
  const domainLabelId = useId()

  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [goal, setGoal] = useState('')
  const [deadline, setDeadline] = useState<Date | undefined>(undefined)
  const [deadlineOpen, setDeadlineOpen] = useState(false)
  const [constraints, setConstraints] = useState<string[]>([])
  const [domain, setDomain] = useState<ProjectDomain>('general')
  const [submitting, setSubmitting] = useState(false)
  const [generalError, setGeneralError] = useState<string | null>(null)
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})

  const trimmedName = name.trim()

  function reset() {
    setName('')
    setGoal('')
    setDeadline(undefined)
    setDeadlineOpen(false)
    setConstraints([])
    setDomain('general')
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
        deadline: deadline ? toUTCMidnightISO(deadline) : undefined,
        constraints: constraints.map((c) => c.trim()).filter(Boolean),
        domain
      })
      setOpen(false)
      reset()
      notify.success('Project created')
      navigate(`/projects/${project.id}`)
    } catch (err) {
      notify.error('Failed to create project')
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
      <DialogTrigger render={trigger} />
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
            <Label id={domainLabelId}>Project type</Label>
            <div
              role="radiogroup"
              aria-labelledby={domainLabelId}
              className="inline-flex w-fit rounded-md border border-border p-0.5"
            >
              {(
                [
                  { value: 'general', label: 'General' },
                  { value: 'software', label: 'Software' },
                ] as const
              ).map(({ value, label }) => (
                <button
                  key={value}
                  type="button"
                  role="radio"
                  aria-checked={domain === value}
                  disabled={submitting}
                  onClick={() => setDomain(value)}
                  className={cn(
                    'rounded-sm px-3 py-1 text-xs font-medium transition-colors disabled:pointer-events-none disabled:opacity-50',
                    domain === value
                      ? 'bg-primary text-primary-foreground'
                      : 'text-muted-foreground hover:text-foreground'
                  )}
                >
                  {label}
                </button>
              ))}
            </div>
            {fieldErrors.domain && <p className="text-xs text-destructive">{fieldErrors.domain}</p>}
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor={goalId}>Description or Goal</Label>
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
            <div className="flex gap-2">
              <Popover open={deadlineOpen} onOpenChange={setDeadlineOpen}>
                <PopoverTrigger
                  render={
                    <Button
                      id={deadlineId}
                      type="button"
                      variant="outline"
                      disabled={submitting}
                      aria-invalid={Boolean(fieldErrors.deadline)}
                      className={cn(
                        'min-w-0 flex-1 justify-start font-normal',
                        !deadline && 'text-muted-foreground'
                      )}
                    />
                  }
                >
                  <CalendarIcon />
                  {deadline ? format(deadline, 'PPP') : 'Pick a date'}
                </PopoverTrigger>
                <PopoverContent className="w-auto p-0">
                  <Calendar
                    mode="single"
                    selected={deadline}
                    onSelect={(date) => {
                      setDeadline(date)
                      setDeadlineOpen(false)
                    }}
                    disabled={{ before: new Date() }}
                    autoFocus
                  />
                </PopoverContent>
              </Popover>
              {deadline && (
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="shrink-0"
                  onClick={() => setDeadline(undefined)}
                  disabled={submitting}
                  aria-label="Clear deadline"
                >
                  <XIcon />
                </Button>
              )}
            </div>
            {fieldErrors.deadline && (
              <p className="text-xs text-destructive">{fieldErrors.deadline}</p>
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
              {submitting ? <Spinner /> : 'Create'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export default NewProjectDialog
