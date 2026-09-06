package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/esvarez/lucas-assist/internal/domain"
)

// MemoryRepository is an in-memory Repository for local dev and unit
// tests. State lives only as long as the process — nothing is durable.
type MemoryRepository struct {
	mu       sync.Mutex
	projects map[string]domain.Project
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{projects: make(map[string]domain.Project)}
}

func (r *MemoryRepository) CreateProject(ctx context.Context, p domain.Project) (domain.Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if p.ID == "" {
		p.ID = newID()
	} else if _, exists := r.projects[p.ID]; exists {
		return domain.Project{}, ErrDuplicateID
	}

	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now

	r.projects[p.ID] = p
	return p, nil
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failing means the platform RNG is broken
	}
	return hex.EncodeToString(b)
}
