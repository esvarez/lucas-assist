// Package api wires the HTTP router and handlers shared between the API
// Lambda (cmd/api) and the local dev server (cmd/local, once it exists).
// Handlers are plain net/http so they run unchanged in both.
package api

import (
	"context"
	"net/http"

	"github.com/esvarez/lucas-assist/internal/domain"
)

// ProjectRepository is the slice of store.Repository the API package
// actually calls. Routes are added one Repository method at a time (see
// AGENTS.MD: adding a CRUD route, not a new Lambda) — this interface grows
// alongside them instead of requiring every route's dependency up front.
// Any store.Repository implementation, complete or partial, satisfies it
// as long as it has CreateProject.
type ProjectRepository interface {
	CreateProject(ctx context.Context, p domain.Project) (domain.Project, error)
	DeleteProject(ctx context.Context, userID, id string) error
	UpdateProject(ctx context.Context, userID string, p domain.Project) (domain.Project, error)
}

// NewRouter builds the API's route table against repo.
func NewRouter(repo ProjectRepository) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /projects", createProjectHandler(repo))
	mux.HandleFunc("PUT /projects/{id}", updateProjectHandler(repo))
	mux.HandleFunc("DELETE /projects/{id}", deleteProjectHandler(repo))
	return mux
}
