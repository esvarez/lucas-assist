// Mirrors internal/domain.Project and the createProjectRequest shape
// documented in docs/openapi.yaml.
export type Project = {
  user_id: string
  id: string
  name: string
  goal: string
  deadline: string | null
  constraints: string[]
  status: string
  created_at: string
  updated_at: string
}

export type CreateProjectInput = {
  name: string
}

// No authentication exists yet (architecture.md §2: "The POC has no
// authentication"). user_id is caller-supplied until Cognito lands, so a
// fixed dev id keeps local testing scoped to one stable partition instead
// of inventing a new one per request.
const DEV_USER_ID = 'dev-user'

export async function createProject(input: CreateProjectInput): Promise<Project> {
  const res = await fetch('/api/projects', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      user_id: DEV_USER_ID,
      name: input.name,
      goal: '',
      constraints: [],
      status: 'on-track',
    }),
  })

  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as { error?: string } | null
    throw new Error(body?.error ?? `create project failed: ${res.status}`)
  }

  return res.json() as Promise<Project>
}
