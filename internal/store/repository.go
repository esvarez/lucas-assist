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

// ErrChangesetNotProposed is returned by AcceptChangeset when the
// changeset's status isn't "proposed" — already applied, rejected,
// expired, or in conflict. Accepting is only ever valid from "proposed".
var ErrChangesetNotProposed = errors.New("changeset is not in the proposed state")

// ErrChangesetTooLarge is returned by AcceptChangeset when a changeset's
// mutation count exceeds the 50-mutation atomic-commit limit
// (architecture.md §8, ADR 009). Splitting an oversized proposal into
// multiple independently reviewable changesets is the worker's job (#76);
// this is the backstop that refuses to apply one anyway.
var ErrChangesetTooLarge = errors.New("changeset exceeds the maximum mutation count")

// ErrIdempotencyKeyMismatch is returned by AcceptChangeset when an
// idempotency key was already used for a different (project, changeset)
// pair (architecture.md §15: "Reusing a key with a different request is
// rejected").
var ErrIdempotencyKeyMismatch = errors.New("idempotency key already used for a different request")

// maxChangesetMutations is the atomic-commit limit (architecture.md §8,
// ADR 009): a proposed changeset's task count above this is rejected
// rather than applied, keeping one accept transaction comfortably below
// TransactWriteItems' 100-item cap once the project version update, the
// changeset status update, the event, and the idempotency record are
// counted alongside the task writes.
const maxChangesetMutations = 50

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
	// Attempt/WorkerID/LeaseUntil/Error, regardless of what the caller
	// passed in r — like Version on CreateProject, these are repo-owned on
	// creation, not client-set.
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

	// CompleteAgentRun and FailAgentRun set the run's terminal state. Like
	// UpdateChangesetStatus, these are unconditional status sets once the
	// run exists — enforcing that a run was actually leased before it's
	// completed or failed belongs to the worker, not this primitive.
	CompleteAgentRun(ctx context.Context, userID, projectID, runID string) (domain.AgentRun, error)
	FailAgentRun(ctx context.Context, userID, projectID, runID, errMsg string) (domain.AgentRun, error)

	// AcceptChangeset atomically commits a proposed changeset: creates its
	// ProposedTasks as real Task rows, bumps the project's version, appends
	// one audit Event, and records the idempotency key — all in a single
	// transaction (architecture.md §1/§8/§9/§15). It loads the changeset
	// itself (by in.ChangesetID) rather than taking it pre-fetched, so it
	// can validate status and mutation count against the same read it
	// commits against.
	//
	// A stale BaseVersion (checked against the project's current version)
	// returns ErrConflict and separately moves the changeset to
	// ChangesetConflict — never retried automatically. A changeset not in
	// ChangesetProposed returns ErrChangesetNotProposed. One over
	// maxChangesetMutations returns ErrChangesetTooLarge without attempting
	// the write. Replaying the same IdempotencyKey with the same RequestHash
	// returns the original result instead of re-applying; a different
	// RequestHash under the same key returns ErrIdempotencyKeyMismatch.
	AcceptChangeset(ctx context.Context, in AcceptChangesetInput) (AcceptChangesetResult, error)
}

// AcceptChangesetInput is what AcceptChangeset needs beyond the changeset's
// own stored data (BaseVersion, ProposedTasks, Status) to commit it.
type AcceptChangesetInput struct {
	UserID         string
	ProjectID      string
	ChangesetID    string
	IdempotencyKey string
	// RequestHash identifies the semantic request this IdempotencyKey was
	// issued for — here, simply a hash of (UserID, ProjectID, ChangesetID),
	// since those three fully determine what accepting does. Computed by
	// the caller (internal/api) so this package stays agnostic to how a
	// "request" is hashed.
	RequestHash string
}

// AcceptChangesetResult is the outcome of a successful accept — or a
// successful idempotent replay of one.
type AcceptChangesetResult struct {
	Project domain.Project `json:"project"`
	Tasks   []domain.Task  `json:"tasks"`
}
