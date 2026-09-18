package domain

import (
	"encoding/json"
	"time"
)

// AgentRunStatus is one state in an AgentRun's lifecycle (architecture.md
// §3):
//
//	queued -> running -> completed
//	                  \-> failed
//	                  \-> cancelled
type AgentRunStatus string

const (
	AgentRunQueued    AgentRunStatus = "queued"
	AgentRunRunning   AgentRunStatus = "running"
	AgentRunCompleted AgentRunStatus = "completed"
	AgentRunFailed    AgentRunStatus = "failed"
	AgentRunCancelled AgentRunStatus = "cancelled"
)

// AgentRun is the execution record for one skill invocation (architecture.md
// §1/§10): the API creates it `queued`, a worker leases it, calls the model,
// and marks it `completed`/`failed`; the client polls it. It's a separate
// entity from the Changeset it may produce because proposing and accepting
// happen in separate requests and can fail independently (architecture.md
// §3).
type AgentRun struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`

	// ProjectID is empty for create_project runs — that skill's whole job
	// is to produce the project, so no project exists yet when the run is
	// created. Every other skill scopes its run to an existing project.
	// See dynamo.go's agentRunSK for how the DynamoDB key schema
	// accommodates the empty case.
	ProjectID string `json:"project_id"`

	Skill  string         `json:"skill"`
	Status AgentRunStatus `json:"status"`

	// Input is the skill's raw request body, captured at enqueue time so
	// the worker that eventually leases the run doesn't need it re-sent
	// through SQS.
	Input json.RawMessage `json:"input,omitempty"`

	// Attempt, WorkerID, and LeaseUntil implement the lease protocol
	// (architecture.md §15): a worker claims a run by conditionally moving
	// it queued->running and recording who holds it and until when, so a
	// duplicate SQS delivery can't be processed twice while a lease is
	// live, and a crashed worker's lease can be reclaimed once it expires.
	Attempt    int        `json:"attempt"`
	WorkerID   string     `json:"worker_id,omitempty"`
	LeaseUntil *time.Time `json:"lease_until,omitempty"`

	// Error holds the failure reason when Status is AgentRunFailed.
	Error string `json:"error,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
