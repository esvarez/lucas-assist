package api

import (
	"errors"
	"net/http"

	"github.com/esvarez/lucas-assist/internal/store"
)

// listTasksHandler backs GET /projects/{id}/tasks (#125). It returns the
// project's tasks flat, exactly as domain.Task and store.Repository.
// ListTasks already shape them (ParentID-linked, not nested) — the same
// convention every other route in this package follows for domain.Project
// and domain.Changeset: no response-shape transformation here. Building a
// tree out of the flat list, if a caller wants one, is presentation logic
// that belongs on the client (see web/src/api/tasks.ts).
//
// The project is fetched first, the same ownership check
// acceptChangesetHandler does, so a nonexistent or another user's project
// ID 404s instead of silently returning an empty task list —
// store.Repository.ListTasks itself never returns ErrNotFound, since it
// only queries a key prefix and has no way to distinguish "no tasks yet"
// from "no such project."
func listTasksHandler(repo ProjectRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.URL.Query().Get("user_id")
		if userID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
			return
		}

		projectID := r.PathValue("id")

		if _, err := repo.GetProject(r.Context(), userID, projectID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		tasks, err := repo.ListTasks(r.Context(), userID, projectID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, tasks)
	}
}
