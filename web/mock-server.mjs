// Dev-only stand-in for cmd/local (Go API + DynamoDB Local), for previewing
// the SPA without standing up the real backend. Implements the same 5
// routes documented in docs/openapi.yaml, plus one route (GET
// /projects/:id/tasks) that doesn't exist on the real backend yet — see
// the comment on tasksByProject below. All in-memory, on :8080, the port
// vite.config.ts already proxies /api/* to. Not part of the real backend;
// delete or ignore once `make local` is available.
//
// Run with: node mock-server.mjs

import { createServer } from 'node:http'
import { randomUUID } from 'node:crypto'

const PORT = 8080

/** @type {Map<string, any[]>} projects keyed by user_id */
const projectsByUser = new Map()

// Tasks aren't a real endpoint yet — internal/api/router.go only wires
// /projects routes, and there's no path that actually creates a task
// today (decompose_task only proposes them in memory; #77, the
// changeset-accept endpoint that would persist them, is still open). This
// Map plus the GET /projects/:id/tasks route below exist purely so the
// list view's progress bar has something to render against; delete both
// once the real endpoint lands.
/** @type {Map<string, any[]>} tasks keyed by project id */
const tasksByProject = new Map()

function makeTask(projectId, order, title, status, description = '', parentId = '') {
  return {
    id: randomUUID(),
    project_id: projectId,
    parent_id: parentId,
    title,
    description,
    status,
    order,
    acceptance_criteria: [],
    subtasks: [],
  }
}

function seed(userId) {
  const now = new Date().toISOString()
  const tidepool = {
    user_id: userId,
    id: randomUUID(),
    name: 'Tidepool Sync',
    goal: 'Offline-first note sync between the CLI and the web app.',
    deadline: null,
    constraints: ['Must ship on Postgres', 'No third-party sync service'],
    status: 'on-track',
    created_at: now,
    updated_at: now,
  }
  const fernweg = {
    user_id: userId,
    id: randomUUID(),
    name: 'Fernweg CLI',
    goal: 'A trip-planning CLI for people who hate trip-planning apps.',
    deadline: null,
    constraints: [],
    status: 'at-risk',
    created_at: now,
    updated_at: now,
  }

  projectsByUser.set(userId, [tidepool, fernweg])

  const cli = makeTask(
    tidepool.id,
    3,
    'CLI sync command',
    'in-progress',
    'The storage layer and conflict resolution are done, but the CLI command has no clear next step yet.',
  )
  cli.subtasks.push(
    makeTask(tidepool.id, 0, 'Add a sync command', 'done', '', cli.id),
    makeTask(tidepool.id, 1, 'Handle auth errors', 'in-progress', '', cli.id),
  )

  const indicator = makeTask(tidepool.id, 4, 'Web app sync indicator', 'todo')
  indicator.subtasks.push(makeTask(tidepool.id, 0, 'Show last-synced time', 'todo', '', indicator.id))

  const pdf = makeTask(
    fernweg.id,
    1,
    'Itinerary import from PDF',
    'todo',
    'The trip data model is in place, but PDF import has no clear next step yet.',
  )
  pdf.subtasks.push(
    makeTask(fernweg.id, 0, 'Parse PDF layout', 'todo', '', pdf.id),
    makeTask(fernweg.id, 1, 'Map fields to the trip model', 'blocked', '', pdf.id),
  )

  tasksByProject.set(tidepool.id, [
    makeTask(tidepool.id, 0, 'Design the sync protocol', 'done'),
    makeTask(tidepool.id, 1, 'Local-first storage layer', 'done'),
    makeTask(tidepool.id, 2, 'Conflict resolution for offline edits', 'done'),
    cli,
    indicator,
  ])
  tasksByProject.set(fernweg.id, [
    makeTask(fernweg.id, 0, 'Trip data model', 'done'),
    pdf,
    makeTask(fernweg.id, 2, 'Offline maps cache', 'todo'),
    makeTask(fernweg.id, 3, 'Packing list generator', 'todo'),
  ])
}

function getProjects(userId) {
  if (!projectsByUser.has(userId)) seed(userId)
  return projectsByUser.get(userId)
}

