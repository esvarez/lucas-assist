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

## Auth rules

GitHub OAuth is the **only** identity provider. Nudge issues its own JWTs. See `docs/architecture-decisions.md` §15.

- **Web:** authorization code + PKCE. **CLI:** device flow (RFC 8628). The CLI is first-class — don't build web-only auth paths.
- **Access token:** self-issued JWT, HS256, 15min. **Refresh token:** opaque, stored as `sha256` hash in DynamoDB with a TTL attribute. Never store a refresh token in plaintext.
- Both clients send `Authorization: Bearer <access_token>`. One middleware path. Web keeps the refresh token in an httpOnly/Secure/SameSite=Strict cookie; the CLI in `~/.nudge/credentials` mode 0600.
- **Never send the GitHub access token to the client.** It stays server-side on the user record.
- **The internal user ID is a generated UUID, not the GitHub user ID.** Login resolves GitHub ID → UUID via a `GetItem` on the `IDENTITY#github#<gh_id>` item. Don't add a GSI for this and don't use the provider ID as the partition key — that would make a second provider a full re-key.
- Validation is Go middleware, not an API Gateway JWT authorizer.

## DynamoDB rules

Single table, **partitioned by user**. The project ID lives in the sort key prefix.

| Item | PK | SK |
|---|---|---|
| Project card | `USER#<uid>` | `META#<pid>` |
| Task | `USER#<uid>` | `P#<pid>#TASK#<uuid>` |
| Decision | `USER#<uid>` | `P#<pid>#DEC#<ts>` |
| Event | `USER#<uid>` | `P#<pid>#EVT#<ts>` |
| Pending changeset | `USER#<uid>` | `P#<pid>#PENDING#<job_id>` |
| User profile | `USER#<uid>` | `PROFILE` |
| Session | `USER#<uid>` | `SESSION#<sha256(refresh)>` |
| Identity lookup | `IDENTITY#github#<gh_id>` | `IDENTITY` |

GSI2 (sparse): `PK = USER#<uid>`, `SK = <priority>#<updated_at>`. There is no GSI1.

### The rule that matters most

**Build the PK from the verified JWT claim only.** Never from a path parameter, query string, or request body.

Authorization in this system is structural: a query is physically scoped to the caller's partition, so a handler that forgets an ownership check still cannot leak data. That property only holds if the user ID comes from the token.

- Resolve the user ID once in auth middleware, put it in `context.Context`, read it from there.
- **Never** add a route shaped like `/api/users/:uid/...`. Project-scoped routes are `/api/projects/:pid/...` and the user ID comes from the token, not the path.
- If you find yourself writing a function that takes `userID string` from an HTTP request, stop.

### Single-user projects

Every project has exactly one owner. **Shared projects are out of scope.**

- No `MEMBER#` items, no `assignee_id`, no roles or permission model. Ownership is the only access concept.
- Don't add fields or abstractions "for when we support teams." That feature is not planned.

### Optimistic concurrency is still required

Single-user is not single-session — two browser tabs hit the same conflict a second user would, because a changeset is proposed against one read and committed in a later round trip.

- The project `META#<pid>` item carries a `version` integer, incremented on every commit.
- A proposed changeset records the version it was built against.
- Every commit transaction includes `ConditionExpression: version = :expected`.
- On `ConditionalCheckFailedException`, return a conflict to the client so it can re-propose. **Do not retry the write.**

### Other rules

- **GSIs only. Never add an LSI.** LSIs cannot be altered after table creation. (This also keeps the 10GB item-collection cap from applying, which is what lets user partitions grow freely.)
- **GSI2 is sparse on purpose.** Write its key attributes only when a task is actionable (unblocked, not done). Marking a task done or blocked means *deleting* those attributes so it drops out of the index.
- **Don't reintroduce a project-partitioned index.** Everything is reachable from the user partition; a `PROJECT#<id>` PK would recreate the authorization hole this schema was designed to close.
- The task tree is `parent_id` attributes assembled in Go after a single Query. **Do not encode hierarchy into the sort key** — the `P#<pid>#` prefix scopes to a project, nothing more. Re-parenting must stay a single `UpdateItem`.
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

## Out of scope for the MVP

Do not build these. If a task seems to need one, stop and ask.

- **Shared or multi-user projects.** Every project has exactly one owner. No members, no roles, no invites, no sharing UI, no `MEMBER#` items. The key schema is user-partitioned specifically because of this; adding collaboration later means re-keying, and that decision hasn't been made.
- **Real-time collaboration or push updates.** No WebSockets, no subscriptions.
- **Assignment.** Tasks have no assignee field. "What should I do next?" means *the owner*.

## Stop and ask before

- Adding a new Lambda function
- Adding a GSI
- Adding a dependency
- Introducing SSR, a new datastore, or a background service
- Anything that puts a Lambda in a VPC
- Anything that lets model output reach the datastore without a user accept
- Any route or function that takes a user ID from the request rather than the token

## Still undecided

Don't assume answers to these — ask.

- **What `priority` means** — GSI2's sort key depends on it and it is undefined. Do not invent a scheme.
- Secrets storage: SSM Parameter Store vs Secrets Manager
- Optimistic concurrency (`version` attribute on project META)
- Whether the sync path persists pending changesets or keeps them in client state
- Idempotency key derivation for chunked commits
- Project deletion strategy (exceeds the 100-item transaction cap)
- SAM vs Terraform for `/infra`

Resolved: auth is GitHub OAuth with self-issued JWTs, and the CLI **is** first-class.
