import { useId, useState, type FormEvent, type ReactElement } from 'react'
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
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import type { Clarification } from '@/src/api/skills'
import { ClarifyStep, ErrorStep, ReviewStep, WorkingStep } from '@/src/components/decompose-steps'
import { useDecomposeRun } from '@/src/hooks/useDecomposeRun'
import { notify } from '@/src/lib/notify'

const DEFAULT_TRIGGER = (
  <Button variant="outline" size="sm">
    <SparklesIcon data-icon="inline-start" />
    Break down
  </Button>
)

// DecomposeTaskDialog covers the manual, form-first entry points — the
// user has one arbitrary task in mind and types it in. The empty-state
// "Break into tasks" CTA runs the same decompose_task loop but in the
// background instead: see BreakIntoTasksButton (#166), which shares this
// state machine via useDecomposeRun rather than duplicating it.
function DecomposeTaskDialog({
  projectId,
  onAccepted,
  trigger = DEFAULT_TRIGGER,
}: {
  projectId: string
  // Called after a successful accept — the caller is responsible for
  // refreshing its own task list (e.g. re-fetching GET
  // /projects/{id}/tasks) rather than this dialog trying to merge the
  // committed tasks into whatever shape the caller renders.
  onAccepted: () => void
  // Lets callers place this dialog behind a differently styled/labeled
  // entry point (e.g. the Tasks section header vs. an empty-state CTA)
  // without duplicating the propose/review/accept flow.
  trigger?: ReactElement
}) {
  const titleId = useId()
  const descriptionId = useId()

  const run = useDecomposeRun(projectId, onAccepted)
  const [open, setOpen] = useState(false)
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  // One answer per question in the current 'clarify' state, same index —
  // cleared whenever a fresh set of questions comes back.
  const [answers, setAnswers] = useState<string[]>([])

  // Derived, not synced via an effect: pads/truncates the raw answers to
  // the current clarify state's question count on every render, so
  // ClarifyStep always sees one slot per question regardless of how many
  // the user has typed into so far.
  const clarifyAnswers =
    run.state.name === 'clarify'
      ? Array.from({ length: run.state.questions.length }, (_, i) => answers[i] ?? '')
      : answers

  function reset() {
    setTitle('')
    setDescription('')
    setAnswers([])
    run.reset()
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (run.state.name === 'working') return
    await run.start(title, description)
  }

  async function handleClarifySubmit(e: FormEvent) {
    e.preventDefault()
    if (run.state.name !== 'clarify') return
    const clarifications: Clarification[] = run.state.questions.map((question, index) => ({
      question,
      answer: clarifyAnswers[index]?.trim() ?? '',
    }))
    await run.start(title, description, { round: 1, clarifications })
  }

  async function handleAccept() {
    const ok = await run.accept()
    if (ok) {
      notify.success('Subtasks added')
      setOpen(false)
      reset()
    } else {
      notify.error('Failed to commit subtasks')
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
          <DialogTitle>Decompose a task</DialogTitle>
        </DialogHeader>

        {run.state.name === 'idle' && (
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

        {run.state.name === 'working' && <WorkingStep label={run.state.label} />}

        {run.state.name === 'clarify' && (
          <ClarifyStep
            questions={run.state.questions}
            answers={clarifyAnswers}
            onAnswerChange={(index, value) =>
              setAnswers((prev) => {
                const next = [...prev]
                next[index] = value
                return next
              })
            }
            onSubmit={handleClarifySubmit}
            onCancel={() => setOpen(false)}
          />
        )}

        {run.state.name === 'error' && (
          <ErrorStep message={run.state.message} onClose={() => setOpen(false)} onRetry={() => run.reset()} />
        )}

        {run.state.name === 'review' && (
          <ReviewStep changeset={run.state.changeset} onAccept={handleAccept} onCancel={() => setOpen(false)} />
        )}
      </DialogContent>
    </Dialog>
  )
}

export default DecomposeTaskDialog
