package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

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

// createTaskRequest is a single task added by hand (#175), outside the
// decompose_task propose/accept flow. ParentID, when set, makes it a
// subtask of an existing task in the same project.
type createTaskRequest struct {
	Title              string   `json:"title" validate:"required"`
	Description        string   `json:"description"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	ParentID           string   `json:"parent_id"`
}

// createTaskHandler backs POST /projects/{id}/tasks (#175). It's a plain
// CRUD write of content the user typed themselves — no model output is
// involved, so it goes straight to store.Repository.CreateTask rather
// than through a changeset (AGENTS.MD rule 1 only covers LLM proposals).
//
// Same ownership check as listTasksHandler: the project is fetched first
// so a nonexistent or another user's project ID 404s. A parent_id is
// looked up the same way, scoped to this user and project, so a subtask
// can never be attached under a task it couldn't otherwise see.
func createTaskHandler(repo ProjectRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createTaskRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
			return
		}

		// Trimmed before validating so a whitespace-only title fails
		// "required" instead of creating a blank-looking task.
		req.Title = strings.TrimSpace(req.Title)
		req.Description = strings.TrimSpace(req.Description)
		req.ParentID = strings.TrimSpace(req.ParentID)
		criteria := make([]string, 0, len(req.AcceptanceCriteria))
		for _, c := range req.AcceptanceCriteria {
			if c = strings.TrimSpace(c); c != "" {
				criteria = append(criteria, c)
			}
		}

		if !validateStruct(w, req) {
			return
		}

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

		if req.ParentID != "" {
			if _, err := repo.GetTask(r.Context(), userID, projectID, req.ParentID); err != nil {
				if errors.Is(err, store.ErrNotFound) {
					writeJSON(w, http.StatusBadRequest, validationErrorResponse{
						Error:  "validation failed",
						Fields: map[string]string{"parent_id": "parent task not found in this project"},
					})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
		}

		// Appended after its siblings — the list is sorted by order within
		// each parent (web/src/api/tasks.ts buildTaskTree), so a hand-added
		// task lands at the end rather than colliding with an existing one.
		existing, err := repo.ListTasks(r.Context(), userID, projectID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		order := 0
		for _, t := range existing {
			if t.ParentID == req.ParentID && t.Order >= order {
				order = t.Order + 1
			}
		}

		created, err := repo.CreateTask(r.Context(), userID, domain.Task{
			ProjectID:   projectID,
			ParentID:    req.ParentID,
			Title:       req.Title,
			Description: req.Description,
			// "todo" for the same reason AcceptChangeset uses it: it's the
			// unstarted status the rest of the app treats as eligible.
			Status:             "todo",
			Order:              order,
			AcceptanceCriteria: criteria,
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
