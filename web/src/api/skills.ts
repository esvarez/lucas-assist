// Typed client for the async skill-dispatch flow (#123): POST /skills,
// poll GET /agent-runs/:id, and POST /projects/:id/changesets/:id/accept —
// the three routes architecture.md §1/§10 describes as "propose, poll,
// accept." Every model-backed operation is a job: dispatch returns 202
// with a run id immediately, nothing is written until an explicit accept.
import { request, type Project } from '@/src/api/projects'
import type { FlatTask } from '@/src/api/tasks'

// ProposedTask mirrors internal/domain.ProposedTask — a subtask as
// proposed, before the accept commit assigns it an id, order, and status.
export interface ProposedTask {
  title: string
  description: string
  acceptance_criteria: string[]
}

// ProposedProject mirrors internal/domain.ProposedProject — create_project's
// mutation payload, present on a Changeset instead of proposed_tasks (#154).
export interface ProposedProject {
  name: string
  goal: string
  deadline: string | null
  constraints: string[]
}

// Changeset mirrors internal/domain.Changeset. Exactly one of
// proposed_tasks (decompose_task) or proposed_project (create_project) is
// present, matching skill.
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
  proposed_project?: ProposedProject
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
// project_id is "" for a create_project run — it has no project yet.
export interface AgentRun {
  id: string
  user_id: string
  project_id: string
  skill: string
  status: AgentRunStatus
  error?: string
  // Clarifying questions, set only when status is "needs_input" (round 0
  // only) — see internal/domain.AgentRun.Questions. Supported by both
  // decompose_task and create_project.
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

// projectId is omitted for a create_project run — it has no project yet.
export async function getAgentRun(runId: string, projectId?: string): Promise<AgentRunResult> {
  const query = projectId ? `?${new URLSearchParams({ project_id: projectId })}` : ''
  return request<AgentRunResult>(`/api/agent-runs/${encodeURIComponent(runId)}${query}`)
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
  projectId?: string,
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
      body: JSON.stringify({ idempotency_key: idempotencyKey }),
    }
  )
}

// dispatchCreateProject starts a create_project run and returns its run id
// to poll (#154). No project_id — a create_project run doesn't have one
// yet, that's the whole point of the skill.
//
// clarification carries a prior needs_input round's answered questions
// (round 0 leaves it undefined), same contract as
// dispatchDecomposeTask's — see internal/agent/skills/create_project.go's
// CreateProjectInput.
export async function dispatchCreateProject(
  description: string,
  clarification?: { round: number; clarifications: Clarification[] }
): Promise<{ run_id: string }> {
  return request<{ run_id: string }>('/api/skills', {
    method: 'POST',
    body: JSON.stringify({
      skill: 'create_project',
      input: {
        description,
        clarification_round: clarification?.round ?? 0,
        clarifications: clarification?.clarifications ?? [],
      },
    }),
  })
}

// acceptCreateProject commits a proposed create_project changeset into a
// brand-new project (#152, #154) — the project-less counterpart to
// acceptChangeset above, since there's no existing project to address in
// a URL. Returns the created Project directly, not wrapped in a result
// object — there are no tasks or event for the caller to consume here.
export async function acceptCreateProject(changesetId: string, idempotencyKey: string): Promise<Project> {
  return request<Project>(`/api/changesets/${encodeURIComponent(changesetId)}/accept`, {
    method: 'POST',
    body: JSON.stringify({ idempotency_key: idempotencyKey }),
  })
}
