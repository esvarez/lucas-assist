import { useEffect, useRef, useState } from 'react'
import { ApiError } from '@/src/api/projects'
import {
  acceptChangeset,
  dispatchDecomposeTask,
  pollAgentRun,
  type Changeset,
  type Clarification,
} from '@/src/api/skills'

// RunState is the propose -> poll -> review -> accept loop
// (architecture.md §1) shared by every decompose_task entry point: request
// the decomposition, wait for the Agent Worker, surface the proposed
// subtasks for explicit review (ADR 002 — nothing is written without it),
// then commit. 'idle' covers "nothing in flight" — what a caller shows for
// it (a form, a plain button) is up to the caller.
//
// 'clarify' handles decompose_task's needs_clarification result (#138):
// round 0 only, at most 3 questions. Answering resubmits as a fresh
// dispatch with clarification_round: 1 — the run that asked is done
// (AgentRunNeedsInput is terminal), not resumed.
export type RunState =
  | { name: 'idle' }
  | { name: 'working'; label: string }
  | { name: 'clarify'; questions: string[] }
  | { name: 'review'; changeset: Changeset; idempotencyKey: string }
  | { name: 'error'; message: string }

function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === 'AbortError'
}

// useDecomposeRun owns decompose_task's dispatch/poll/accept mechanics so
// a form-driven dialog and a background-driven entry point (#166) share
// one implementation instead of two copies of the same state machine.
export function useDecomposeRun(projectId: string, onAccepted: () => void) {
  const [state, setState] = useState<RunState>({ name: 'idle' })
  // Cancels an in-flight poll on reset (or unmount) so a stale response
  // doesn't land after the caller has moved on.
  const pollAbort = useRef<AbortController | null>(null)

  useEffect(() => {
    return () => pollAbort.current?.abort()
  }, [])

  function reset() {
    pollAbort.current?.abort()
    pollAbort.current = null
    setState({ name: 'idle' })
  }

  async function start(
    title: string,
    description: string,
    clarification?: { round: number; clarifications: Clarification[] }
  ) {
    setState({ name: 'working', label: 'Starting decomposition…' })
    try {
      const { run_id } = await dispatchDecomposeTask(
        projectId,
        title.trim(),
        description.trim(),
        clarification
      )

      setState({ name: 'working', label: 'Decomposing — this can take a few seconds…' })
      const controller = new AbortController()
      pollAbort.current = controller
      const run = await pollAgentRun(run_id, projectId, { signal: controller.signal })
      pollAbort.current = null

      if (run.status === 'failed') {
        setState({ name: 'error', message: run.error || 'The decomposition failed.' })
        return
      }
      if (run.status === 'needs_input') {
        setState({ name: 'clarify', questions: run.questions ?? [] })
        return
      }
      if (!run.changeset) {
        setState({ name: 'error', message: 'The run completed without a proposal to review.' })
        return
      }

      setState({ name: 'review', changeset: run.changeset, idempotencyKey: crypto.randomUUID() })
    } catch (err) {
      // Aborted on purpose (reset(), or the caller unmounted) — not a
      // real failure to report.
      if (isAbortError(err)) return
      setState({ name: 'error', message: err instanceof Error ? err.message : 'Something went wrong' })
    }
  }

  // accept commits the current 'review' state's changeset. It reports
  // success/failure back to the caller rather than picking its own
  // success/error UI, since a dialog closing and a toast look different.
  async function accept(): Promise<boolean> {
    if (state.name !== 'review') return false
    const { changeset, idempotencyKey } = state

    setState({ name: 'working', label: 'Committing subtasks…' })
    try {
      await acceptChangeset(projectId, changeset.id, idempotencyKey)
      onAccepted()
      reset()
      return true
    } catch (err) {
      const message =
        err instanceof ApiError && err.status === 409
          ? 'This proposal can no longer be accepted — the project may have changed since it was generated, or it was already applied. Try decomposing again.'
          : err instanceof Error
            ? err.message
            : 'Failed to commit the subtasks'
      setState({ name: 'error', message })
      return false
    }
  }

  return { state, start, accept, reset }
}
