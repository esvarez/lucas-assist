import { useEffect, useRef, useState } from 'react'
import { ApiError } from '@/src/api/projects'
import {
  acceptChangeset,
  dispatchDecomposeTask,
  pollAgentRun,
  updateChangesetTasks,
  type AcceptTaskSelection,
  type Changeset,
  type Clarification,
  type ProposedTask,
} from '@/src/api/skills'

// RunState is the propose -> poll -> review -> accept loop
// (architecture.md §1) shared by every decompose_task entry point: request
// the decomposition, wait for the Agent Worker, surface the proposed
// subtasks for explicit review (ADR 002 — nothing is written without it),
// then commit some or all of them (#169 — a changeset review no longer has
// to be all-or-nothing, so 'review' persists across accept/remove calls
// until nothing is left proposed). 'idle' covers "nothing in flight" —
// what a caller shows for it (a form, a plain button) is up to the caller.
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

// useDecomposeRun owns decompose_task's dispatch/poll/accept/remove
// mechanics so every entry point on a project page (a fresh project's
// empty state, an arbitrary task's manual form) shares one implementation
// and one in-flight proposal instead of independent copies.
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

  // resumeReview puts an already-proposed changeset (e.g. one
  // rediscovered via listChangesets after a page refresh, #169) straight
  // into 'review' — no dispatch, no poll.
  function resumeReview(changeset: Changeset) {
    setState({ name: 'review', changeset, idempotencyKey: crypto.randomUUID() })
  }

  // pollAndResolve polls an already-dispatched run to a terminal status and
  // transitions state accordingly — shared by start() (right after
  // dispatching) and resumePoll() (picking a run back up after a page
  // refresh, #169), so there's one poll-then-transition implementation
  // instead of two.
  async function pollAndResolve(runId: string) {
    setState({ name: 'working', label: 'Decomposing — this can take a few seconds…' })
    try {
      const controller = new AbortController()
      pollAbort.current = controller
      const run = await pollAgentRun(runId, projectId, { signal: controller.signal })
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

      resumeReview(run.changeset)
    } catch (err) {
      // Aborted on purpose (reset(), or the caller unmounted) — not a
      // real failure to report.
      if (isAbortError(err)) return
      setState({ name: 'error', message: err instanceof Error ? err.message : 'Something went wrong' })
    }
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
      await pollAndResolve(run_id)
    } catch (err) {
      if (isAbortError(err)) return
      setState({ name: 'error', message: err instanceof Error ? err.message : 'Something went wrong' })
    }
  }

  // resumePoll picks up polling an already-dispatched, still queued/
  // running run rediscovered after a page refresh (#169) — no new
  // dispatch, so this never sends a fresh clarification_round: 0 request
  // on top of one already in flight; it just waits for the one that
  // exists.
  function resumePoll(runId: string) {
    void pollAndResolve(runId)
  }

  // accept commits the current 'review' state's selected tasks (by index,
  // with possibly-edited content — #169). Anything not selected stays
  // proposed: the state moves back to 'review' with the changeset the
  // response returned (a fresh idempotency key for the next call) rather
  // than resetting, unless nothing is left, in which case it's done. It
  // reports success/failure back to the caller rather than picking its
  // own success/error UI, since different callers want different toasts.
  async function accept(selections: AcceptTaskSelection[]): Promise<boolean> {
    if (state.name !== 'review') return false
    const { changeset, idempotencyKey } = state

    setState({ name: 'working', label: 'Committing…' })
    try {
      const result = await acceptChangeset(projectId, changeset.id, idempotencyKey, selections)
      onAccepted()
      if ((result.changeset.proposed_tasks?.length ?? 0) > 0) {
        resumeReview(result.changeset)
      } else {
        reset()
      }
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

  // removeTasks persists the current 'review' state's changeset with
  // `remaining` as its new proposed_tasks (#169) — no Task rows, just
  // discarding what isn't in `remaining`. An empty `remaining` rejects the
  // changeset (the backend's call), which reset() reflects here as 'idle'.
  async function removeTasks(remaining: ProposedTask[]): Promise<boolean> {
    if (state.name !== 'review') return false

    try {
      const { changeset } = await updateChangesetTasks(projectId, state.changeset.id, remaining)
      if ((changeset.proposed_tasks?.length ?? 0) > 0) {
        resumeReview(changeset)
      } else {
        reset()
      }
      return true
    } catch (err) {
      const message =
        err instanceof ApiError && err.status === 409
          ? 'This proposal changed elsewhere and could not be updated. Try decomposing again.'
          : err instanceof Error
            ? err.message
            : 'Failed to update the proposal'
      setState({ name: 'error', message })
      return false
    }
  }

  return { state, start, accept, removeTasks, resumeReview, resumePoll, reset }
}
