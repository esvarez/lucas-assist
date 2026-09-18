// Dev-only stand-in for cmd/local (Go API + DynamoDB Local), for previewing
// the SPA without standing up the real backend. Implements the same 5
// routes documented in docs/openapi.yaml, in-memory, on :8080 — the port
// vite.config.ts already proxies /api/* to. Not part of the real backend;
// delete or ignore once `make local` is available.
//
// Run with: node mock-server.mjs

import { createServer } from 'node:http'
import { randomUUID } from 'node:crypto'

const PORT = 8080

/** @type {Map<string, any[]>} projects keyed by user_id */
const projectsByUser = new Map()

function seed(userId) {
  const now = new Date().toISOString()
  projectsByUser.set(userId, [
    {
      user_id: userId,
      id: randomUUID(),
      name: 'Tidepool Sync',
      goal: 'Offline-first note sync between the CLI and the web app.',
      deadline: null,
      constraints: ['Must ship on Postgres', 'No third-party sync service'],
      status: 'on-track',
      created_at: now,
      updated_at: now,
    },
    {
      user_id: userId,
      id: randomUUID(),
      name: 'Fernweg CLI',
      goal: 'A trip-planning CLI for people who hate trip-planning apps.',
      deadline: null,
      constraints: [],
      status: 'at-risk',
      created_at: now,
      updated_at: now,
    },
  ])
}

function getProjects(userId) {
  if (!projectsByUser.has(userId)) seed(userId)
  return projectsByUser.get(userId)
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

    sendJSON(res, 404, { error: 'not found' })
  } catch (err) {
    sendJSON(res, 500, { error: err instanceof Error ? err.message : 'internal error' })
  }
})

server.listen(PORT, () => {
  console.log(`mock API listening on http://localhost:${PORT} (routes: docs/openapi.yaml)`)
})
