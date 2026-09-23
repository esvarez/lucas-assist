import { useId, useRef, useState, type FormEvent, type ReactElement } from 'react'
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
import { ApiError, type ProjectDomain } from '@/src/api/projects'
import {
  acceptCreateProject,
  dispatchCreateProject,
  pollAgentRun,
  type Clarification,
  type ProposedProject,
} from '@/src/api/skills'
import { notify } from '@/src/lib/notify'
import { cn } from '@/lib/utils'

// Step is the propose -> poll -> review -> accept loop (architecture.md
// §1), same shape as DecomposeTaskDialog's — see that component's doc
// comment for the general pattern. 'clarify' handles create_project's
// needs_clarification result (#153): round 0 only. Answering resubmits as
// a fresh dispatch with clarification_round: 1.
type Step =
  | { name: 'form' }
  | { name: 'working'; label: string }
  | { name: 'clarify'; questions: string[] }
  | { name: 'review'; project: ProposedProject; changesetId: string; idempotencyKey: string }
  | { name: 'error'; message: string }

function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === 'AbortError'
}

// Calendar hands back a Date at local midnight for the picked day.
function formatDeadlineForDescription(date: Date): string {
  return format(date, 'PPP')
}

// buildDescription turns the structured form fields into the freeform text
// create_project's skill expects (internal/agent/skills/create_project.go's
// CreateProjectInput.Description) — the fields stay the primary,
// exact-control input; this is just how they're handed to the skill (#154:
// "same form, wired to the agent").
function buildDescription(name: string, goal: string, deadline: Date | undefined, constraints: string[]): string {
  const parts = [`Name: ${name}`]
  if (goal) parts.push(`Goal: ${goal}`)
  if (deadline) parts.push(`Deadline: ${formatDeadlineForDescription(deadline)}`)
  if (constraints.length > 0) parts.push(`Constraints:\n${constraints.map((c) => `- ${c}`).join('\n')}`)
  return parts.join('\n\n')
}

