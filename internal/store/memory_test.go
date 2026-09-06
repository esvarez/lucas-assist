package store

import (
	"context"
	"errors"
	"testing"

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
