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

	// ProposedTasks is the mutation payload for decompose_task's
	// changesets. Exactly one of ProposedTasks/ProposedProject is
	// populated, matching Skill — there's no dedicated "changeset kind"
	// field, since Skill already says which shape to expect.
	ProposedTasks []ProposedTask `json:"proposed_tasks,omitempty"`

	// Assumptions are decompose_task's disclosed unspecified-choice
	// decisions — every choice the model made on the user's behalf because
	// the task didn't specify it, meant to be read before the subtask diff
	// during changeset review. Empty for create_project changesets.
	Assumptions []string `json:"assumptions,omitempty"`

	// ProposedProject is create_project's mutation payload (#101) — the
	// second proposal shape #73 anticipated needing "when actually
	// needed." A create_project run has no ProjectID yet (that's the
	// point of the run), so BaseVersion is meaningless for these
	// changesets: there's no existing project version to check against.
	ProposedProject *ProposedProject `json:"proposed_project,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}
