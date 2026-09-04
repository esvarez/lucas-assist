# AGENTS.md

Guidance for AI coding agents working in this repository.

## What this is

**Nudge** — an assistant that helps indie developers ship. It breaks big tasks into small ones, maintains project context, and answers "where are we?" and "what should I do next?".

Go backend on AWS Lambda, DynamoDB, OpenAI. Static React SPA on S3 + CloudFront.

Full rationale for every decision below lives in `docs/architecture-decisions.md`. Read it before proposing structural changes.

## Setup

```bash
go mod download
make build          # cross-compiles all Lambda binaries
make test           # unit tests, no AWS calls
make test-integration   # requires DynamoDB Local on :8000
make local          # runs the API as a normal http.Server on :8080
cd web && npm install && npm run dev
```

Requires Go 1.22+, Node 20+, Docker (for DynamoDB Local).

## Repo layout

```
/cmd/api              API Lambda entrypoint (lambda.Start)
/cmd/skills           LLM Lambda entrypoints
/cmd/worker           Step Functions task handlers
/cmd/local            Local http.Server for development
/internal/domain      Project, Task, Decision, Event, Note, Changeset
/internal/store       Repository interface + DynamoDB and in-memory impls
/internal/agent       Context assembler, skill router, changeset applier
/internal/agent/skills  One file per skill
/internal/llm         OpenAI client wrapper + schema helpers
/web                  Vite + React SPA
/infra                SAM or Terraform templates
/docs                 architecture-decisions.md
```

Put new code in an existing package before creating a new one.

---

## Architecture rules

These encode decisions that were made deliberately. **Do not violate them without asking.** If a task seems to require breaking one, stop and raise it.

### 1. The LLM never writes to the datastore

Skills return a `Changeset`. Nothing is persisted until a separate, explicit user-accept request hits the API Lambda. There is no code path where an OpenAI response leads directly to a `PutItem`.

If you are writing a skill and find yourself importing `store`, you are writing it wrong — skills *read* through the context assembler and *return* proposals.

### 2. Facts come from code, language comes from the model

- `project_status` computes counts, stale tasks, and blocked tasks in Go, then asks the model to narrate the computed summary.
- `next_action` computes the candidate set deterministically, then asks the model to rank and explain. A validation layer rejects any task ID the model returns that isn't in the candidate set.

Never ask the model to count, aggregate, or invent identifiers.

### 3. Lambda stays outside the VPC

DynamoDB over its public endpoint with IAM auth; OpenAI over the public internet. **Never add VPC configuration to a Lambda** — it silently removes internet access and requires a NAT Gateway (~$32/month). Cost control is a primary constraint on this project.

### 4. Split by resource profile, not by route

| Function | Memory | Timeout | Contains |
|---|---|---|---|
| API Lambda | 512MB | 5s | All CRUD, changeset commit, dozens of routes |
| Skill Lambdas | 1GB | 60s | Anything that calls OpenAI |
| Worker Lambdas | 1GB | long | Step Functions tasks |

Adding a CRUD endpoint means adding a route to the existing API Lambda, **not** creating a new function. Only create a new Lambda if its memory/timeout profile genuinely differs.

### 5. No SSR, ever

The SPA is client-rendered and served as static files. Do not introduce Next.js, server components, Lambda@Edge rendering, or any per-request compute for the UI.

---

## Go conventions

- Router is `gorilla/mux`, adapted for Lambda via `awslabs/aws-lambda-go-api-proxy/gorillamux`. Handlers are plain `net/http` — they must run unchanged under `cmd/local`.
- Build for `provided.al2023` on **arm64** (Graviton is cheaper). The binary must be named `bootstrap`:
  ```bash
  GOOS=linux GOARCH=arm64 go build -tags lambda.norpc -o bootstrap ./cmd/api
  ```
- Clients (DynamoDB, OpenAI, skill registry) are constructed once at package init and reused across invocations. Never construct them inside a handler.
- Errors wrap with `fmt.Errorf("...: %w", err)`. No naked returns of `err` from more than one layer down.
- All exported types in `/internal/domain` are plain structs with JSON tags. No ORM-style behavior on domain types.

## DynamoDB rules

Single table. Access is always scoped to a project.

| | PK | SK |
|---|---|---|
| Base | `PROJECT#<id>` | `META` / `TASK#<uuid>` / `DECISION#<ts>` / `EVENT#<ts>` |
| GSI1 | `USER#<id>` | `<updated_at>` |
| GSI2 (sparse) | `PROJECT#<id>` | `<priority>#<updated_at>` |

- **GSIs only. Never add an LSI.** LSIs cannot be altered after table creation.
- **GSI2 is sparse on purpose.** Write its key attributes only when a task is actionable (unblocked, not done). Marking a task done or blocked means *deleting* those attributes so it drops out of the index.
- The task tree is `parent_id` attributes assembled in Go after a single Query. **Do not encode hierarchy into the sort key** — re-parenting must stay a single `UpdateItem`.
- `TransactWriteItems` caps at **100 items**. The changeset applier must chunk larger changesets and each chunk must be idempotent.
- Each GSI an item projects into is a separate billed write. Don't add indexes without discussing it.

## OpenAI integration rules

- Structured output via `response_format: { type: "json_schema", strict: true }`. Not tool calls. Tool calls are reserved for context retrieval (`get_task_tree`, `search_decisions`).
- Schemas are generated from Go structs with `invopop/jsonschema`, then passed through the strict-mode helper in `/internal/llm`. That helper sets `additionalProperties: false`, marks every property required, and rewrites optional fields as nullable unions. **Always use it** — raw `invopop` output will be rejected.
- No `anyOf` at the schema root. Discriminated unions are modeled as one object with a `status` field and nullable branches.
- **Prompt ordering is load-bearing.** OpenAI caches prompt prefixes automatically. Stable content first (system prompt, then project card), variable content last (user question, recently changed tasks). Do not reorder prompt assembly without understanding this.
- Context assembly is tiered: project card + active tasks + recent decisions are always injected; everything else is fetched by the model via tools. Do not dump the whole project into the prompt. There is no vector search and none is needed.

## Frontend rules

- Vite + React SPA in `/web`. Assets get hashed filenames; `index.html` is served `no-cache`.
- The API is same-origin at `/api/*` via CloudFront. **Do not add CORS handling or configure a separate API domain** — that would undo the single-distribution setup.
- The proposed changeset lives in client state between the propose and commit round trips. A refresh loses it; that's accepted for the MVP.

## Testing

- Unit tests use the in-memory `store.Repository` implementation. They must not touch AWS.
- Integration tests run against DynamoDB Local.
- LLM calls are stubbed in tests via the `llm.Client` interface. Never call OpenAI in CI.
- New skills need a test asserting the strict-mode schema round-trips through the Go struct.

## Commits and PRs

- Conventional commits: `feat:`, `fix:`, `refactor:`, `docs:`, `chore:`.
- Run `make test lint` before proposing a change.
- Any change to the DynamoDB key schema, Lambda topology, or the propose/commit flow must update `docs/architecture-decisions.md` in the same change.

## Stop and ask before

- Adding a new Lambda function
- Adding a GSI
- Adding a dependency
- Introducing SSR, a new datastore, or a background service
- Anything that puts a Lambda in a VPC
- Anything that lets model output reach the datastore without a user accept

## Still undecided

Don't assume answers to these — ask.

- Auth provider (Cognito vs Clerk vs WorkOS)
- Whether the CLI is a first-class client (gates cookies vs token auth)
- SAM vs Terraform for `/infra`