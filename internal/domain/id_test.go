package domain

import (
	"net/url"
	"testing"
	"time"
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

func TestNewRunID(t *testing.T) {
	id := NewRunID()

	if id == "" {
		t.Fatal("NewRunID() = \"\", want a non-empty ID")
	}

	if esc := url.QueryEscape(id); esc != id {
		t.Errorf("NewRunID() = %q, not URL-safe (escapes to %q)", id, esc)
	}
}

func TestNewRunID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	const n = 1000

	for i := 0; i < n; i++ {
		id := NewRunID()
		if seen[id] {
			t.Fatalf("NewRunID() produced a duplicate after %d calls: %q", i, id)
		}
		seen[id] = true
	}
}

func TestNewRunID_ChronologicallySortable(t *testing.T) {
	first := NewRunID()
	time.Sleep(time.Millisecond)
	second := NewRunID()

	if first >= second {
		t.Errorf("NewRunID() = %q then %q, want lexically increasing IDs", first, second)
	}
}

func TestNewEventID(t *testing.T) {
	id := NewEventID()

	if id == "" {
		t.Fatal("NewEventID() = \"\", want a non-empty ID")
	}

	if esc := url.QueryEscape(id); esc != id {
		t.Errorf("NewEventID() = %q, not URL-safe (escapes to %q)", id, esc)
	}
}

func TestNewEventID_ChronologicallySortable(t *testing.T) {
	first := NewEventID()
	time.Sleep(time.Millisecond)
	second := NewEventID()

	if first >= second {
		t.Errorf("NewEventID() = %q then %q, want lexically increasing IDs", first, second)
	}
}
