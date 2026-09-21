import { useId, useRef, useState, type FormEvent } from 'react'
import { SparklesIcon } from 'lucide-react'
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
import { Item, ItemContent, ItemDescription, ItemGroup, ItemTitle } from '@/components/ui/item'
import { Label } from '@/components/ui/label'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { ApiError } from '@/src/api/projects'
import { acceptChangeset, dispatchDecomposeTask, pollAgentRun, type Changeset } from '@/src/api/skills'
import { notify } from '@/src/lib/notify'

// Step is a small state machine covering the whole propose -> poll ->
// review -> accept loop (architecture.md §1) in one dialog: request the
// decomposition, wait for the Agent Worker, show the proposed subtasks for
// explicit review (ADR 002 — nothing is written without it), then commit.
type Step =
  | { name: 'form' }
  | { name: 'working'; label: string }
  | { name: 'review'; changeset: Changeset; idempotencyKey: string }
  | { name: 'error'; message: string }

function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === 'AbortError'
}

function DecomposeTaskDialog({
  projectId,
  onAccepted,
}: {
  projectId: string
  // Called after a successful accept — the caller is responsible for
  // refreshing its own task list (e.g. re-fetching GET
  // /projects/{id}/tasks) rather than this dialog trying to merge the
  // committed tasks into whatever shape the caller renders.
  onAccepted: () => void
}) {
  const titleId = useId()
  const descriptionId = useId()

  const [open, setOpen] = useState(false)
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [step, setStep] = useState<Step>({ name: 'form' })
  // Cancels an in-flight poll when the dialog closes mid-wait, so a
  // stale response doesn't land after the user has moved on.
  const pollAbort = useRef<AbortController | null>(null)

  const working = step.name === 'working'

  function reset() {
    setTitle('')
    setDescription('')
    setStep({ name: 'form' })
    pollAbort.current?.abort()
    pollAbort.current = null
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (working) return

    setStep({ name: 'working', label: 'Starting decomposition…' })
    try {
      const { run_id } = await dispatchDecomposeTask(projectId, title.trim(), description.trim())

      setStep({ name: 'working', label: 'Decomposing — this can take a few seconds…' })
      const controller = new AbortController()
      pollAbort.current = controller
      const run = await pollAgentRun(run_id, projectId, { signal: controller.signal })
      pollAbort.current = null

      if (run.status === 'failed') {
        setStep({ name: 'error', message: run.error || 'The decomposition failed.' })
        return
      }
      if (!run.changeset) {
        setStep({ name: 'error', message: 'The run completed without a proposal to review.' })
        return
      }

      setStep({ name: 'review', changeset: run.changeset, idempotencyKey: crypto.randomUUID() })
    } catch (err) {
      // The dialog was closed (or reset) mid-poll, which aborts on
      // purpose — reset() already put the dialog back in its initial
      // state, so this isn't a real failure to report.
      if (isAbortError(err)) return
      setStep({ name: 'error', message: err instanceof Error ? err.message : 'Something went wrong' })
    }
  }

  async function handleAccept() {
    if (step.name !== 'review') return
    const { changeset, idempotencyKey } = step

    setStep({ name: 'working', label: 'Committing subtasks…' })
    try {
      await acceptChangeset(projectId, changeset.id, idempotencyKey)
      onAccepted()
      notify.success('Subtasks added')
      setOpen(false)
      reset()
    } catch (err) {
      notify.error('Failed to commit subtasks')
      const message =
        err instanceof ApiError && err.status === 409
          ? 'This proposal can no longer be accepted — the project may have changed since it was generated, or it was already applied. Close this and try decomposing again.'
          : err instanceof Error
            ? err.message
            : 'Failed to commit the subtasks'
      setStep({ name: 'error', message })
    }
  }

  const proposedTasks = step.name === 'review' ? (step.changeset.proposed_tasks ?? []) : []

  return (
    <Dialog
      open={open}
      onOpenChange={(next: boolean) => {
        setOpen(next)
        if (!next) reset()
      }}
    >
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        <SparklesIcon data-icon="inline-start" />
        Decompose a task
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Decompose a task</DialogTitle>
        </DialogHeader>

        {step.name === 'form' && (
          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <div className="flex flex-col gap-2">
              <Label htmlFor={titleId}>Task title</Label>
              <Input
                id={titleId}
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                placeholder="e.g. Add sync command to the CLI"
                required
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor={descriptionId}>Description</Label>
              <Textarea
                id={descriptionId}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="What does done look like for this task?"
              />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={!title.trim()}>
                Propose subtasks
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
              Review the proposed subtasks below. Nothing is saved until you accept.
            </p>
            <ItemGroup className="max-h-[50vh] overflow-y-auto">
              {proposedTasks.map((task, index) => (
                <Item key={index} variant="outline" size="sm">
                  <ItemContent>
                    <ItemTitle>{task.title}</ItemTitle>
                    {task.description && <ItemDescription>{task.description}</ItemDescription>}
                    {task.acceptance_criteria.length > 0 && (
                      <ul className="list-inside list-disc text-xs text-muted-foreground">
                        {task.acceptance_criteria.map((criterion, i) => (
                          <li key={i}>{criterion}</li>
                        ))}
                      </ul>
                    )}
                  </ItemContent>
                </Item>
              ))}
            </ItemGroup>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button type="button" onClick={handleAccept}>
                Accept {proposedTasks.length} subtask{proposedTasks.length === 1 ? '' : 's'}
              </Button>
            </DialogFooter>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

export default DecomposeTaskDialog
