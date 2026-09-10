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
	// UserID is caller-supplied for now — populating it from a verified
	// JWT subject is separate, undecided auth work (domain.Project).
	// Required: the DynamoDB key schema partitions by user
	// (PK=USER#<uid>), so an empty UserID isn't a valid project owner.
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

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
