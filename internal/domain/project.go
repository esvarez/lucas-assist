package domain

import "time"

// Project is the top-level entity everything else — tasks, decisions,
// events, notes — hangs off of. It's the "project card" injected into
// every skill prompt.
type Project struct {
	// UserID scopes the project to its owner (architecture.md §7: DynamoDB
	// partitions by user). Caller-supplied for now — populating it from a
	// verified JWT subject is separate, undecided auth work.
	UserID      string     `json:"user_id"`
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Goal        string     `json:"goal"`
	Deadline    *time.Time `json:"deadline,omitempty"`
	Constraints []string   `json:"constraints"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// ProposedProject is the content of a Project before the commit path
// (POST /projects) assigns it an ID, status, and timestamps. It's what
// the create_project skill proposes.
type ProposedProject struct {
	Name        string     `json:"name" jsonschema:"description=Short memorable project name"`
	Goal        string     `json:"goal" jsonschema:"description=What shipping or done looks like for this project"`
	Deadline    *time.Time `json:"deadline" jsonschema:"nullable,description=Target completion date if the user gave one"`
	Constraints []string   `json:"constraints" jsonschema:"description=Constraints or must-haves the user mentioned such as tech stack budget or timeline limits"`
}
