// Package store defines the persistence boundary and its implementations
// (in-memory for local dev/tests, DynamoDB for deployed environments).
package store

import (
	"context"
	"errors"

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
}
