// Speculative client for a task-listing endpoint that doesn't exist on the
// real backend yet. internal/api/router.go only wires up /projects routes
// — ListTasks exists on store.Repository (from #82) but nothing calls it
// over HTTP, and there's no path that persists a task at all today
// (decompose_task only proposes them in memory; #77, the changeset-accept
// endpoint, is what would actually commit them). Only web/mock-server.mjs
// serves GET /projects/:id/tasks right now.
//
// Task mirrors internal/domain.Task field-for-field, plus `subtasks`
// nested on the mock response (empty array when a task has none).
import { getUserId } from '@/src/api/projects'

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

type TaskPayload = Omit<Task, 'subtasks'> & { subtasks?: TaskPayload[] }

function normalizeTask(task: TaskPayload): Task {
  return {
    ...task,
    subtasks: (task.subtasks ?? []).map(normalizeTask).sort((a, b) => a.order - b.order),
  }
}

export function flattenTasks(tasks: Task[]): Task[] {
  return tasks.flatMap((task) => [task, ...flattenTasks(task.subtasks)])
}

export async function listTasks(projectId: string): Promise<Task[]> {
  const params = new URLSearchParams({ user_id: getUserId() })
  const res = await fetch(`/api/projects/${encodeURIComponent(projectId)}/tasks?${params}`)
  if (!res.ok) {
    throw new Error(`list tasks failed with status ${res.status}`)
  }
  const body = (await res.json()) as TaskPayload[]
  return body.map(normalizeTask).sort((a, b) => a.order - b.order)
}
