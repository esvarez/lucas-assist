package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/esvarez/lucas-assist/internal/domain"
	"github.com/esvarez/lucas-assist/internal/store"
)

type createProjectRequest struct {
	UserID      string     `json:"user_id"`
	Name        string     `json:"name"`
	Goal        string     `json:"goal"`
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

		if req.UserID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
			return
		}

		created, err := repo.CreateProject(r.Context(), domain.Project{
			UserID:      req.UserID,
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

// deleteProjectHandler removes a project. user_id is a query param, not a
// body — DELETE requests conventionally carry no body — consistent with
// #47's "user_id is required, not silently empty" convention. Returns 204
// on success: DeleteProject only reports success/failure, not the deleted
// project, so there's nothing to echo back (unlike the 201-with-body
// createProjectHandler returns).
func deleteProjectHandler(repo ProjectRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.URL.Query().Get("user_id")
		if userID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
			return
		}

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

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
