package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

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

// acceptTaskSelection pairs a proposed task's current index — into the
// changeset's stored proposed_tasks, as last returned to the client by
// GetChangeset/ListChangesets/a prior accept — with the content to commit
// for it, possibly edited from what the model proposed (#169).
type acceptTaskSelection struct {
	Index int `json:"index"`
	domain.ProposedTask
}

// acceptChangesetRequest is the POST body for accepting a changeset.
type acceptChangesetRequest struct {
	// IdempotencyKey deduplicates retries (architecture.md §15): replaying
	// an accept with the same key returns the original result instead of
	// re-applying it. Required — there's no sane default that would still
	// protect against a duplicate submission.
	IdempotencyKey string `json:"idempotency_key" validate:"required"`

	// Tasks selects which of the changeset's currently-proposed tasks to
	// commit on this call, by index, with each one's (possibly edited)
	// content (#169) — lets the client remove or edit decompose_task's
	// proposal during review without a separate edit endpoint or a second
	// round-trip through the model. Omit it entirely to accept every
	// currently-proposed task unmodified, the original all-at-once
	// behavior. Any proposed task whose index isn't listed here stays
	// "proposed" on the changeset for a later accept (or removal via
	// PATCH .../tasks) instead of being discarded — see AcceptChangeset.
	// Ignored by acceptCreateProjectChangesetHandler, which has no
	// proposed tasks.
	Tasks []acceptTaskSelection `json:"tasks,omitempty"`
}

// splitProposedTasks partitions a changeset's stored proposed tasks into
// what an accept request selected to commit now and what's left, given
// selections by index (#169). A nil selections accepts every stored task
// unmodified. Returns an error message to send as 400 on invalid input
// (an out-of-range or repeated index, or a blank edited title), ""
// otherwise.
func splitProposedTasks(stored []domain.ProposedTask, selections []acceptTaskSelection) (accepted, remaining []domain.ProposedTask, errMsg string) {
	if selections == nil {
		return stored, nil, ""
	}

	chosen := make(map[int]domain.ProposedTask, len(selections))
	for _, sel := range selections {
		if sel.Index < 0 || sel.Index >= len(stored) {
			return nil, nil, fmt.Sprintf("tasks: index %d is out of range", sel.Index)
		}
		if _, dup := chosen[sel.Index]; dup {
			return nil, nil, fmt.Sprintf("tasks: index %d selected more than once", sel.Index)
		}
		if strings.TrimSpace(sel.Title) == "" {
			return nil, nil, fmt.Sprintf("tasks: index %d has a blank title", sel.Index)
		}
		chosen[sel.Index] = sel.ProposedTask
	}

	for i, orig := range stored {
		if edited, ok := chosen[i]; ok {
			accepted = append(accepted, edited)
		} else {
			remaining = append(remaining, orig)
		}
	}
	return accepted, remaining, ""
}

type acceptChangesetResponse struct {
	Project   domain.Project   `json:"project"`
	Tasks     []domain.Task    `json:"tasks"`
	Event     domain.Event     `json:"event"`
	Changeset domain.Changeset `json:"changeset"`
}

// acceptChangesetHandler commits some or all of a proposed changeset's
// tasks atomically (architecture.md §1's "Accept changeset -> conditional
// transaction -> applied result"). It enforces the two business rules
// that are safe to check before any I/O: the selection must resolve to at
// least one task, and it can't exceed the mutation cap. Whether the
// changeset is actually acceptable — "proposed" status, or a replay of an
// already-applied request — is AcceptChangeset's call, not this handler's:
// that decision needs the idempotency-key lookup, which only the store can
// make (see AcceptChangeset's doc comment).
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

		acceptedTasks, remainingTasks, errMsg := splitProposedTasks(changeset.ProposedTasks, req.Tasks)
		if errMsg != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
			return
		}
		// Only a genuine "you selected nothing out of what's actually
		// available" is a 400 here. If changeset.ProposedTasks was already
		// empty before this request even ran (already applied by an
		// earlier call, or a replay), that's AcceptChangeset's status/
		// idempotency check to make below, not this handler's — see
		// TestAcceptChangeset_NotProposed and
		// TestAcceptChangeset_IdempotentReplay.
		if len(changeset.ProposedTasks) > 0 && len(acceptedTasks) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "select at least one task to accept"})
			return
		}
		if len(acceptedTasks) > maxChangesetMutations {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "changeset exceeds the maximum of 50 mutations"})
			return
		}

		// The idempotency hash has to be keyed off the request as the
		// client sent it (req.Tasks, before resolving it against
		// changeset.ProposedTasks), not the resolved acceptedTasks —
		// changeset.ProposedTasks shrinks after every accept, so hashing
		// the resolved list would make a legitimate replay of the exact
		// same request hash differently once the first attempt already
		// succeeded (see requestHashWithFingerprint's doc comment).
		fingerprint, err := json.Marshal(req.Tasks)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		changeset.ProposedTasks = acceptedTasks
		result, err := repo.AcceptChangeset(r.Context(), project, changeset, remainingTasks, string(fingerprint), req.IdempotencyKey)
		if err != nil {
			if errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrIdempotencyKeyReused) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, acceptChangesetResponse{
			Project:   result.Project,
			Tasks:     result.Tasks,
			Event:     result.Event,
			Changeset: result.Changeset,
		})
	}
}

type listChangesetsResponse struct {
	Changesets []domain.Changeset `json:"changesets"`
}

// listChangesetsHandler returns a project's changesets, optionally
// filtered to one status via ?status= (e.g. ?status=proposed). Added so a
// client can rediscover a pending decompose_task proposal's id after a
// page refresh (#169) — nothing else exposes one without already having
// it in hand from the AgentRun that created it.
func listChangesetsHandler(repo ProjectRepository) http.HandlerFunc {
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

		changesets, err := repo.ListChangesets(r.Context(), userID, projectID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		if status := r.URL.Query().Get("status"); status != "" {
			filtered := make([]domain.Changeset, 0, len(changesets))
			for _, c := range changesets {
				if string(c.Status) == status {
					filtered = append(filtered, c)
				}
			}
			changesets = filtered
		}

		writeJSON(w, http.StatusOK, listChangesetsResponse{Changesets: changesets})
	}
}

type updateChangesetTasksRequest struct {
	// Tasks is the full list of proposed tasks that should remain after
	// this call — i.e. the client's current list minus whatever it just
	// removed. An empty list rejects the changeset: nothing "proposed" is
	// left to act on.
	Tasks []domain.ProposedTask `json:"tasks"`
}

type updateChangesetTasksResponse struct {
	Changeset domain.Changeset `json:"changeset"`
}

// updateChangesetTasksHandler persists removing one or more proposed
// tasks from a still-"proposed" changeset without accepting anything —
// no Task rows are created, no project version change (#169). This is
// the durable half of "remove a proposal without accepting it"; an edit
// the user hasn't accepted yet is never sent here, only ever taken
// through the accept endpoint alongside that same task's acceptance.
func updateChangesetTasksHandler(repo ProjectRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateChangesetTasksRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
			return
		}

		userID, _ := auth.UserIDFromContext(r.Context())
		projectID := r.PathValue("id")
		changesetID := r.PathValue("changesetId")

		if _, err := repo.GetChangeset(r.Context(), userID, projectID, changesetID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		changeset, err := repo.UpdateChangesetProposedTasks(r.Context(), userID, projectID, changesetID, req.Tasks)
		if err != nil {
			if errors.Is(err, store.ErrConflict) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, updateChangesetTasksResponse{Changeset: changeset})
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
