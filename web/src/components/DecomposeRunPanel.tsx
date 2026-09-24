import { useState, type FormEvent } from 'react'
import type { AcceptTaskSelection, Clarification, ProposedTask } from '@/src/api/skills'
import { ClarifyStep, ErrorStep, ReviewStep, WorkingStep } from '@/src/components/decompose-steps'
import type { useDecomposeRun } from '@/src/hooks/useDecomposeRun'
import { notify } from '@/src/lib/notify'

// DecomposeRunPanel renders a useDecomposeRun's current state inline in
// the Tasks section (#169) — the shared body every decompose_task entry
// point on a project page drops into once a run exists, instead of
// duplicating this per-trigger or wrapping it in a dialog. Renders
// nothing while idle.
function DecomposeRunPanel({
  run,
  title,
  description,
}: {
  run: ReturnType<typeof useDecomposeRun>
  // The title/description this run was (or will be, on retry) dispatched
  // with — the caller remembers these across the run's lifecycle since
  // the form that collected them (if any) has already closed.
  title: string
  description: string
}) {
  // One answer per question in the current 'clarify' state, same index —
  // local until submitted, since only accept/remove persist server-side.
  const [answers, setAnswers] = useState<string[]>([])

  if (run.state.name === 'idle') return null

  // Derived, not synced via an effect: pads/truncates the raw answers to
  // the current clarify state's question count on every render.
  const clarifyAnswers =
    run.state.name === 'clarify'
      ? Array.from({ length: run.state.questions.length }, (_, i) => answers[i] ?? '')
      : answers

  function handleClarifySubmit(e: FormEvent) {
    e.preventDefault()
    if (run.state.name !== 'clarify') return
    const clarifications: Clarification[] = run.state.questions.map((question, index) => ({
      question,
      answer: clarifyAnswers[index]?.trim() ?? '',
    }))
    void run.start(title, description, { round: 1, clarifications })
  }

  async function handleAcceptSelections(selections: AcceptTaskSelection[]) {
    const ok = await run.accept(selections)
    if (ok) {
      notify.success(selections.length === 1 ? 'Subtask added' : 'Subtasks added')
    } else {
      notify.error('Failed to commit subtasks')
    }
  }

  async function handleRemoveTasks(remaining: ProposedTask[]) {
    const ok = await run.removeTasks(remaining)
    if (!ok) {
      notify.error('Failed to update the proposal')
    }
  }

  return (
    <>
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
          onCancel={run.reset}
        />
      )}

      {run.state.name === 'review' && (
        <ReviewStep
          changeset={run.state.changeset}
          onAcceptSelections={handleAcceptSelections}
          onRemoveTasks={handleRemoveTasks}
        />
      )}

      {run.state.name === 'error' && (
        <ErrorStep
          message={run.state.message}
          onClose={run.reset}
          // No title to redispatch with means this is a proposal
          // rediscovered after a refresh (#169) — retrying from scratch
          // would dispatch decompose_task with a blank task instead.
          onRetry={title.trim() ? () => void run.start(title, description) : undefined}
        />
      )}
    </>
  )
}

export default DecomposeRunPanel
