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
// GetProject and ListProjects take a userID because the DynamoDB key schema
// (architecture.md §7) partitions by user: PK=USER#<uid>. DeleteProject and
// UpdateProject still key off ID alone — they're not yet implemented against
// DynamoDB (see #11, #12) and will pick up the same treatment then.
type Repository interface {
	CreateProject(ctx context.Context, p domain.Project) (domain.Project, error)
	GetProject(ctx context.Context, userID, id string) (domain.Project, error)
	ListProjects(ctx context.Context, userID string) ([]domain.Project, error)
	DeleteProject(ctx context.Context, id string) error
	UpdateProject(ctx context.Context, p domain.Project) (domain.Project, error)
}
