package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/esvarez/lucas-assist/internal/store"
)

// acceptChangesetRequest is deliberately minimal: everything the commit
// needs (BaseVersion, ProposedTasks, Status) already lives on the
// changeset itself. The caller only says who's accepting it and supplies
// the idempotency key architecture.md §15 requires.
type acceptChangesetRequest struct {
	UserID         string `json:"user_id" validate:"required"`
	IdempotencyKey string `json:"idempotency_key" validate:"required"`
}

// acceptChangesetHandler backs POST
// /projects/{id}/changesets/{changesetId}/accept (architecture.md §1's
// "Accept changeset -> conditional transaction -> applied result").
// Success and an idempotent replay of a prior success return the same 200
// shape — a client retrying after a network blip can't tell the
// difference, which is the point.
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

		projectID := r.PathValue("id")
		changesetID := r.PathValue("changesetId")

		result, err := repo.AcceptChangeset(r.Context(), store.AcceptChangesetInput{
			UserID:         req.UserID,
			ProjectID:      projectID,
			ChangesetID:    changesetID,
			IdempotencyKey: req.IdempotencyKey,
			RequestHash:    acceptChangesetRequestHash(req.UserID, projectID, changesetID),
		})
		if err != nil {
			switch {
			case errors.Is(err, store.ErrNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			case errors.Is(err, store.ErrChangesetTooLarge):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			case errors.Is(err, store.ErrConflict),
				errors.Is(err, store.ErrChangesetNotProposed),
				errors.Is(err, store.ErrIdempotencyKeyMismatch):
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			}
			return
		}

		writeJSON(w, http.StatusOK, result)
	}
}

// acceptChangesetRequestHash identifies the semantic request an
// idempotency key was issued for. (userID, projectID, changesetID) fully
// determine what accepting does — there's no other variable request
// content — so reusing the same key against a different one of these
// three is exactly the "different request" architecture.md §15 says must
// be rejected.
func acceptChangesetRequestHash(userID, projectID, changesetID string) string {
	sum := sha256.Sum256([]byte(userID + ":" + projectID + ":" + changesetID))
	return hex.EncodeToString(sum[:])
}
