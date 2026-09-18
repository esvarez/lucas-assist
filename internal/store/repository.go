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

	// CreateTask, GetTask, and ListTasks all take userID explicitly, same
	// reason as the Project methods above — the DynamoDB key schema
	// (architecture.md §8) partitions by user, and unlike domain.Project,
	// domain.Task carries no UserID field of its own (only ProjectID).
	CreateTask(ctx context.Context, userID string, t domain.Task) (domain.Task, error)
	GetTask(ctx context.Context, userID, projectID, taskID string) (domain.Task, error)
	ListTasks(ctx context.Context, userID, projectID string) ([]domain.Task, error)
}
