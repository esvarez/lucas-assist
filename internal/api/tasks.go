package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/esvarez/lucas-assist/internal/auth"
	"github.com/esvarez/lucas-assist/internal/domain"
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
		userID, _ := auth.UserIDFromContext(r.Context())
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

// updateTaskStatusRequest's Status is restricted to the vocabulary
// web/src/lib/task-status.ts already renders a label/color for — the only
// four values any client does anything with today (#181).
type updateTaskStatusRequest struct {
	Status string `json:"status" validate:"required,oneof=todo in-progress done blocked"`
}

type updateTaskStatusResponse struct {
	Task domain.Task `json:"task"`
}

// updateTaskStatusHandler backs PATCH /projects/{id}/tasks/{taskId} (#181)
// — the durable half of the project detail page's done checkbox, which
// previously only ever updated its own local React state and forgot the
// change on the next page load. No precheck fetch: UpdateTaskStatus's own
// conditional update already reports ErrNotFound unambiguously (unlike
// updateChangesetTasksHandler's UpdateChangesetProposedTasks, which
// conflates "missing" with "wrong status" and so needs one).
func updateTaskStatusHandler(repo ProjectRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateTaskStatusRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
			return
		}

		if !validateStruct(w, req) {
			return
		}

		userID, _ := auth.UserIDFromContext(r.Context())
		projectID := r.PathValue("id")
		taskID := r.PathValue("taskId")

		task, err := repo.UpdateTaskStatus(r.Context(), userID, projectID, taskID, req.Status)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, updateTaskStatusResponse{Task: task})
	}
}
