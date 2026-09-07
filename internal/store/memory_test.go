package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/esvarez/lucas-assist/internal/domain"
)

var _ Repository = (*MemoryRepository)(nil)

func TestMemoryRepository_CreateProject(t *testing.T) {
	repo := NewMemoryRepository()

	created, err := repo.CreateProject(context.Background(), domain.Project{
		Name:        "Nudge",
		Goal:        "Ship the POC",
		Constraints: []string{"no VPC", "no SSR"},
		Status:      "active",
	})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if created.ID == "" {
		t.Error("ID = \"\", want a generated ID")
	}
	if created.Name != "Nudge" || created.Goal != "Ship the POC" {
		t.Errorf("CreateProject() = %+v, want Name/Goal preserved from input", created)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Errorf("CreateProject() = %+v, want CreatedAt/UpdatedAt set", created)
	}
	if !created.CreatedAt.Equal(created.UpdatedAt) {
		t.Errorf("CreatedAt = %v, UpdatedAt = %v, want equal on creation", created.CreatedAt, created.UpdatedAt)
	}
}

func TestMemoryRepository_CreateProject_ExplicitID(t *testing.T) {
	repo := NewMemoryRepository()

	created, err := repo.CreateProject(context.Background(), domain.Project{ID: "proj_1", Name: "Nudge"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if created.ID != "proj_1" {
		t.Errorf("ID = %q, want the caller-supplied ID %q", created.ID, "proj_1")
	}
}

func TestMemoryRepository_CreateProject_DuplicateID(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	if _, err := repo.CreateProject(ctx, domain.Project{ID: "proj_1", Name: "First"}); err != nil {
		t.Fatalf("first CreateProject() error = %v", err)
	}

	_, err := repo.CreateProject(ctx, domain.Project{ID: "proj_1", Name: "Second"})
	if !errors.Is(err, ErrDuplicateID) {
		t.Fatalf("second CreateProject() error = %v, want %v", err, ErrDuplicateID)
	}
}

func TestMemoryRepository_CreateProject_GeneratesUniqueIDs(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	first, err := repo.CreateProject(ctx, domain.Project{Name: "First"})
	if err != nil {
		t.Fatalf("first CreateProject() error = %v", err)
	}
	second, err := repo.CreateProject(ctx, domain.Project{Name: "Second"})
	if err != nil {
		t.Fatalf("second CreateProject() error = %v", err)
	}

	if first.ID == second.ID {
		t.Errorf("both projects got ID %q, want unique generated IDs", first.ID)
	}
}

func TestMemoryRepository_GetProject_Found(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateProject(ctx, domain.Project{Name: "Nudge"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	got, err := repo.GetProject(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if !reflect.DeepEqual(got, created) {
		t.Errorf("GetProject() = %+v, want %+v", got, created)
	}
}

func TestMemoryRepository_GetProject_NotFound(t *testing.T) {
	repo := NewMemoryRepository()

	_, err := repo.GetProject(context.Background(), "does-not-exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetProject() error = %v, want %v", err, ErrNotFound)
	}
}

func TestMemoryRepository_UpdateProject(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateProject(ctx, domain.Project{
		Name:        "Nudge",
		Goal:        "Ship the POC",
		Constraints: []string{"no VPC"},
		Status:      "active",
	})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	time.Sleep(time.Millisecond)

	deadline := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	updated, err := repo.UpdateProject(ctx, domain.Project{
		ID:          created.ID,
		Name:        "should be ignored",
		Goal:        "Ship v2",
		Deadline:    &deadline,
		Constraints: []string{"no VPC", "no SSR"},
		Status:      "done",
	})
	if err != nil {
		t.Fatalf("UpdateProject() error = %v", err)
	}

	if updated.Name != created.Name {
		t.Errorf("Name = %q, want unchanged %q", updated.Name, created.Name)
	}
	if updated.Goal != "Ship v2" || updated.Status != "done" {
		t.Errorf("UpdateProject() = %+v, want Goal/Status updated", updated)
	}
	if updated.Deadline == nil || !updated.Deadline.Equal(deadline) {
		t.Errorf("Deadline = %v, want %v", updated.Deadline, deadline)
	}
	if !reflect.DeepEqual(updated.Constraints, []string{"no VPC", "no SSR"}) {
		t.Errorf("Constraints = %v, want updated", updated.Constraints)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Errorf("UpdatedAt = %v, want after %v", updated.UpdatedAt, created.UpdatedAt)
	}
	if !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("CreatedAt = %v, want unchanged %v", updated.CreatedAt, created.CreatedAt)
	}

	got, err := repo.GetProject(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if !reflect.DeepEqual(got, updated) {
		t.Errorf("GetProject() after update = %+v, want %+v", got, updated)
	}
}

func TestMemoryRepository_UpdateProject_NotFound(t *testing.T) {
	repo := NewMemoryRepository()

	_, err := repo.UpdateProject(context.Background(), domain.Project{ID: "does-not-exist"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateProject() error = %v, want %v", err, ErrNotFound)
	}
}

func TestMemoryRepository_ListProjects(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	if _, err := repo.CreateProject(ctx, domain.Project{Name: "First"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := repo.CreateProject(ctx, domain.Project{Name: "Second"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	projects, err := repo.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(projects) != 2 {
		t.Errorf("ListProjects() returned %d projects, want 2", len(projects))
	}
}

func TestMemoryRepository_ListProjects_Empty(t *testing.T) {
	repo := NewMemoryRepository()

	projects, err := repo.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if projects == nil {
		t.Error("ListProjects() = nil, want an empty (non-nil) slice")
	}
	if len(projects) != 0 {
		t.Errorf("ListProjects() returned %d projects, want 0", len(projects))
	}
}
