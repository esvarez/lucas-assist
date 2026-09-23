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

## Environment variables

Set these in a git-ignored `.env.local` in this directory (Vite loads it automatically):

| Variable | Purpose |
| --- | --- |
| `VITE_COGNITO_USER_POOL_ID` | Cognito User Pool ID that sign-up/sign-in call directly (ADR 017) — value is the deployed stack's `UserPoolId` output. |
| `VITE_COGNITO_CLIENT_ID` | The pool's public app client ID — value is the deployed stack's `UserPoolClientId` output. |
| `VITE_API_PROXY_TARGET` | Overrides the dev-server proxy's target (default `http://localhost:8080`) — e.g. a deployed API Gateway URL. |
| `VITE_API_STAGE_PATH` | Prefix the proxy rewrites `/api` to before forwarding (default `""`) — e.g. `/dev` for a deployed stage. |

Fetch the Cognito/API values for a given stack with `aws cloudformation describe-stacks --stack-name <stack> --query "Stacks[0].Outputs"`.

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
- `amazon-cognito-identity-js` calls the Cognito User Pool's native API (SRP) directly — no Hosted UI (ADR 017). See `src/lib/cognito.ts`.

## Layout

```
src/api        Fetch wrappers for the backend (projects, tasks)
src/routes     Route-level pages (ProjectsPage, ProjectDetailPage)
src/layout     Shared page chrome
src/components Reusable and feature components
src/lib        Small client-side helpers (status derivation, etc.)
```
