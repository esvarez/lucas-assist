package auth

import (
	"encoding/json"
	"net/http"
)

// Middleware resolves the caller's user ID once via resolver and attaches
// it to the request's context.Context for every handler downstream to
// read via UserIDFromContext — AGENTS.MD's "resolve once, put it in
// context.Context, read it from there." A request resolver can't
// authenticate is rejected with 401 before it reaches any handler; no
// handler behind this middleware needs to check for a missing user ID
// itself.
func Middleware(resolver Resolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, err := resolver.ResolveUserID(r)
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), userID)))
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
