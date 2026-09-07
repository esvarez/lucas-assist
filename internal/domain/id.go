package domain

import gonanoid "github.com/matoous/go-nanoid/v2"

// NewID generates a URL-safe, collision-resistant identifier. Not every
// entity uses this scheme — Task IDs are UUIDs, Decision/Event keys off a
// timestamp instead. See architecture.md's "ID formats" section.
func NewID() string {
	return gonanoid.Must()
}
