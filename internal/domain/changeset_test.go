package domain

import "testing"

// TestChangesetStatus_MatchesLifecycleDiagram guards against silently
// dropping a state from architecture.md §3's Changeset lifecycle diagram —
// which is exactly what happened once already: ChangesetFailed was missed
// on the first pass despite being in the diagram.
func TestChangesetStatus_MatchesLifecycleDiagram(t *testing.T) {
	want := map[ChangesetStatus]string{
		ChangesetProposed: "proposed",
		ChangesetAccepted: "accepted",
		ChangesetApplying: "applying",
		ChangesetApplied:  "applied",
		ChangesetRejected: "rejected",
		ChangesetExpired:  "expired",
		ChangesetConflict: "conflict",
		ChangesetFailed:   "failed",
	}

	if len(want) != 8 {
		t.Fatalf("test itself is wrong: want %d distinct statuses, got %d", 8, len(want))
	}

	for status, value := range want {
		if string(status) != value {
			t.Errorf("%v = %q, want %q", status, string(status), value)
		}
	}
}
