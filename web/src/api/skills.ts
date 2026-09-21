// Typed client for the async skill-dispatch flow (#123): POST /skills,
// poll GET /agent-runs/:id, and POST /projects/:id/changesets/:id/accept —
// the three routes architecture.md §1/§10 describes as "propose, poll,
// accept." Every model-backed operation is a job: dispatch returns 202
// with a run id immediately, nothing is written until an explicit accept.
import { getUserId, request, type Project } from '@/src/api/projects'
import type { FlatTask } from '@/src/api/tasks'

// ProposedTask mirrors internal/domain.ProposedTask — a subtask as
// proposed, before the accept commit assigns it an id, order, and status.
export interface ProposedTask {
  title: string
  description: string
  acceptance_criteria: string[]
}

// Changeset mirrors internal/domain.Changeset's decompose_task shape.
// proposed_project is omitted — create_project's changeset shape isn't
// used by this client (#123 is scoped to decompose_task only).
export interface Changeset {
  id: string
  project_id: string
  user_id: string
  skill: string
  base_version: number
  status: string
  proposed_tasks?: ProposedTask[]
  // Unspecified choices the model made on the user's behalf while
  // decomposing — rendered above the subtask list in review, since these
  // are what's most likely to need correcting.
  assumptions?: string[]
  created_at: string
}

export type AgentRunStatus = 'queued' | 'running' | 'completed' | 'failed' | 'cancelled' | 'needs_input'

// Clarification mirrors internal/agent/skills.Clarification — one prior
// round's question with the user's answer attached.
export interface Clarification {
  question: string
  answer: string
}

// AgentRun mirrors internal/domain.AgentRun's client-relevant fields.
export interface AgentRun {
  id: string
  user_id: string
  project_id: string
  skill: string
  status: AgentRunStatus
  error?: string
  // decompose_task's clarifying questions, set only when status is
  // "needs_input" (round 0 only) — see internal/domain.AgentRun.Questions.
  questions?: string[]
  changeset_id?: string
  created_at: string
  updated_at: string
}

// AgentRunResult mirrors internal/api.agentRunResponse: the run plus its
// Changeset once one exists (only once status is "completed").
export interface AgentRunResult extends AgentRun {
  changeset?: Changeset
}

export interface AcceptChangesetResult {
  project: Project
  tasks: FlatTask[]
}

// dispatchDecomposeTask starts a decompose_task run for projectId and
// returns its run id to poll. domain is left unset — the skill defaults
// to "general" — since #123 doesn't add a domain picker to the UI.
//
// clarification carries a prior needs_input round's answered questions
// (round 0 leaves it undefined). Each answered round is a fresh dispatch,
// not a resume of the run that asked — the skill is stateless and
// round-scoped (internal/agent/skills/decompose_task.go's
// DecomposeInput).
export async function dispatchDecomposeTask(
  projectId: string,
  taskTitle: string,
  taskDescription: string,
  clarification?: { round: number; clarifications: Clarification[] }
): Promise<{ run_id: string }> {
  return request<{ run_id: string }>('/api/skills', {
    method: 'POST',
    body: JSON.stringify({
      skill: 'decompose_task',
      user_id: getUserId(),
      project_id: projectId,
      input: {
        task_title: taskTitle,
        task_description: taskDescription,
        project_id: projectId,
        clarification_round: clarification?.round ?? 0,
        clarifications: clarification?.clarifications ?? [],
      },
    }),
  })
}

export async function getAgentRun(runId: string, projectId: string): Promise<AgentRunResult> {
  const params = new URLSearchParams({ user_id: getUserId(), project_id: projectId })
  return request<AgentRunResult>(`/api/agent-runs/${encodeURIComponent(runId)}?${params}`)
}

// PollOptions bounds pollAgentRun (architecture.md §10 step 8: "poll ...
// with bounded backoff") — it must not poll forever against a stuck or
// lost run.
export interface PollOptions {
  signal?: AbortSignal
  timeoutMs?: number
}

const POLL_INITIAL_DELAY_MS = 1000
const POLL_MAX_DELAY_MS = 5000
const POLL_BACKOFF_FACTOR = 1.5
const POLL_DEFAULT_TIMEOUT_MS = 120_000

function sleep(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(resolve, ms)
    signal?.addEventListener('abort', () => {
      clearTimeout(timer)
      reject(new DOMException('Aborted', 'AbortError'))
    })
  })
}

function isTerminal(status: AgentRunStatus): boolean {
  return status === 'completed' || status === 'failed' || status === 'cancelled' || status === 'needs_input'
}

// pollAgentRun polls GET /agent-runs/:id with increasing backoff
// (1s, capped at 5s) until the run reaches a terminal status, or throws
// once timeoutMs has elapsed — a decompose_task run normally finishes in
// a few seconds, so two minutes is a generous ceiling before treating it
// as stuck rather than polling indefinitely.
export async function pollAgentRun(
  runId: string,
  projectId: string,
  { signal, timeoutMs = POLL_DEFAULT_TIMEOUT_MS }: PollOptions = {}
): Promise<AgentRunResult> {
  const deadline = Date.now() + timeoutMs
  let delay = POLL_INITIAL_DELAY_MS

  for (;;) {
    const run = await getAgentRun(runId, projectId)
    if (isTerminal(run.status)) {
      return run
    }
    if (Date.now() >= deadline) {
      throw new Error(
        'Timed out waiting for the decomposition to finish. It may still complete — check back on the project shortly.'
      )
    }
    await sleep(delay, signal)
    delay = Math.min(delay * POLL_BACKOFF_FACTOR, POLL_MAX_DELAY_MS)
  }
}

export async function acceptChangeset(
  projectId: string,
  changesetId: string,
  idempotencyKey: string
): Promise<AcceptChangesetResult> {
  return request<AcceptChangesetResult>(
    `/api/projects/${encodeURIComponent(projectId)}/changesets/${encodeURIComponent(changesetId)}/accept`,
    {
      method: 'POST',
      body: JSON.stringify({ user_id: getUserId(), idempotency_key: idempotencyKey }),
    }
  )
}
