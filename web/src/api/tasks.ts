// Typed client for GET /projects/:id/tasks (#125), served for real by
// cmd/api / cmd/local now — previously this only worked against
// web/mock-server.mjs (see git history). The backend returns tasks flat,
// linked by parent_id, exactly as internal/domain.Task shapes them; this
// module is what turns that into the nested `subtasks` tree the rest of
// the app (ProjectDetailPage, WhatsNextCard) renders.
import { request } from '@/src/api/projects'

// Task mirrors internal/domain.Task field-for-field, plus a client-side
// `subtasks` array built by buildTaskTree — the wire response has no such
// field.
export interface Task {
  id: string
  project_id: string
  parent_id: string
  title: string
  description: string
  status: string
  order: number
  acceptance_criteria: string[]
  subtasks: Task[]
}

// FlatTask is the wire shape: internal/domain.Task as-is, no subtasks.
// Exported so other clients returning the same shape (e.g.
// web/src/api/skills.ts's accept-changeset response) can reuse it instead
// of redefining it.
export type FlatTask = Omit<Task, 'subtasks'>

// buildTaskTree groups a flat, parent_id-linked list into a tree: tasks
// with no parent_id (or one that isn't present in the list) are roots.
// Each level is sorted by order, matching how the backend orders subtasks
// within their parent.
export function buildTaskTree(flat: FlatTask[]): Task[] {
  const byParent = new Map<string, FlatTask[]>()
  const ids = new Set(flat.map((task) => task.id))
  for (const task of flat) {
    const parentKey = task.parent_id && ids.has(task.parent_id) ? task.parent_id : ''
    const siblings = byParent.get(parentKey) ?? []
    siblings.push(task)
    byParent.set(parentKey, siblings)
  }

  function attach(parentKey: string): Task[] {
    const siblings = byParent.get(parentKey) ?? []
    return siblings
      .map((task) => ({ ...task, subtasks: attach(task.id) }))
      .sort((a, b) => a.order - b.order)
  }

  return attach('')
}

export function flattenTasks(tasks: Task[]): Task[] {
  return tasks.flatMap((task) => [task, ...flattenTasks(task.subtasks)])
}

export async function listTasks(projectId: string): Promise<Task[]> {
  const body = await request<FlatTask[]>(`/api/projects/${encodeURIComponent(projectId)}/tasks`)
  return buildTaskTree(body)
}

// CreateTaskRequest mirrors docs/openapi.yaml's CreateTaskRequest — a
// single task added by hand (#175), outside decompose_task's
// propose/accept flow. parent_id nests it under an existing task.
export interface CreateTaskRequest {
  title: string
  description?: string
  acceptance_criteria?: string[]
  parent_id?: string
}

export function createTask(projectId: string, input: CreateTaskRequest): Promise<FlatTask> {
  return request<FlatTask>(`/api/projects/${encodeURIComponent(projectId)}/tasks`, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}
