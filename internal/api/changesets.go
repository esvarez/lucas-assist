package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/esvarez/lucas-assist/internal/auth"
	"github.com/esvarez/lucas-assist/internal/domain"
	"github.com/esvarez/lucas-assist/internal/store"
)

// maxChangesetMutations caps an accepted changeset at 50 proposed tasks
// (architecture.md §8, ADR 009) — comfortably below TransactWriteItems'
// 100-item limit once the project version update, changeset status
// update, event, and idempotency record are added (4 more items,
// DynamoRepository.AcceptChangeset). A changeset over this limit is
// rejected outright; splitting an oversized proposal into multiple
// reviewable changesets is the worker's job (#76/#101), not this
// endpoint's.
const maxChangesetMutations = 50

// acceptChangesetRequest is the POST body for accepting a changeset.
type acceptChangesetRequest struct {
	// IdempotencyKey deduplicates retries (architecture.md §15): replaying
	// an accept with the same key returns the original result instead of
	// re-applying it. Required — there's no sane default that would still
	// protect against a duplicate submission.
	IdempotencyKey string `json:"idempotency_key" validate:"required"`
}

type acceptChangesetResponse struct {
	Project domain.Project `json:"project"`
	Tasks   []domain.Task  `json:"tasks"`
	Event   domain.Event   `json:"event"`
}

// acceptChangesetHandler commits a proposed changeset's tasks atomically
// (architecture.md §1's "Accept changeset -> conditional transaction ->
// applied result"). It enforces the one business rule that's safe to check
// before any I/O: the mutation cap. Whether the changeset is actually
// acceptable — "proposed" status, or a replay of an already-applied one —
// is AcceptChangeset's call, not this handler's: that decision needs the
// idempotency-key lookup, which only the store can make (see
// AcceptChangeset's doc comment).
func acceptChangesetHandler(repo ProjectRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req acceptChangesetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
			return
		}

		if !validateStruct(w, req) {
			return
		}

		userID, _ := auth.UserIDFromContext(r.Context())
		projectID := r.PathValue("id")
		changesetID := r.PathValue("changesetId")

		project, err := repo.GetProject(r.Context(), userID, projectID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		changeset, err := repo.GetChangeset(r.Context(), userID, projectID, changesetID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		if len(changeset.ProposedTasks) > maxChangesetMutations {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "changeset exceeds the maximum of 50 mutations"})
			return
		}

		result, err := repo.AcceptChangeset(r.Context(), project, changeset, req.IdempotencyKey)
		if err != nil {
			if errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrIdempotencyKeyReused) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, acceptChangesetResponse{
			Project: result.Project,
			Tasks:   result.Tasks,
			Event:   result.Event,
		})
	}
}

// acceptCreateProjectChangesetHandler commits a create_project changeset
// into a brand-new project (architecture.md §1/§9, #152). It's a separate
// route and handler from acceptChangesetHandler above, not a variant of
// it: a create_project changeset has no existing project yet, so there's
// no {id} to address in the URL and no existing project for GetProject to
// load first.
func acceptCreateProjectChangesetHandler(repo ProjectRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req acceptChangesetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
			return
		}

		if !validateStruct(w, req) {
			return
		}

		userID, _ := auth.UserIDFromContext(r.Context())
		changesetID := r.PathValue("changesetId")

		// projectID is "" — the NOPROJECT placeholder segment
		// (internal/store/dynamo.go) is exactly what lets this lookup
		// succeed for a project-less changeset.
		changeset, err := repo.GetChangeset(r.Context(), userID, "", changesetID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		// Fast, no-I/O check, same tier as acceptChangesetHandler's
		// maxChangesetMutations check above: this route only ever commits
		// a create_project proposal. A task changeset (or a malformed one
		// with neither payload set) belongs to the other route instead.
		if changeset.Skill != "create_project" || changeset.ProposedProject == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "changeset is not a create_project proposal"})
			return
		}

		result, err := repo.AcceptCreateProjectChangeset(r.Context(), changeset, req.IdempotencyKey)
		if err != nil {
			if errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrIdempotencyKeyReused) || errors.Is(err, store.ErrDuplicateID) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusCreated, result.Project)
	}
}
