package domain

import "time"

// EventType identifies what kind of accepted change an Event records. Only
// one value exists today — the changeset-accept endpoint that writes
// events doesn't yet have a second kind to distinguish from
// (architecture.md §9's audit log currently has exactly one writer).
type EventType string

const (
	// EventChangesetAccepted marks a changeset's proposed mutations being
	// committed to a project (architecture.md §1/§9).
	EventChangesetAccepted EventType = "changeset_accepted"
)

// Event is an append-only audit record of an accepted project change,
// written in the same transaction as the changes it describes so project
// state and the audit log never diverge (architecture.md §9). It's never
// mutated after creation.
type Event struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	UserID      string    `json:"user_id"`
	Type        EventType `json:"type"`
	ChangesetID string    `json:"changeset_id"`

	// ActorID is who accepted the change. It's UserID today — every
	// project is single-owner (AGENTS.MD) — but kept as its own field since
	// architecture.md §14 calls out recording the actor explicitly,
	// distinct from project ownership.
	ActorID string `json:"actor_id"`

	BaseVersion int       `json:"base_version"`
	CreatedAt   time.Time `json:"created_at"`
}
