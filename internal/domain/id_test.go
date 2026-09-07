package domain

import (
	"net/url"
	"testing"
)

func TestNewID(t *testing.T) {
	id := NewID()

	if id == "" {
		t.Fatal("NewID() = \"\", want a non-empty ID")
	}

	if esc := url.QueryEscape(id); esc != id {
		t.Errorf("NewID() = %q, not URL-safe (escapes to %q)", id, esc)
	}
}

func TestNewID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	const n = 1000

	for i := 0; i < n; i++ {
		id := NewID()
		if seen[id] {
			t.Fatalf("NewID() produced a duplicate after %d calls: %q", i, id)
		}
		seen[id] = true
	}
}