// trigger lets callers swap in a more prominent CTA (e.g. the empty
// state, #93) without duplicating the dialog/form itself.
function NewProjectDialog({ trigger = <Button>+ New project</Button> }: { trigger?: ReactElement } = {}) {
  const navigate = useNavigate()
  const nameId = useId()
  const goalId = useId()
  const deadlineId = useId()
  const domainLabelId = useId()
  const clarifyBaseId = useId()

  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [goal, setGoal] = useState('')
  const [deadline, setDeadline] = useState<Date | undefined>(undefined)
  const [deadlineOpen, setDeadlineOpen] = useState(false)
  const [constraints, setConstraints] = useState<string[]>([])
  const [projectDomain, setProjectDomain] = useState<ProjectDomain>('general')
  const [step, setStep] = useState<Step>({ name: 'form' })
  // One answer per question in the current 'clarify' step, same index —
  // cleared on reset and whenever a fresh set of questions comes back.
  const [answers, setAnswers] = useState<string[]>([])
  // Cancels an in-flight poll when the dialog closes mid-wait, so a stale
  // response doesn't land after the user has moved on.
  const pollAbort = useRef<AbortController | null>(null)

  const trimmedName = name.trim()
  const working = step.name === 'working'

  function reset() {
    setName('')
    setGoal('')
    setDeadline(undefined)
    setDeadlineOpen(false)
    setConstraints([])
    setProjectDomain('general')
    setAnswers([])
    setStep({ name: 'form' })
    pollAbort.current?.abort()
    pollAbort.current = null
  }

  function updateConstraint(index: number, value: string) {
    setConstraints((prev) => prev.map((c, i) => (i === index ? value : c)))
  }

  function removeConstraint(index: number) {
    setConstraints((prev) => prev.filter((_, i) => i !== index))
  }

  // startRun dispatches create_project and polls it to a terminal status —
  // shared by the initial submit (round 0, no clarification) and the
  // clarify step's resubmit (round 1, with answers attached).
  async function startRun(clarification?: { round: number; clarifications: Clarification[] }) {
    setStep({ name: 'working', label: 'Starting…' })
    try {
      const description = buildDescription(
        trimmedName,
        goal.trim(),
        deadline,
        constraints.map((c) => c.trim()).filter(Boolean)
      )
      const { run_id } = await dispatchCreateProject(description, projectDomain, clarification)

      setStep({ name: 'working', label: 'Thinking — this can take a few seconds…' })
      const controller = new AbortController()
      pollAbort.current = controller
      const run = await pollAgentRun(run_id, undefined, { signal: controller.signal })
      pollAbort.current = null

      if (run.status === 'failed') {
        setStep({ name: 'error', message: run.error || 'Creating the project failed.' })
        return
      }
      if (run.status === 'needs_input') {
        const questions = run.questions ?? []
        setAnswers(new Array(questions.length).fill(''))
        setStep({ name: 'clarify', questions })
        return
      }
      if (!run.changeset || !run.changeset.proposed_project) {
        setStep({ name: 'error', message: 'The run completed without a proposal to review.' })
        return
      }

      setStep({
        name: 'review',
        project: run.changeset.proposed_project,
        changesetId: run.changeset.id,
        idempotencyKey: crypto.randomUUID(),
      })
    } catch (err) {
      // The dialog was closed (or reset) mid-poll, which aborts on
      // purpose — reset() already put the dialog back in its initial
      // state, so this isn't a real failure to report.
      if (isAbortError(err)) return
      setStep({ name: 'error', message: err instanceof Error ? err.message : 'Something went wrong' })
    }
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!trimmedName || working) return
    await startRun()
  }

  async function handleClarifySubmit(e: FormEvent) {
    e.preventDefault()
    if (step.name !== 'clarify' || working) return

    const clarifications: Clarification[] = step.questions.map((question, index) => ({
      question,
      answer: answers[index]?.trim() ?? '',
    }))
    await startRun({ round: 1, clarifications })
  }

  async function handleAccept() {
    if (step.name !== 'review') return
    const { changesetId, idempotencyKey } = step

    setStep({ name: 'working', label: 'Creating your project…' })
    try {
      const project = await acceptCreateProject(changesetId, idempotencyKey)
      notify.success('Project created')
      setOpen(false)
      reset()
      navigate(`/projects/${project.id}`)
    } catch (err) {
      notify.error('Failed to create project')
      const message =
        err instanceof ApiError && err.status === 409
          ? 'This proposal can no longer be accepted — it may have already been applied. Close this and try again.'
          : err instanceof Error
            ? err.message
            : 'Failed to create the project'
      setStep({ name: 'error', message })
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
        <DialogHeader>
          <DialogTitle>New project</DialogTitle>
        </DialogHeader>

        {step.name === 'form' && (
          <form onSubmit={handleSubmit} className="flex max-h-[80vh] flex-col gap-4 overflow-y-auto">
            <div className="flex flex-col gap-2">
              <Label htmlFor={nameId}>Name</Label>
              <Input
                id={nameId}
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. Tidepool Sync"
                autoFocus
              />
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
                    aria-checked={projectDomain === value}
                    onClick={() => setProjectDomain(value)}
                    className={cn(
                      'rounded-sm px-3 py-1 text-xs font-medium transition-colors',
                      projectDomain === value
                        ? 'bg-primary text-primary-foreground'
                        : 'text-muted-foreground hover:text-foreground'
                    )}
                  >
                    {label}
                  </button>
                ))}
              </div>
            </div>

            <div className="flex flex-col gap-2">
              <Label htmlFor={goalId}>Description or Goal</Label>
              <Textarea
                id={goalId}
                value={goal}
                onChange={(e) => setGoal(e.target.value)}
                placeholder="What does shipping this look like?"
              />
            </div>

            <div className="flex flex-col gap-2">
              <Label htmlFor={deadlineId}>Deadline</Label>
              <div className="relative">
                <Popover open={deadlineOpen} onOpenChange={setDeadlineOpen}>
                  <PopoverTrigger
                    render={
                      <Button
                        id={deadlineId}
                        type="button"
                        variant="outline"
                        className={cn('w-full justify-start pr-9 font-normal', !deadline && 'text-muted-foreground')}
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
                    size="icon-sm"
                    className="absolute top-1/2 right-1 -translate-y-1/2"
                    onClick={(e) => {
                      e.stopPropagation()
                      setDeadline(undefined)
                    }}
                    aria-label="Clear deadline"
                  >
                    <XIcon />
                  </Button>
                )}
              </div>
            </div>

            <div className="flex flex-col gap-2">
              <Label>Constraints</Label>
              {constraints.map((constraint, index) => (
                <div key={index} className="flex gap-2">
                  <Input
                    value={constraint}
                    onChange={(e) => updateConstraint(index, e.target.value)}
                    placeholder="e.g. Must ship on Postgres"
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    onClick={() => removeConstraint(index)}
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
                className="self-start"
              >
                <PlusIcon /> Add constraint
              </Button>
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={!trimmedName}>
                Propose project
              </Button>
            </DialogFooter>
          </form>
        )}

        {step.name === 'working' && (
          <div className="flex flex-col items-center gap-3 py-8 text-sm text-muted-foreground">
            <Spinner className="size-6" />
            <p>{step.label}</p>
          </div>
        )}

        {step.name === 'clarify' && (
          <form onSubmit={handleClarifySubmit} className="flex flex-col gap-4">
            <p className="text-xs text-muted-foreground">
              A couple of details would change what this project card looks like.
            </p>
            {step.questions.map((question, index) => (
              <div key={index} className="flex flex-col gap-2">
                <Label htmlFor={`${clarifyBaseId}-${index}`}>{question}</Label>
                <Textarea
                  id={`${clarifyBaseId}-${index}`}
                  value={answers[index] ?? ''}
                  onChange={(e) =>
                    setAnswers((prev) => {
                      const next = [...prev]
                      next[index] = e.target.value
                      return next
                    })
                  }
                  placeholder="Your answer (or “no preference” / “out of scope”)"
                  required
                />
              </div>
            ))}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={answers.some((a) => !a.trim())}>
                Continue
              </Button>
            </DialogFooter>
          </form>
        )}

        {step.name === 'error' && (
          <div className="flex flex-col gap-4">
            <p className="text-sm text-destructive">{step.message}</p>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                Close
              </Button>
              <Button type="button" onClick={() => setStep({ name: 'form' })}>
                Try again
              </Button>
            </DialogFooter>
          </div>
        )}

        {step.name === 'review' && (
          <div className="flex flex-col gap-4">
            <p className="text-xs text-muted-foreground">
              Review the proposed project below. Nothing is created until you accept.
            </p>
            <div className="flex flex-col gap-3 rounded-md border border-border p-4">
              <div>
                <p className="text-sm font-medium">{step.project.name}</p>
                {step.project.goal && (
                  <p className="mt-1 text-sm text-muted-foreground">{step.project.goal}</p>
                )}
              </div>
              {step.project.deadline && (
                <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                  <CalendarIcon className="size-3.5" />
                  {format(new Date(step.project.deadline), 'PPP')}
                </div>
              )}
              {step.project.constraints.length > 0 && (
                <ul className="list-inside list-disc text-xs text-muted-foreground">
                  {step.project.constraints.map((constraint, index) => (
                    <li key={index}>{constraint}</li>
                  ))}
                </ul>
              )}
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button type="button" onClick={handleAccept}>
                Create project
              </Button>
            </DialogFooter>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

export default NewProjectDialog
