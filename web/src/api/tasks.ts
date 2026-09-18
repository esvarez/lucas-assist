// Speculative client for a task-listing endpoint that doesn't exist on the
// real backend yet. internal/api/router.go only wires up /projects routes
// — ListTasks exists on store.Repository (from #82) but nothing calls it
// over HTTP, and there's no path that persists a task at all today
// (decompose_task only proposes them in memory; #77, the changeset-accept
// endpoint, is what would actually commit them). Only web/mock-server.mjs
// serves GET /projects/:id/tasks right now.
//
// Task mirrors internal/domain.Task field-for-field, same convention as
// Project in ./projects.ts, so this is a straight port once the real
// endpoint exists.
import { getUserId } from './projects'

export interface Task {
  id: string
  project_id: string
  parent_id: string
  title: string
  description: string
  status: string
  order: number
  acceptance_criteria: string[]
}

export async function listTasks(projectId: string): Promise<Task[]> {
  const params = new URLSearchParams({ user_id: getUserId() })
  const res = await fetch(`/api/projects/${encodeURIComponent(projectId)}/tasks?${params}`)
  if (!res.ok) {
    throw new Error(`list tasks failed with status ${res.status}`)
  }
  return res.json() as Promise<Task[]>
}
