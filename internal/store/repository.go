// Package store defines the persistence boundary and its implementations
// (in-memory for local dev/tests, DynamoDB for deployed environments).
package store

import (
	"context"
	"errors"
	"time"

	"github.com/esvarez/lucas-assist/internal/domain"
)

// ErrDuplicateID is returned when CreateProject is called with an ID that
// already exists in the store.
var ErrDuplicateID = errors.New("project with this id already exists")

// ErrNotFound is returned when a lookup by ID finds nothing.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when a conditional write's expected version
// doesn't match the item's current version (architecture.md §8's "Project
// version" — a stale baseVersion, not a missing item).
var ErrConflict = errors.New("version conflict")

// ErrRunLeased is returned by LeaseAgentRun when the run isn't eligible to
// be leased: another attempt currently holds a live lease, or the run has
// already reached a terminal status (architecture.md §3/§15).
var ErrRunLeased = errors.New("agent run not eligible for lease")

// ErrIdempotencyKeyReused is returned by AcceptChangeset when the given
// idempotency key was already used for a different request (architecture.md
// §15: "Reusing a key with a different request is rejected").
var ErrIdempotencyKeyReused = errors.New("idempotency key reused with a different request")

// Repository is the persistence interface every skill and the API Lambda
// read and write through.
//
// GetProject, ListProjects, DeleteProject, and UpdateProject all take a
// userID explicitly because the DynamoDB key schema (architecture.md §8)
// partitions by user: PK=USER#<uid>.
type Repository interface {
	CreateProject(ctx context.Context, p domain.Project) (domain.Project, error)
	GetProject(ctx context.Context, userID, id string) (domain.Project, error)
	ListProjects(ctx context.Context, userID string) ([]domain.Project, error)
	DeleteProject(ctx context.Context, userID, id string) error

	// UpdateProject enforces optimistic concurrency on p.Version: it must
	// match the project's current stored version or the update is rejected
	// with ErrConflict instead of applied — never retried automatically
	// (architecture.md §8, AGENTS.MD "do not retry the write"). On success
	// the stored version increments by one.
	UpdateProject(ctx context.Context, userID string, p domain.Project) (domain.Project, error)

	// CreateTask, GetTask, and ListTasks all take userID explicitly, same
	// reason as the Project methods above — the DynamoDB key schema
	// (architecture.md §8) partitions by user, and unlike domain.Project,
	// domain.Task carries no UserID field of its own (only ProjectID).
	CreateTask(ctx context.Context, userID string, t domain.Task) (domain.Task, error)
	GetTask(ctx context.Context, userID, projectID, taskID string) (domain.Task, error)
	ListTasks(ctx context.Context, userID, projectID string) ([]domain.Task, error)

	// CreateChangeset reads UserID/ProjectID off c itself — domain.Changeset
	// carries UserID directly (like domain.Project, unlike domain.Task).
	// GetChangeset and UpdateChangesetStatus still take userID and
	// projectID explicitly since a lookup needs them before it has the
	// changeset in hand. UpdateChangesetStatus is an unconditional status
	// set; enforcing which prior states may transition to which next state
	// belongs to the changeset-accept endpoint, not this primitive.
	CreateChangeset(ctx context.Context, c domain.Changeset) (domain.Changeset, error)
	GetChangeset(ctx context.Context, userID, projectID, changesetID string) (domain.Changeset, error)
	UpdateChangesetStatus(ctx context.Context, userID, projectID, changesetID string, status domain.ChangesetStatus) (domain.Changeset, error)

	// CreateAgentRun, GetAgentRun, LeaseAgentRun, CompleteAgentRun, and
	// FailAgentRun implement the AgentRun lifecycle (architecture.md
	// §3/§10): the API creates a run `queued`, a worker leases it, and the
	// worker marks it `completed` or `failed` once the model call resolves.
	// All five take userID and projectID explicitly, same reason as the
	// Task and Changeset methods above; projectID may be "" since
	// domain.AgentRun.ProjectID is empty for create_project runs.
	//
	// CreateAgentRun always sets Status to AgentRunQueued and resets
	// Attempt/WorkerID/LeaseUntil/Error/Questions, regardless of what the
	// caller passed in r — like Version on CreateProject, these are
	// repo-owned on creation, not client-set.
	CreateAgentRun(ctx context.Context, r domain.AgentRun) (domain.AgentRun, error)
	GetAgentRun(ctx context.Context, userID, projectID, runID string) (domain.AgentRun, error)

	// LeaseAgentRun conditionally moves a run from queued to running, or
	// reclaims a running run whose LeaseUntil has already passed — the
	// worker that held it crashed or timed out mid-attempt (architecture.md
	// §15: SQS delivers at least once, and duplicate delivery must not let
	// two workers process the same run concurrently). It increments
	// Attempt and records workerID/leaseUntil on success, and returns
	// ErrRunLeased if another attempt currently holds a live lease or the
	// run has already reached a terminal status.
	LeaseAgentRun(ctx context.Context, userID, projectID, runID, workerID string, leaseUntil time.Time) (domain.AgentRun, error)

	// CompleteAgentRun, FailAgentRun, and NeedsInputAgentRun set the run's
	// terminal state. Like UpdateChangesetStatus, these are unconditional
	// status sets once the run exists — enforcing that a run was actually
	// leased before it's completed, failed, or needs input belongs to the
	// worker, not this primitive. CompleteAgentRun records changesetID (the
	// Changeset the worker saved for this run) so GET /agent-runs/{id}
	// (#100) can fetch and include it without a second lookup path.
	// NeedsInputAgentRun records questions instead — decompose_task's
	// needs_clarification result (round 0 only) has no Changeset to save.
	CompleteAgentRun(ctx context.Context, userID, projectID, runID, changesetID string) (domain.AgentRun, error)
	FailAgentRun(ctx context.Context, userID, projectID, runID, errMsg string) (domain.AgentRun, error)
	NeedsInputAgentRun(ctx context.Context, userID, projectID, runID string, questions []string) (domain.AgentRun, error)

	// AcceptChangeset atomically commits c's proposed tasks against p:
	// creates each domain.Task, increments the project's version, and
	// appends one domain.Event, all in a single transaction (architecture.md
	// §1/§9's "Accept changeset -> conditional transaction -> applied
	// result"). idempotencyKey deduplicates retries (§15): replaying the
	// same key returns the original result unchanged instead of
	// re-applying it, checked first and ahead of everything below —
	// otherwise a legitimate replay of an already-applied changeset would
	// wrongly fail the "must be proposed" check just below. Reusing the
	// key against a different request is rejected with
	// ErrIdempotencyKeyReused.
	//
	// Past that idempotency check, c must be in "proposed" status —
	// checked here, not by the changeset-accept endpoint, precisely
	// because it has to come after the idempotency check above. A
	// changeset that isn't proposed (already applied/rejected/expired/
	// conflict) returns ErrConflict without touching its status, which may
	// already correctly describe something else.
	//
	// The transaction's authoritative gate past that is the project's
	// stored version matching c.BaseVersion — not whatever p.Version
	// happens to hold when this is called, which the caller may have read
	// slightly before. A mismatch moves c's status to conflict and returns
	// ErrConflict; the caller must not retry automatically (AGENTS.MD).
	//
	// The 50-mutation cap (ADR 009) has no such ordering dependency, so it
	// stays a fast, no-I/O check in the changeset-accept endpoint instead.
	AcceptChangeset(ctx context.Context, p domain.Project, c domain.Changeset, idempotencyKey string) (AcceptChangesetResult, error)

	// AcceptCreateProjectChangeset commits a create_project changeset —
	// c.ProposedProject must be non-nil — into a brand-new Project
	// (architecture.md §1/§9). Unlike AcceptChangeset, there's no existing
	// project to update or version-check against: c.BaseVersion is
	// meaningless for these changesets (domain.Changeset.ProposedProject's
	// doc comment), so this writes a new project META item the same way
	// CreateProject does, plus an EventProjectCreated event and the
	// changeset's status update to "applied", in one transaction.
	//
	// Same idempotency contract as AcceptChangeset (architecture.md §15):
	// replaying idempotencyKey against the same changeset returns the
	// original result; reusing it against a different request is rejected
	// with ErrIdempotencyKeyReused. c must be in "proposed" status, checked
	// after the idempotency lookup for the same reason AcceptChangeset's
	// doc comment gives.
	AcceptCreateProjectChangeset(ctx context.Context, c domain.Changeset, idempotencyKey string) (AcceptCreateProjectResult, error)
}

// AcceptChangesetResult is the outcome of a successful AcceptChangeset
// call: the project at its new version, the tasks created from the
// changeset's proposal, and the audit event written alongside them.
type AcceptChangesetResult struct {
	Project domain.Project `json:"project"`
	Tasks   []domain.Task  `json:"tasks"`
	Event   domain.Event   `json:"event"`
}

// AcceptCreateProjectResult is the outcome of a successful
// AcceptCreateProjectChangeset call: the newly created project.
type AcceptCreateProjectResult struct {
	Project domain.Project `json:"project"`
}
