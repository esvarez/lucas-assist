// Package skillsapi is the HTTP layer for skill dispatch: envelope
// parsing, registry lookup, AgentRun creation, and enqueueing. It's a
// plain net/http.Handler shared between cmd/skills (wrapped for Lambda)
// and cmd/local, mirroring how internal/api is shared between cmd/api
// and cmd/local (architecture.md §11's "handlers are plain net/http"
// principle, extended to the Skill Lambda).
//
// Dispatch is asynchronous (architecture.md §1/§10): this handler creates
// a queued AgentRun and enqueues it, returning 202 with the run ID. It
// never calls agent.Run itself — the Agent Worker (#101) does that once it
// leases the run off the queue.
package skillsapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/esvarez/lucas-assist/internal/agent"
	"github.com/esvarez/lucas-assist/internal/domain"
)

type requestEnvelope struct {
	Skill string          `json:"skill"`
	Input json.RawMessage `json:"input"`

	// UserID is required — every AgentRun needs an owner (architecture.md
	// §8: every item is partitioned by user).
	UserID string `json:"user_id"`

	// ProjectID is optional: empty for create_project (which doesn't have
	// a project yet — that's the point of the run) and required in
	// practice for skills that operate on an existing project, though this
	// handler doesn't enforce that per-skill — see domain.AgentRun.
	ProjectID string `json:"project_id"`
}

// AgentRunStore is the slice of store.Repository skillsapi actually calls —
// same narrowing convention as internal/api's ProjectRepository.
// FailAgentRun exists alongside CreateAgentRun so a run that was created
// but never made it onto the queue doesn't stay queued forever with
// nothing left to lease it (see NewHandler's enqueue-failure handling).
type AgentRunStore interface {
	CreateAgentRun(ctx context.Context, r domain.AgentRun) (domain.AgentRun, error)
	FailAgentRun(ctx context.Context, userID, projectID, runID, errMsg string) (domain.AgentRun, error)
}

// Enqueuer is the slice of *queue.Enqueuer skillsapi actually calls,
// narrowed so tests can stub it without a real SQS client.
type Enqueuer interface {
	EnqueueRun(ctx context.Context, runID string) error
}

// NewHandler builds the skill-dispatch HTTP handler against reg, runs, and
// enqueuer.
func NewHandler(reg *agent.Registry, runs AgentRunStore, enqueuer Enqueuer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var envelope requestEnvelope
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
			return
		}

		if envelope.UserID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
			return
		}

		skill, err := reg.Get(envelope.Skill)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}

		// BuildContext is called here purely to validate the input shape —
		// its chat messages are discarded; the model call itself happens
		// later in the Agent Worker (#101), once this run is leased off
		// the queue. No point creating and enqueueing a run for a request
		// that can't possibly succeed. envelope.UserID is attached to ctx
		// first since a skill (e.g. decompose_task with a project_id) may
		// need it to load that user's data (architecture.md §8).
		ctx := agent.WithUserID(r.Context(), envelope.UserID)
		if _, err := skill.BuildContext(ctx, envelope.Input); err != nil {
			writeJSON(w, statusForBuildContextError(err), map[string]string{"error": err.Error()})
			return
		}

		run, err := runs.CreateAgentRun(r.Context(), domain.AgentRun{
			UserID:    envelope.UserID,
			ProjectID: envelope.ProjectID,
			Skill:     envelope.Skill,
			Input:     envelope.Input,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		if err := enqueuer.EnqueueRun(r.Context(), run.ID); err != nil {
			// The run was already persisted as queued, but nothing will
			// ever send it to the queue a second time — left as-is, it
			// would sit queued forever with no worker able to lease it, and
			// the caller has no run_id to poll or retry against (#100).
			// Marking it failed here surfaces that plainly instead of
			// silently orphaning the row. Best-effort: if this also fails,
			// the run stays orphaned, but the caller still gets the
			// original enqueue error.
			if _, failErr := runs.FailAgentRun(r.Context(), run.UserID, run.ProjectID, run.ID, err.Error()); failErr != nil {
				err = fmt.Errorf("%w (and failed to mark run %q failed: %v)", err, run.ID, failErr)
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error(), "run_id": run.ID})
			return
		}

		writeJSON(w, http.StatusAccepted, map[string]string{"run_id": run.ID})
	})
}

// statusForBuildContextError maps a BuildContext validation error to an
// HTTP status: malformed or missing skill input is the client's mistake
// (400). BuildContext does no I/O, so anything else is an unexpected
// internal error (500), not an upstream failure — unlike the old
// statusForRunError, which also had to account for agent.Run's OpenAI call.
func statusForBuildContextError(err error) int {
	if errors.Is(err, agent.ErrInvalidInput) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
