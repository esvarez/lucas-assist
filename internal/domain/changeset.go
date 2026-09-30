package domain

import "time"

// ChangesetStatus is one state in a Changeset's lifecycle (architecture.md
// §3):
//
//	proposed -> accepted -> applying -> applied
//	        \-> rejected
//	        \-> expired
//	        \-> conflict
//	        \-> failed
type ChangesetStatus string

const (
	ChangesetProposed ChangesetStatus = "proposed"
	ChangesetAccepted ChangesetStatus = "accepted"
	ChangesetApplying ChangesetStatus = "applying"
	ChangesetApplied  ChangesetStatus = "applied"
	ChangesetRejected ChangesetStatus = "rejected"
	ChangesetExpired  ChangesetStatus = "expired"
	ChangesetConflict ChangesetStatus = "conflict"
	// ChangesetFailed covers an apply that errored for a reason other than
	// a version conflict (architecture.md §3) — e.g. a transient DynamoDB
	// failure mid-TransactWriteItems — so it doesn't collide with
	// ChangesetConflict, which is reserved for a stale baseVersion.
	ChangesetFailed ChangesetStatus = "failed"
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
	ProposedTasks []ProposedTask `json:"proposed_tasks,omitempty"`
	Assumptions []string `json:"assumptions,omitempty"`
	ParentTaskID string `json:"parent_task_id,omitempty"`
	ProposedProject *ProposedProject `json:"proposed_project,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
