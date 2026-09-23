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

	// GetAgentRun and GetChangeset back GET /agent-runs/{id} — see
	// getAgentRunHandler's doc comment for why the route also needs
	// GetChangeset, not just GetAgentRun.
	GetAgentRun(ctx context.Context, userID, projectID, runID string) (domain.AgentRun, error)
	GetChangeset(ctx context.Context, userID, projectID, changesetID string) (domain.Changeset, error)

	// AcceptChangeset backs the changeset-accept route (architecture.md
	// §1/§9, issue #77) — added here rather than a separate interface
	// since this package's convention is one growing interface per Lambda,
	// not one per entity (see doc comment above).
	AcceptChangeset(ctx context.Context, p domain.Project, c domain.Changeset, idempotencyKey string) (store.AcceptChangesetResult, error)

	// AcceptCreateProjectChangeset backs the project-less changeset-accept
	// route (#152) — a create_project changeset has no existing project to
	// address in a URL path, so it's a separate route from AcceptChangeset
	// above rather than a variant of it.
	AcceptCreateProjectChangeset(ctx context.Context, c domain.Changeset, idempotencyKey string) (store.AcceptCreateProjectResult, error)

	// ListTasks backs GET /projects/{id}/tasks (#125) — the real
	// replacement for what web/mock-server.mjs has been standing in for.
	ListTasks(ctx context.Context, userID, projectID string) ([]domain.Task, error)
}

// NewRouter builds the API's route table against repo.
func NewRouter(repo ProjectRepository) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /projects", createProjectHandler(repo))
	mux.HandleFunc("GET /projects", listProjectsHandler(repo))
	mux.HandleFunc("GET /projects/{id}", getProjectHandler(repo))
	mux.HandleFunc("PUT /projects/{id}", updateProjectHandler(repo))
	mux.HandleFunc("DELETE /projects/{id}", deleteProjectHandler(repo))
	mux.HandleFunc("GET /agent-runs/{id}", getAgentRunHandler(repo))
	mux.HandleFunc("POST /projects/{id}/changesets/{changesetId}/accept", acceptChangesetHandler(repo))
	mux.HandleFunc("POST /changesets/{changesetId}/accept", acceptCreateProjectChangesetHandler(repo))
	mux.HandleFunc("GET /projects/{id}/tasks", listTasksHandler(repo))
	return mux
}
