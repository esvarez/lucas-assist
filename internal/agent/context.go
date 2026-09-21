package agent

import "context"

type contextKey int

const userIDKey contextKey = iota

// WithUserID attaches the verified user ID to ctx for a Skill's
// BuildContext to read back via UserIDFromContext when it needs to load
// that user's data — every store.Repository call is scoped by user
// (architecture.md §8). Nothing sets this yet: today BuildContext is only
// ever called for input validation (internal/skillsapi) with a bare
// context.Background(). Once the Agent Worker (#101) exists, it sets this
// from the leased AgentRun's UserID before calling Run, the same way an
// HTTP auth middleware would (AGENTS.MD's "resolve once, put it in
// context.Context, read it from there").
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// UserIDFromContext reads back the ID set by WithUserID. ok is false if
// nothing set it.
func UserIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(userIDKey).(string)
	return v, ok
}
