package domain

import "time"

const (
	ProjectDomainSoftware = "software"
	ProjectDomainGeneral  = "general"
)

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
	// Domain is either ProjectDomainSoftware or ProjectDomainGeneral. See
	// the package-level constants' doc comment for the default-to-General
	// backward-compatibility rule.
	Domain string `json:"domain"`

	// Version supports optimistic concurrency on changeset commit
	// (architecture.md §8): every proposed changeset records the version
	// it was built against, and a commit is only applied if that still
	// matches. CreateProject always sets this to 1, regardless of any
	// caller-supplied value — like CreatedAt/UpdatedAt, it's repo-owned,
	// not client-set.
	Version int `json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProposedProject is the content of a Project before the commit path
// (POST /projects) assigns it an ID, status, and timestamps. It's what
// the create_project skill proposes, and what a Changeset stores until
// it's accepted — hence the dynamodbav tags alongside the json/jsonschema
// ones (same reasoning as ProposedTask).
type ProposedProject struct {
	Name        string     `json:"name" dynamodbav:"name" jsonschema:"description=Short memorable project name"`
	Goal        string     `json:"goal" dynamodbav:"goal" jsonschema:"description=What shipping or done looks like for this project"`
	Deadline    *time.Time `json:"deadline" dynamodbav:"deadline,omitempty" jsonschema:"nullable,description=Target completion date if the user gave one"`
	Constraints []string   `json:"constraints" dynamodbav:"constraints,omitempty" jsonschema:"description=Constraints or must-haves the user mentioned such as tech stack budget or timeline limits"`
}
