import { useId, useState, type FormEvent } from 'react'
import { format } from 'date-fns'
import { CalendarIcon, PencilIcon, PlusIcon, XIcon } from 'lucide-react'
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { ApiError, ValidationError, getProject, updateProject, type Project } from '@/src/api/projects'
import { PROJECT_STATUS_OPTIONS } from '@/src/lib/project-status'
import { cn } from '@/lib/utils'

// Fields this form renders — name is immutable via PUT (docs/openapi.yaml
// UpdateProjectRequest), so it's deliberately absent, same as the request
// type itself. Anything the server flags outside this set surfaces as a
// general error instead of being silently dropped.
const KNOWN_FIELDS = new Set(['goal', 'deadline', 'constraints', 'status'])

// Calendar hands back a Date at local midnight for the picked day.
// date.toISOString() converts that to UTC, which shifts the day backward
// in any timezone ahead of UTC — normalize to UTC midnight for the same
// calendar day instead of the instant the local midnight represents.
function toUTCMidnightISO(date: Date): string {
  return new Date(Date.UTC(date.getFullYear(), date.getMonth(), date.getDate())).toISOString()
}

// Inverse of toUTCMidnightISO: project.deadline is UTC midnight for a
// calendar day. new Date(iso) keeps that instant, so reading it back with
// local getters (Calendar's `selected`, or re-encoding through
// toUTCMidnightISO on save) shifts the day in any timezone behind UTC.
// Rebuild a Date whose *local* Y/M/D match the ISO string's *UTC* Y/M/D
// so the calendar day round-trips correctly.
function parseDeadline(iso: string): Date {
  const utc = new Date(iso)
  return new Date(utc.getUTCFullYear(), utc.getUTCMonth(), utc.getUTCDate())
}

function EditProjectDialog({
  project,
  onUpdated,
}: {
  project: Project
  onUpdated: (project: Project) => void
}) {
  const goalId = useId()
  const deadlineId = useId()
  const statusId = useId()

  const [open, setOpen] = useState(false)
  const [goal, setGoal] = useState(project.goal)
  const [deadline, setDeadline] = useState<Date | undefined>(
    project.deadline ? parseDeadline(project.deadline) : undefined
  )
  const [deadlineOpen, setDeadlineOpen] = useState(false)
  const [constraints, setConstraints] = useState<string[]>(project.constraints)
  const [status, setStatus] = useState(project.status)
  const [submitting, setSubmitting] = useState(false)
  const [generalError, setGeneralError] = useState<string | null>(null)
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})

  function resetToProject() {
    setGoal(project.goal)
    setDeadline(project.deadline ? parseDeadline(project.deadline) : undefined)
    setDeadlineOpen(false)
    setConstraints(project.constraints)
    setStatus(project.status)
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
    if (submitting) return

    setSubmitting(true)
    setGeneralError(null)
    setFieldErrors({})
    try {
      const updated = await updateProject(project.id, {
        version: project.version,
        goal: goal.trim(),
        deadline: deadline ? toUTCMidnightISO(deadline) : null,
        constraints: constraints.map((c) => c.trim()).filter(Boolean),
        status,
      })
      onUpdated(updated)
      setOpen(false)
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
          // Reload the current version so a retry can actually succeed —
          // just telling the user to reopen doesn't help if this dialog
          // keeps handing the next save the same stale version.
          try {
            const fresh = await getProject(project.id)
            onUpdated(fresh)
            setGeneralError(
              'This project changed elsewhere since you opened it — reloaded the latest version. Review your edits and save again.'
            )
          } catch {
            setGeneralError(
              'This project changed elsewhere since you opened it, and the latest version failed to load. Close and reopen to try again.'
            )
          }
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
        if (next) resetToProject()
      }}
    >
      <DialogTrigger render={<Button variant="outline" size="icon" aria-label="Edit project" />}>
        <PencilIcon />
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={handleSubmit} className="flex max-h-[80vh] flex-col gap-4 overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Edit project</DialogTitle>
          </DialogHeader>

          {generalError && <p className="text-xs text-destructive">{generalError}</p>}

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
                        'w-full justify-start font-normal',
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
            <Label htmlFor={statusId}>Status</Label>
            <Select value={status} onValueChange={(next) => setStatus(next as string)}>
              <SelectTrigger id={statusId} className="w-full" disabled={submitting}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {PROJECT_STATUS_OPTIONS.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {fieldErrors.status && <p className="text-xs text-destructive">{fieldErrors.status}</p>}
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
            <Button type="submit" disabled={submitting}>
              {submitting ? 'Saving…' : 'Save'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export default EditProjectDialog
