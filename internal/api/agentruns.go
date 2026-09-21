package api

import (
	"errors"
	"net/http"

	"github.com/esvarez/lucas-assist/internal/auth"
	"github.com/esvarez/lucas-assist/internal/domain"
	"github.com/esvarez/lucas-assist/internal/store"
)

// agentRunResponse embeds domain.AgentRun (flattened into the top-level
// JSON object) plus the associated Changeset once one exists, so a client
// polling a completed run gets both in one round trip instead of needing
// a second GET.
type agentRunResponse struct {
	domain.AgentRun
	Changeset *domain.Changeset `json:"changeset,omitempty"`
}

// getAgentRunHandler backs GET /agent-runs/{id} (architecture.md §10 step
// 8: "Client polls GET /agent-runs/{runId} with bounded backoff").
//
// architecture.md's route has no project in the URL, but domain.AgentRun's
// DynamoDB key embeds ProjectID (or a fixed placeholder for a
// project-less create_project run — see dynamo.go's agentRunSK), so a
// lookup by run ID alone isn't possible without either a GSI or a table
// Scan, neither of which is warranted for this. project_id is accepted
// here as an optional query param instead: a client polling a run it
// created against an existing project already has that project's ID in
// hand from the original request, and omitting it defaults to the
// create_project placeholder, matching how such a run was created.
func getAgentRunHandler(repo ProjectRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, _ := auth.UserIDFromContext(r.Context())
		projectID := r.URL.Query().Get("project_id")

		run, err := repo.GetAgentRun(r.Context(), userID, projectID, r.PathValue("id"))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		resp := agentRunResponse{AgentRun: run}
		if run.Status == domain.AgentRunCompleted && run.ChangesetID != "" {
			changeset, err := repo.GetChangeset(r.Context(), userID, run.ProjectID, run.ChangesetID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			resp.Changeset = &changeset
		}

		writeJSON(w, http.StatusOK, resp)
	}
}
