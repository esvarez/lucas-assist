package domain

import "time"

// EventChangesetApplied is the only Event type today: a changeset was
// accepted and committed. Kept as a plain string rather than a typed enum
// (unlike ChangesetStatus/AgentRunStatus) because the set of event types is
// expected to grow open-endedly as more skills gain accept paths, not
// cycle through one fixed lifecycle.
const EventChangesetApplied = "changeset_applied"

// Event is an append-only record of one accepted project change
// (architecture.md §3/§9), written in the same transaction as the change
// itself so project state and audit record never diverge.
//
// UserID isn't in architecture.md §3's field list for Event, but it's
// needed to build PK (architecture.md §8: PK=USER#<uid>) and every other
// independently-keyed entity here carries it directly (domain.Changeset,
// domain.AgentRun) rather than requiring a separate lookup parameter.
type Event struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	UserID      string    `json:"user_id"`
	Type        string    `json:"type"`
	ChangesetID string    `json:"changeset_id"`
	ActorID     string    `json:"actor_id"`
	BaseVersion int       `json:"base_version"`
	CreatedAt   time.Time `json:"created_at"`
}
