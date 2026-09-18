// Typed client for the Projects CRUD routes (docs/openapi.yaml), served by
// cmd/api / cmd/local. Requests go to /api/projects* — the dev server
// proxies that to the local API (vite.config.ts) and production is
// expected to route it the same way (architecture.md §13).
//
// There's no auth yet: user_id is caller-supplied (docs/openapi.yaml's
// intro note, architecture.md "Still undecided"). It lives in localStorage
// and every function here attaches it itself — callers never pass it —
// so swapping this for a real JWT later only touches this file.

const BASE_PATH = '/api/projects'
const USER_ID_STORAGE_KEY = 'nudge:user_id'

export function getUserId(): string {
  return localStorage.getItem(USER_ID_STORAGE_KEY) ?? ''
}

export function setUserId(userId: string): void {
  localStorage.setItem(USER_ID_STORAGE_KEY, userId)
}

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
  // Optimistic-concurrency version (architecture.md §8). Pass the value
  // from the project you loaded back into updateProject — a stale value
  // is rejected with a 409 (ApiError), not silently overwritten.
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

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
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

function userIdParam(): string {
  return `user_id=${encodeURIComponent(getUserId())}`
}

export function createProject(input: CreateProjectRequest): Promise<Project> {
  return request<Project>(BASE_PATH, {
    method: 'POST',
    body: JSON.stringify({ ...input, user_id: getUserId() }),
  })
}

export function listProjects(): Promise<Project[]> {
  return request<Project[]>(`${BASE_PATH}?${userIdParam()}`)
}

export function getProject(id: string): Promise<Project> {
  return request<Project>(`${BASE_PATH}/${encodeURIComponent(id)}?${userIdParam()}`)
}

export function updateProject(id: string, input: UpdateProjectRequest): Promise<Project> {
  return request<Project>(`${BASE_PATH}/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify({ ...input, user_id: getUserId() }),
  })
}

export function deleteProject(id: string): Promise<void> {
  return request<void>(`${BASE_PATH}/${encodeURIComponent(id)}?${userIdParam()}`, {
    method: 'DELETE',
  })
}
