// Package api wires the HTTP router and handlers shared between the API
// Lambda (cmd/api) and the local dev server (cmd/local, once it exists).
// Handlers are plain net/http so they run unchanged in both.
package api

import (
	"context"
	"net/http"

	"github.com/esvarez/lucas-assist/internal/domain"
	"github.com/esvarez/lucas-assist/internal/store"
)

// ProjectRepository is the slice of store.Repository the API package
// actually calls. Routes are added one Repository method at a time (see
// AGENTS.MD: adding a CRUD route, not a new Lambda) — this interface grows
// alongside them instead of requiring every route's dependency up front.
// Any store.Repository implementation, complete or partial, satisfies it
// as long as it has CreateProject.
type ProjectRepository interface {
	CreateProject(ctx context.Context, p domain.Project) (domain.Project, error)
	GetProject(ctx context.Context, userID, id string) (domain.Project, error)
	ListProjects(ctx context.Context, userID string) ([]domain.Project, error)
	DeleteProject(ctx context.Context, userID, id string) error
	UpdateProject(ctx context.Context, userID string, p domain.Project) (domain.Project, error)

	// GetChangeset and AcceptChangeset back the changeset-accept route
	// (architecture.md §1/§9, issue #77) — added here rather than a
	// separate interface since this package's convention is one growing
	// interface per Lambda, not one per entity (see doc comment above).
	GetChangeset(ctx context.Context, userID, projectID, changesetID string) (domain.Changeset, error)
	AcceptChangeset(ctx context.Context, p domain.Project, c domain.Changeset, idempotencyKey string) (store.AcceptChangesetResult, error)
}

// NewRouter builds the API's route table against repo.
func NewRouter(repo ProjectRepository) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /projects", createProjectHandler(repo))
	mux.HandleFunc("GET /projects", listProjectsHandler(repo))
	mux.HandleFunc("GET /projects/{id}", getProjectHandler(repo))
	mux.HandleFunc("PUT /projects/{id}", updateProjectHandler(repo))
	mux.HandleFunc("DELETE /projects/{id}", deleteProjectHandler(repo))
	mux.HandleFunc("POST /projects/{id}/changesets/{changesetId}/accept", acceptChangesetHandler(repo))
	return mux
}
