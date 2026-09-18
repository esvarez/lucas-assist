package domain

import "time"

// ChangesetStatus is one state in a Changeset's lifecycle (architecture.md
// §3):
//
//	proposed -> accepted -> applying -> applied
//	        \-> rejected
//	        \-> expired
//	        \-> conflict
type ChangesetStatus string

const (
	ChangesetProposed ChangesetStatus = "proposed"
	ChangesetAccepted ChangesetStatus = "accepted"
	ChangesetApplying ChangesetStatus = "applying"
	ChangesetApplied  ChangesetStatus = "applied"
	ChangesetRejected ChangesetStatus = "rejected"
	ChangesetExpired  ChangesetStatus = "expired"
	ChangesetConflict ChangesetStatus = "conflict"
)

// Changeset is a skill's proposed mutation, reviewed and explicitly
// accepted before anything is written to a Project (architecture.md §1,
// ADR 002). It's a separate entity from the AgentRun that produced it
// because proposing and accepting happen in separate requests and can fail
// independently (architecture.md §3).
type Changeset struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id"`
	UserID      string          `json:"user_id"`
	Skill       string          `json:"skill"`
	BaseVersion int             `json:"base_version"`
	Status      ChangesetStatus `json:"status"`

	// ProposedTasks is the mutation payload for decompose_task's
	// changesets — the only proposal shape needed today. A skill that
	// proposes something else (e.g. create_project) gets its own field
	// here when that's actually needed, rather than a premature generic
	// "mutations" abstraction.
	ProposedTasks []ProposedTask `json:"proposed_tasks"`

	CreatedAt time.Time `json:"created_at"`
}
