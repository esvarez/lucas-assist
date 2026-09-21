# Nudge

An assistant that helps indie developers finish projects: breaking large work into small tasks, preserving context, and recommending a concrete next action.

Go backend on AWS Lambda + DynamoDB + SQS, calling OpenAI's Responses API. Static React (Vite) SPA served from S3 + CloudFront. Every model-proposed change is a reviewable `Changeset` — the LLM never writes to the datastore directly.

**Status:** pre-implementation / early MVP. Project CRUD and one skill (`decompose_task`, `create_project`) exist; auth, async agent jobs, and changeset acceptance are still being built. See the full decision log in [`architecture.md`](architecture.md) and the product brief in [`PRODUCT.md`](PRODUCT.md).

## Prerequisites

- Go 1.27+ (see `go.mod`)
- Node 20+
- Docker (for DynamoDB Local)
- An OpenAI API key
- AWS CLI + [SAM CLI](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/install-sam-cli.html), only if you're deploying

## Setup

```bash
go mod download
docker compose up -d          # DynamoDB Local on :8000
make local                    # Go API on :8080
```

```bash
cd web
npm install
npm run dev                   # SPA on :5173, proxies /api/* to :8080
```

`make local` needs an OpenAI key to serve `/skills`:

```bash
export OPENAI_API_KEY=sk-...
make local
```

Other env vars `make local` reads (all optional, shown with their defaults):

| Variable | Default | Purpose |
| --- | --- | --- |
| `OPENAI_API_KEY` | _(empty)_ | Required for skill calls; read directly from the environment only in local dev. |
| `DYNAMODB_ENDPOINT` | `http://localhost:8000` | Points at DynamoDB Local. |
| `DYNAMODB_TABLE` | `nudge-local` | Created automatically on startup if missing. |

### Frontend without the Go backend

`web` also ships a Node mock server that implements the same routes as the real API, for UI work without Go/Docker running:

```bash
cd web
npm run mock                  # mock API on :8080
npm run dev                   # in a second terminal
```

## Testing

```bash
make test               # unit tests, no AWS calls
make test-integration   # requires DynamoDB Local (docker compose up -d)
```

```bash
cd web && npm run lint
```

## API

The API Lambda currently exposes project CRUD (`/projects`) plus `POST /skills` for model-backed proposals. See [`docs/openapi.yaml`](docs/openapi.yaml) for the full spec.

## Deploying

Built and deployed with AWS SAM (`template.yaml`):

```bash
make build
sam deploy --guided
```

Before the first deploy to a given environment, put the OpenAI key in SSM (read at runtime by the Skill Lambda, never passed through CloudFormation):

```bash
aws ssm put-parameter \
  --name /<environment>/nudge/openai-api-key \
  --type SecureString \
  --value <key>
```

Once the stack exists, build and publish the SPA to the bucket CloudFront serves (`sam deploy` provisions the bucket/distribution; it doesn't upload `web/dist`'s contents):

```bash
make deploy-web STACK_NAME=<your-sam-stack-name>
```

## Repo layout

```
cmd/api               API Lambda entrypoint
cmd/skills            Skill Lambda entrypoint (decompose_task, create_project, ...)
cmd/local             Local http.Server wrapping the same handlers, for dev
internal/domain       Project, Task, Decision, Event, Note, Changeset
internal/store        Repository interface + DynamoDB and in-memory implementations
internal/agent        Skill registry and context assembly
internal/agent/skills One file per skill
internal/llm          OpenAI client wrapper + strict-schema helpers
internal/api          HTTP router and handlers
internal/skillsapi    HTTP adapter for the skill registry
internal/queue        SQS client wrapper
web                   Vite + React SPA (shadcn/ui components)
docs                  openapi.yaml
template.yaml         AWS SAM infrastructure definition
```

## Further reading

- [`architecture.md`](architecture.md) — architecture decisions, ADRs, domain model, open questions.
- [`AGENTS.MD`](AGENTS.MD) — rules and conventions for AI coding agents working in this repo (also a good orientation doc for humans).
- [`PRODUCT.md`](PRODUCT.md) — one-page product brief.
- [`docs/openapi.yaml`](docs/openapi.yaml) — API spec.
