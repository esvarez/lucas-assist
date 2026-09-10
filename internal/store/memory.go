package store

import (
	"context"
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
		p.ID = domain.NewID()
	} else if _, exists := r.projects[p.ID]; exists {
		return domain.Project{}, ErrDuplicateID
	}

	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now

	r.projects[p.ID] = p
	return p, nil
}

func (r *MemoryRepository) GetProject(ctx context.Context, userID, id string) (domain.Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.projects[id]
	if !ok || p.UserID != userID {
		return domain.Project{}, ErrNotFound
	}
	return p, nil
}

func (r *MemoryRepository) DeleteProject(ctx context.Context, userID, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.projects[id]
	if !ok || p.UserID != userID {
		return ErrNotFound
	}

	delete(r.projects, id)
	return nil
}

func (r *MemoryRepository) UpdateProject(ctx context.Context, userID string, p domain.Project) (domain.Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.projects[p.ID]
	if !ok || existing.UserID != userID {
		return domain.Project{}, ErrNotFound
	}

	existing.Goal = p.Goal
	existing.Deadline = p.Deadline
	existing.Constraints = p.Constraints
	existing.Status = p.Status
	existing.UpdatedAt = time.Now().UTC()

	r.projects[existing.ID] = existing
	return existing, nil
}

func (r *MemoryRepository) ListProjects(ctx context.Context, userID string) ([]domain.Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	projects := make([]domain.Project, 0)
	for _, p := range r.projects {
		if p.UserID == userID {
			projects = append(projects, p)
		}
	}
	return projects, nil
}
