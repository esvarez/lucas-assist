package domain

import gonanoid "github.com/matoous/go-nanoid/v2"

// NewID generates a URL-safe, collision-resistant identifier. Used for
// Project and Task IDs; Decision/Event keys off a timestamp instead (see
// architecture.md §8's key schema table).
func NewID() string {
	return gonanoid.Must()
}
