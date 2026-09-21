// Package auth resolves the caller's verified user ID from an HTTP
// request and makes it available to handlers via context.Context
// (architecture.md §12/§14, AGENTS.MD's "resolve once, put it in
// context.Context, read it from there"). It is the HTTP-request-scoped
// identity — separate from internal/agent's WithUserID/UserIDFromContext,
// which carries the skill-execution identity the Agent Worker sets from a
// leased AgentRun's stored UserID, a different lifecycle entirely.
package auth

import "context"

type contextKey int

const userIDKey contextKey = iota

// WithUserID attaches the resolved user ID to ctx.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// UserIDFromContext reads back the ID set by WithUserID. ok is false if
// nothing set it — which Middleware guarantees never happens for a
// request that reached a wrapped handler, since it 401s anything it
// couldn't resolve a user ID for first.
func UserIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(userIDKey).(string)
	return v, ok
}
