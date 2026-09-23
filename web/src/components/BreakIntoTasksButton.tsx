import { useEffect, useRef, useState, type FormEvent } from 'react'
import { SparklesIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Spinner } from '@/components/ui/spinner'
import type { Project } from '@/src/api/projects'
import { ClarifyStep, ErrorStep, ReviewStep, WorkingStep } from '@/src/components/decompose-steps'
import { useDecomposeRun } from '@/src/hooks/useDecomposeRun'
import { notify } from '@/src/lib/notify'

// projectSeed derives decompose_task's task_title/task_description from
// the project's own card (#163) — this button breaks down a brand-new
// project, not one task the user has in mind, so there's nothing to ask
// for that isn't already on the card.
function projectSeed(project: Project): { title: string; description: string } {
  const description =
    project.constraints.length > 0
      ? `${project.goal}\n\nConstraints: ${project.constraints.join('; ')}`
      : project.goal
  return { title: project.name, description }
}

// BreakIntoTasksButton runs decompose_task in the background instead of
// behind a dialog (#166): a fresh project's empty state has nothing to
// wait on the user for, so blocking the page with a modal while the Agent
// Worker runs has no purpose. Progress and completion surface as a toast;
// a dialog opens only once there's something to act on — answering a
// clarifying question or reviewing/accepting the proposed subtasks (ADR
// 002 still requires explicit acceptance before anything is written).
function BreakIntoTasksButton({ project, onAccepted }: { project: Project; onAccepted: () => void }) {
  const run = useDecomposeRun(project.id, onAccepted)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [answers, setAnswers] = useState<string[]>([])
  const toastId = useRef<string | number | null>(null)

  function dispatch() {
    const seed = projectSeed(project)
    toastId.current = notify.loading('Breaking your project into tasks…')
    void run.start(seed.title, seed.description)
  }

  function openDialog() {
    if (toastId.current !== null) {
      notify.dismiss(toastId.current)
      toastId.current = null
    }
    setDialogOpen(true)
  }

  // Drives the toast while the dialog is closed. Once the dialog opens
  // (via an action toast's onClick), it takes over presenting
  // 'working'/'clarify'/'review'/'error' directly, the same way
  // DecomposeTaskDialog does — the toast and the dialog never show at once.
  useEffect(() => {
    if (dialogOpen) return

    if (run.state.name === 'working') {
      if (toastId.current !== null) notify.updateLoading(toastId.current, run.state.label)
    } else if (run.state.name === 'clarify') {
      toastId.current = notify.action(
        toastId.current,
        'A couple of details would help — Nudge has questions before it can break this project down.',
        { label: 'Answer', onClick: openDialog }
      )
    } else if (run.state.name === 'review') {
      const count = run.state.changeset.proposed_tasks?.length ?? 0
      toastId.current = notify.action(
        toastId.current,
        `${count} subtask${count === 1 ? '' : 's'} ready to review`,
        { label: 'Review', onClick: openDialog }
      )
    } else if (run.state.name === 'error') {
      const id = toastId.current ?? undefined
      toastId.current = null
      notify.error(run.state.message, { id, retry: dispatch })
      run.reset()
    }
    // dispatch/openDialog close over `project`/`run`, both stable for the
    // lifetime of this component — re-running this effect only on state
    // transitions (not on every render) is what keeps one toast per run.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [run.state, dialogOpen])

  // Derived, not synced via an effect: pads/truncates the raw answers to
  // the current clarify state's question count on every render.
  const clarifyAnswers =
    run.state.name === 'clarify'
      ? Array.from({ length: run.state.questions.length }, (_, i) => answers[i] ?? '')
      : answers

  function handleClarifySubmit(e: FormEvent) {
    e.preventDefault()
    if (run.state.name !== 'clarify') return
    const clarifications = run.state.questions.map((question, index) => ({
      question,
      answer: clarifyAnswers[index]?.trim() ?? '',
    }))
    const seed = projectSeed(project)
    void run.start(seed.title, seed.description, { round: 1, clarifications })
  }

  async function handleAccept() {
    const ok = await run.accept()
    if (ok) {
      notify.success('Subtasks added')
      setDialogOpen(false)
    } else {
      notify.error('Failed to commit subtasks')
    }
  }

  function closeDialog() {
    setDialogOpen(false)
    run.reset()
  }

  return (
    <>
      <Button onClick={dispatch} disabled={run.state.name !== 'idle'}>
        {run.state.name === 'working' ? (
          <Spinner data-icon="inline-start" className="size-4" />
        ) : (
          <SparklesIcon data-icon="inline-start" />
        )}
        Break into tasks
      </Button>

      <Dialog open={dialogOpen} onOpenChange={(next) => (next ? setDialogOpen(true) : closeDialog())}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Break into tasks</DialogTitle>
          </DialogHeader>
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
              onCancel={closeDialog}
            />
          )}
          {run.state.name === 'review' && (
            <ReviewStep changeset={run.state.changeset} onAccept={handleAccept} onCancel={closeDialog} />
          )}
          {run.state.name === 'error' && (
            <ErrorStep
              message={run.state.message}
              onClose={closeDialog}
              onRetry={() => {
                closeDialog()
                dispatch()
              }}
            />
          )}
        </DialogContent>
      </Dialog>
    </>
  )
}

export default BreakIntoTasksButton
