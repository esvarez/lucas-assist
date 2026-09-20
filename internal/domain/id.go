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

// newSortableID generates a URL-safe, chronologically sortable identifier:
// a fixed-width, zero-padded UnixNano timestamp followed by a nanoid
// suffix for collision resistance within the same nanosecond. Shared by
// every ID scheme below that folds a timestamp into the ID itself instead
// of a separate SK segment — see NewRunID's doc comment for why.
func newSortableID() string {
	return fmt.Sprintf("%020d-%s", time.Now().UTC().UnixNano(), gonanoid.Must())
}

// NewRunID generates a URL-safe, chronologically sortable identifier for an
// AgentRun.
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
	return newSortableID()
}

// NewEventID generates a URL-safe, chronologically sortable identifier for
// an Event, same reasoning and scheme as NewRunID: architecture.md §8
// documents Event's sort key as `P#<pid>#EVT#<timestamp>#<event_id>`, and
// folding the timestamp into the ID reduces that to `P#<pid>#EVT#<event_id>`
// (dynamo.go's eventSK) — sortable and directly addressable without a
// second lookup.
func NewEventID() string {
	return newSortableID()
}
