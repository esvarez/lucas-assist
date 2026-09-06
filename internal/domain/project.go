package domain

import "time"

// Project is the top-level entity everything else — tasks, decisions,
// events, notes — hangs off of. It's the "project card" injected into
// every skill prompt.
type Project struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Goal        string     `json:"goal"`
	Deadline    *time.Time `json:"deadline,omitempty"`
	Constraints []string   `json:"constraints"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