// New projects start with a small, mostly-incomplete task list so the
// progress bar has something to show right after creating one in the demo.
function getTasks(projectId) {
  if (!tasksByProject.has(projectId)) {
    const scaffolding = makeTask(
      projectId,
      1,
      'Set up the initial scaffolding',
      'todo',
      'Scope is defined, but the initial scaffolding has no clear next step yet.',
    )
    scaffolding.subtasks.push(
      makeTask(projectId, 0, 'Create the repo', 'done', '', scaffolding.id),
      makeTask(projectId, 1, 'Add CI', 'todo', '', scaffolding.id),
    )
    tasksByProject.set(projectId, [
      makeTask(projectId, 0, 'Define the project scope', 'done'),
      scaffolding,
      makeTask(projectId, 2, 'Write the first test', 'todo'),
    ])
  }
  return tasksByProject.get(projectId)
}

function sendJSON(res, status, body) {
  const data = body === undefined ? '' : JSON.stringify(body)
  res.writeHead(status, { 'Content-Type': 'application/json' })
  res.end(data)
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    let raw = ''
    req.on('data', (chunk) => (raw += chunk))
    req.on('end', () => {
      if (!raw) return resolve({})
      try {
        resolve(JSON.parse(raw))
      } catch (err) {
        reject(err)
      }
    })
    req.on('error', reject)
  })
}

const server = createServer(async (req, res) => {
  const url = new URL(req.url, `http://localhost:${PORT}`)
  const userId = url.searchParams.get('user_id') ?? ''
  const parts = url.pathname.split('/').filter(Boolean) // ["projects", ":id"?]

  if (parts[0] !== 'projects') {
    return sendJSON(res, 404, { error: 'not found' })
  }

  try {
    // POST /projects
    if (req.method === 'POST' && parts.length === 1) {
      const body = await readBody(req)
      const bodyUserId = body.user_id ?? ''
      if (!bodyUserId) {
        return sendJSON(res, 400, {
          error: 'validation failed',
          fields: { user_id: 'user_id is required' },
        })
      }
      const now = new Date().toISOString()
      const project = {
        user_id: bodyUserId,
        id: randomUUID(),
        name: body.name ?? '',
        goal: body.goal ?? '',
        deadline: body.deadline ?? null,
        constraints: body.constraints ?? [],
        status: body.status ?? '',
        created_at: now,
        updated_at: now,
      }
      getProjects(bodyUserId).unshift(project)
      return sendJSON(res, 201, project)
    }

    // GET /projects
    if (req.method === 'GET' && parts.length === 1) {
      return sendJSON(res, 200, getProjects(userId))
    }

    // /projects/:id — GET and DELETE take user_id as a query param, PUT
    // takes it in the body, exactly like updateProjectHandler /
    // getProjectHandler / deleteProjectHandler in internal/api.
    if (parts.length === 2) {
      const id = decodeURIComponent(parts[1])

      if (req.method === 'GET') {
        const list = getProjects(userId)
        const index = list.findIndex((p) => p.id === id)
        if (index === -1) return sendJSON(res, 404, { error: 'not found' })
        return sendJSON(res, 200, list[index])
      }

      if (req.method === 'PUT') {
        const body = await readBody(req)
        const list = getProjects(body.user_id ?? '')
        const index = list.findIndex((p) => p.id === id)
        if (index === -1) return sendJSON(res, 404, { error: 'not found' })
        const updated = {
          ...list[index],
          goal: body.goal ?? list[index].goal,
          deadline: body.deadline ?? list[index].deadline,
          constraints: body.constraints ?? list[index].constraints,
          status: body.status ?? list[index].status,
          updated_at: new Date().toISOString(),
        }
        list[index] = updated
        return sendJSON(res, 200, updated)
      }

      if (req.method === 'DELETE') {
        const list = getProjects(userId)
        const index = list.findIndex((p) => p.id === id)
        if (index === -1) return sendJSON(res, 404, { error: 'not found' })
        list.splice(index, 1)
        res.writeHead(204)
        return res.end()
      }
    }

    // GET /projects/:id/tasks — mock-only, see the comment on
    // tasksByProject above.
    if (req.method === 'GET' && parts.length === 3 && parts[2] === 'tasks') {
      const id = decodeURIComponent(parts[1])
      const list = getProjects(userId)
      if (!list.some((p) => p.id === id)) {
        return sendJSON(res, 404, { error: 'not found' })
      }
      return sendJSON(res, 200, getTasks(id))
    }

    sendJSON(res, 404, { error: 'not found' })
  } catch (err) {
    sendJSON(res, 500, { error: err instanceof Error ? err.message : 'internal error' })
  }
})

server.listen(PORT, () => {
  console.log(`mock API listening on http://localhost:${PORT} (routes: docs/openapi.yaml)`)
})
