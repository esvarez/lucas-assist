package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/esvarez/lucas-assist/internal/auth"
	"github.com/esvarez/lucas-assist/internal/domain"
	"github.com/esvarez/lucas-assist/internal/store"
)

type createProjectRequest struct {
	Name        string     `json:"name" validate:"required"`
	Goal        string     `json:"goal" validate:"required"`
	Deadline    *time.Time `json:"deadline,omitempty"`
	Constraints []string   `json:"constraints"`
	Status      string     `json:"status"`
}

func createProjectHandler(repo ProjectRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createProjectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
			return
		}

		if !validateStruct(w, req) {
			return
		}

		userID, _ := auth.UserIDFromContext(r.Context())
		created, err := repo.CreateProject(r.Context(), domain.Project{
			UserID:      userID,
			Name:        req.Name,
			Goal:        req.Goal,
			Deadline:    req.Deadline,
			Constraints: req.Constraints,
			Status:      req.Status,
		})
		if err != nil {
			if errors.Is(err, store.ErrDuplicateID) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusCreated, created)
	}
}

// updateProjectRequest carries only the mutable fields — Repository.
// UpdateProject never touches Name. Version is required: it's the
// caller's optimistic-concurrency check-in, the project's version as last
// read, not something that defaults sanely to zero (architecture.md §8).
type updateProjectRequest struct {
	Version     int        `json:"version" validate:"required"`
	Goal        string     `json:"goal"`
	Deadline    *time.Time `json:"deadline,omitempty"`
	Constraints []string   `json:"constraints"`
	Status      string     `json:"status"`
}

func updateProjectHandler(repo ProjectRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateProjectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
			return
		}

		if !validateStruct(w, req) {
			return
		}

		userID, _ := auth.UserIDFromContext(r.Context())
		updated, err := repo.UpdateProject(r.Context(), userID, domain.Project{
			ID:          r.PathValue("id"),
			Version:     req.Version,
			Goal:        req.Goal,
			Deadline:    req.Deadline,
			Constraints: req.Constraints,
			Status:      req.Status,
		})
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			if errors.Is(err, store.ErrConflict) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, updated)
	}
}

// deleteProjectHandler removes a project. Returns 204 on success:
// DeleteProject only reports success/failure, not the deleted project, so
// there's nothing to echo back (unlike the 201-with-body
// createProjectHandler returns).
func deleteProjectHandler(repo ProjectRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, _ := auth.UserIDFromContext(r.Context())
		id := r.PathValue("id")
		if err := repo.DeleteProject(r.Context(), userID, id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// getProjectHandler fetches a single project.
func getProjectHandler(repo ProjectRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, _ := auth.UserIDFromContext(r.Context())
		project, err := repo.GetProject(r.Context(), userID, r.PathValue("id"))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, project)
	}
}

// listProjectsHandler lists every project owned by the caller.
func listProjectsHandler(repo ProjectRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, _ := auth.UserIDFromContext(r.Context())
		projects, err := repo.ListProjects(r.Context(), userID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, projects)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
