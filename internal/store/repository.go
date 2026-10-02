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
	UpdateProject(ctx context.Context, userID string, p domain.Project) (domain.Project, error)

	CreateTask(ctx context.Context, userID string, t domain.Task) (domain.Task, error)
	GetTask(ctx context.Context, userID, projectID, taskID string) (domain.Task, error)
	ListTasks(ctx context.Context, userID, projectID string) ([]domain.Task, error)
	UpdateTaskStatus(ctx context.Context, userID, projectID, taskID, status string) (domain.Task, error)

	CreateChangeset(ctx context.Context, c domain.Changeset) (domain.Changeset, error)
	GetChangeset(ctx context.Context, userID, projectID, changesetID string) (domain.Changeset, error)
	ListChangesets(ctx context.Context, userID, projectID string) ([]domain.Changeset, error)
	UpdateChangesetStatus(ctx context.Context, userID, projectID, changesetID string, status domain.ChangesetStatus) (domain.Changeset, error)
	UpdateChangesetProposedTasks(ctx context.Context, userID, projectID, changesetID string, tasks []domain.ProposedTask) (domain.Changeset, error)
	
	CreateAgentRun(ctx context.Context, r domain.AgentRun) (domain.AgentRun, error)
	GetAgentRun(ctx context.Context, userID, projectID, runID string) (domain.AgentRun, error)
	ListAgentRuns(ctx context.Context, userID, projectID string, statuses []domain.AgentRunStatus) ([]domain.AgentRun, error)
	LeaseAgentRun(ctx context.Context, userID, projectID, runID, workerID string, leaseUntil time.Time) (domain.AgentRun, error)
	CompleteAgentRun(ctx context.Context, userID, projectID, runID, changesetID string) (domain.AgentRun, error)
	FailAgentRun(ctx context.Context, userID, projectID, runID, errMsg string) (domain.AgentRun, error)
	NeedsInputAgentRun(ctx context.Context, userID, projectID, runID string, questions []string) (domain.AgentRun, error)
	// CancelAgentRun sets a run's status to cancelled unconditionally, the
	// same unconditional-status-set pattern CompleteAgentRun/FailAgentRun
	// use (#219) — called when a needs_input run's questions are answered
	// via a fresh dispatch, so the run that asked stops looking pending
	// (e.g. to a later ListAgentRuns(statuses=[...needs_input...]) scan)
	// once it's actually been superseded.
	CancelAgentRun(ctx context.Context, userID, projectID, runID string) (domain.AgentRun, error)
	AcceptChangeset(ctx context.Context, p domain.Project, c domain.Changeset, remainingProposedTasks []domain.ProposedTask, requestFingerprint, idempotencyKey string) (AcceptChangesetResult, error)
	AcceptCreateProjectChangeset(ctx context.Context, c domain.Changeset, idempotencyKey string) (AcceptCreateProjectResult, error)
}

// AcceptChangesetResult is the outcome of a successful AcceptChangeset
// call: the project at its new version, the tasks created from the
// changeset's proposal, the audit event written alongside them, and the
// changeset itself as it now stands — still "proposed" with whatever
// wasn't accepted this call (#169), or "applied" once nothing is left.
// Callers use Changeset to keep their own copy of the remaining proposal
// in sync without a separate GetChangeset round-trip.
type AcceptChangesetResult struct {
	Project   domain.Project   `json:"project"`
	Tasks     []domain.Task    `json:"tasks"`
	Event     domain.Event     `json:"event"`
	Changeset domain.Changeset `json:"changeset"`
}

// AcceptCreateProjectResult is the outcome of a successful
// AcceptCreateProjectChangeset call: the newly created project.
type AcceptCreateProjectResult struct {
	Project domain.Project `json:"project"`
}
