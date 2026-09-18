package domain

import (
	"fmt"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// NewID generates a URL-safe, collision-resistant identifier. Used for
// Project and Task IDs; Decision/Event keys off a timestamp instead (see
// architecture.md §8's key schema table).
func NewID() string {
	return gonanoid.Must()
}

// NewRunID generates a URL-safe, chronologically sortable identifier for an
// AgentRun: a fixed-width, zero-padded UnixNano timestamp followed by a
// nanoid suffix for collision resistance within the same nanosecond.
//
// architecture.md §8 puts AgentRun's sort key as
// `P#<pid>#RUN#<timestamp>#<run_id>` — a separate timestamp segment. Storing
// the timestamp inside the ID itself instead means the run's SK reduces to
// `P#<pid>#RUN#<run_id>` (see dynamo.go's agentRunSK): still chronologically
// sortable by Query, but also directly addressable by GetItem from
// (userID, projectID, runID) alone, matching every other Get* method in
// store.Repository instead of requiring a second lookup just to learn the
// timestamp.
func NewRunID() string {
	return fmt.Sprintf("%020d-%s", time.Now().UTC().UnixNano(), gonanoid.Must())
}
