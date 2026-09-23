// Typed client for the Projects CRUD routes (docs/openapi.yaml), served by
// cmd/api / cmd/local. Requests go to /api/projects* — the dev server
// proxies that to the local API (vite.config.ts) and production is
// expected to route it the same way (architecture.md §13).
//
// Auth: every route requires a valid Cognito access token (architecture.md
// §14, #81) — the backend resolves the caller from the token's verified
// claims, not from a request-supplied user_id. request() attaches it as
// `Authorization: Bearer <token>` itself so callers never pass one.
import { getAccessToken } from '@/src/lib/cognito'

const BASE_PATH = '/api/projects'

export type ProjectDomain = 'software' | 'general'

// Project mirrors the Project schema in docs/openapi.yaml field-for-field
// (snake_case, matching the Go backend's JSON tags directly).
export interface Project {
  user_id: string
  id: string
  name: string
  goal: string
  deadline: string | null
  constraints: string[]
  status: string
  domain: ProjectDomain
  version: number
  created_at: string
  updated_at: string
}

// user_id is deliberately absent — see the file header, it's attached
// automatically rather than taken from the caller.
export interface CreateProjectRequest {
  name?: string
  goal?: string
  deadline?: string | null
  constraints?: string[]
  status?: string
  domain?: ProjectDomain
}

// name is intentionally absent too — PUT /projects/{id} never touches it
// (docs/openapi.yaml's UpdateProjectRequest). version is required: pass
// the value from the Project you loaded (see Project.version) so the
// backend can detect a conflicting concurrent edit.
export interface UpdateProjectRequest {
  version: number
  goal?: string
  deadline?: string | null
  constraints?: string[]
  status?: string
}

export interface ErrorResponse {
  error: string
}

export interface ValidationErrorResponse {
  error: string
  fields: Record<string, string>
}

export class ApiError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

// Thrown for a 400 that carries per-field messages, so a form can show
// each failing field instead of one generic error string.
export class ValidationError extends ApiError {
  readonly fields: Record<string, string>

  constructor(status: number, message: string, fields: Record<string, string>) {
    super(status, message)
    this.name = 'ValidationError'
    this.fields = fields
  }
}

function isValidationErrorResponse(body: unknown): body is ValidationErrorResponse {
  return (
    typeof body === 'object' &&
    body !== null &&
    'fields' in body &&
    typeof (body as { fields: unknown }).fields === 'object'
  )
}

// Exported so other API clients (web/src/api/skills.ts, tasks.ts) share the
// same fetch/auth/error-mapping behavior instead of reimplementing it.
export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const token = await getAccessToken()
  const res = await fetch(path, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...init?.headers,
    },
  })

  if (res.status === 204) {
    return undefined as T
  }

  const body: unknown = await res.json().catch(() => null)

  if (!res.ok) {
    if (res.status === 400 && isValidationErrorResponse(body)) {
      throw new ValidationError(res.status, body.error, body.fields)
    }
    const message = (body as ErrorResponse | null)?.error ?? `request failed with status ${res.status}`
    throw new ApiError(res.status, message)
  }

  return body as T
}

export function createProject(input: CreateProjectRequest): Promise<Project> {
  return request<Project>(BASE_PATH, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export function listProjects(): Promise<Project[]> {
  return request<Project[]>(BASE_PATH)
}

export function getProject(id: string): Promise<Project> {
  return request<Project>(`${BASE_PATH}/${encodeURIComponent(id)}`)
}

export function updateProject(id: string, input: UpdateProjectRequest): Promise<Project> {
  return request<Project>(`${BASE_PATH}/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(input),
  })
}

export function deleteProject(id: string): Promise<void> {
  return request<void>(`${BASE_PATH}/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}
