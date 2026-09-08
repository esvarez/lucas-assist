// Package skillsapi is the HTTP layer for skill dispatch: envelope
// parsing, registry lookup, agent.Run, and error-to-status mapping. It's
// a plain net/http.Handler shared between cmd/skills (wrapped for Lambda)
// and cmd/local, mirroring how internal/api is shared between cmd/api
// and cmd/local (architecture.md §11's "handlers are plain net/http"
// principle, extended to the Skill Lambda).
package skillsapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/esvarez/lucas-assist/internal/agent"
)

type requestEnvelope struct {
	Skill string          `json:"skill"`
	Input json.RawMessage `json:"input"`
}

// NewHandler builds the skill-dispatch HTTP handler against reg.
func NewHandler(reg *agent.Registry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var envelope requestEnvelope
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
			return
		}

		skill, err := reg.Get(envelope.Skill)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}

		result, err := agent.Run(r.Context(), skill, envelope.Input)
		if err != nil {
			writeJSON(w, statusForRunError(err), map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, result)
	})
}

// statusForRunError maps an agent.Run error to an HTTP status: malformed
// or missing skill input is the client's mistake (400), anything else
// (the chat completion call, the model's output) is treated as upstream
// (502) — Run's errors are wrapped with %w all the way down, so this
// checks all the way down too.
func statusForRunError(err error) int {
	if errors.Is(err, agent.ErrInvalidInput) {
		return http.StatusBadRequest
	}
	return http.StatusBadGateway
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
