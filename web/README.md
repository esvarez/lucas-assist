# Nudge web

Vite + React SPA for Nudge. Client-rendered only — no SSR (see [`../AGENTS.MD`](../AGENTS.MD), "No SSR, ever").

## Prerequisites

Node 20+.

## Setup

```bash
npm install
npm run dev
```

By default this proxies `/api/*` to `http://localhost:8080` (see `vite.config.ts`), so pair it with one of:

- The real backend: `make local` from the repo root (needs Docker + `OPENAI_API_KEY`, see the [root README](../README.md)).
- The mock backend, for UI work without Go/Docker: `npm run mock` in this directory (`mock-server.mjs`), in a separate terminal.

## Scripts

| Command | Purpose |
| --- | --- |
| `npm run dev` | Vite dev server on :5173. |
| `npm run mock` | In-memory Node mock of the API on :8080 — same routes as `docs/openapi.yaml`, plus a couple that don't exist on the real backend yet (see comments in `mock-server.mjs`). |
| `npm run build` | Type-check (`tsc -b`) then production build to `dist/`. |
| `npm run preview` | Serve the production build locally. |
| `npm run lint` | ESLint. |

## Stack

- [Vite](https://vite.dev/) + React 19 + TypeScript.
- [shadcn/ui](https://ui.shadcn.com/) (Radix + Tailwind CSS v4) for components — added via `npx shadcn add <component>` into `src/components/ui`, not an opaque dependency. See `../architecture.md` §13 and `../AGENTS.MD` "Frontend rules" before adding UI.
- `react-router-dom` for client-side routing.

## Layout

```
src/api        Fetch wrappers for the backend (projects, tasks)
src/routes     Route-level pages (ProjectsPage, ProjectDetailPage)
src/layout     Shared page chrome
src/components Reusable and feature components
src/lib        Small client-side helpers (status derivation, etc.)
```
