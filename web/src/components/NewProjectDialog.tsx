import { useId, useRef, useState, type FormEvent, type ReactElement } from 'react'
import { useNavigate } from 'react-router-dom'
import { format } from 'date-fns'
import { CalendarIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { ApiError } from '@/src/api/projects'
import {
  acceptCreateProject,
  dispatchCreateProject,
  pollAgentRun,
  type Clarification,
  type ProposedProject,
} from '@/src/api/skills'
import { notify } from '@/src/lib/notify'

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

// trigger lets callers swap in a more prominent CTA (e.g. the empty
// state, #93) without duplicating the dialog/form itself.
function NewProjectDialog({ trigger = <Button>+ New project</Button> }: { trigger?: ReactElement } = {}) {
  const navigate = useNavigate()
  const descriptionId = useId()
  const clarifyBaseId = useId()

  const [open, setOpen] = useState(false)
  const [description, setDescription] = useState('')
  const [step, setStep] = useState<Step>({ name: 'form' })
  // One answer per question in the current 'clarify' step, same index —
  // cleared on reset and whenever a fresh set of questions comes back.
  const [answers, setAnswers] = useState<string[]>([])
  // Cancels an in-flight poll when the dialog closes mid-wait, so a stale
  // response doesn't land after the user has moved on.
  const pollAbort = useRef<AbortController | null>(null)

  const trimmedDescription = description.trim()
  const working = step.name === 'working'

  function reset() {
    setDescription('')
    setAnswers([])
    setStep({ name: 'form' })
    pollAbort.current?.abort()
    pollAbort.current = null
  }

  // startRun dispatches create_project and polls it to a terminal status —
  // shared by the initial submit (round 0, no clarification) and the
  // clarify step's resubmit (round 1, with answers attached).
  async function startRun(clarification?: { round: number; clarifications: Clarification[] }) {
    setStep({ name: 'working', label: 'Starting…' })
    try {
      const { run_id } = await dispatchCreateProject(trimmedDescription, clarification)

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
    if (!trimmedDescription || working) return
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
          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <div className="flex flex-col gap-2">
              <Label htmlFor={descriptionId}>Describe your project</Label>
              <Textarea
                id={descriptionId}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="e.g. A CLI tool for indie developers to track tasks, shipping by end of Q2"
                autoFocus
                required
              />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={!trimmedDescription}>
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
