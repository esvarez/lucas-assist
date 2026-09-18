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

	created, err := repo.CreateProject(ctx, domain.Project{UserID: "user_1", Name: "Nudge"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	got, err := repo.GetProject(ctx, "user_1", created.ID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if !reflect.DeepEqual(got, created) {
		t.Errorf("GetProject() = %+v, want %+v", got, created)
	}
}

func TestMemoryRepository_GetProject_NotFound(t *testing.T) {
	repo := NewMemoryRepository()

	_, err := repo.GetProject(context.Background(), "user_1", "does-not-exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetProject() error = %v, want %v", err, ErrNotFound)
	}
}

func TestMemoryRepository_GetProject_WrongUser(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateProject(ctx, domain.Project{UserID: "user_1", Name: "Nudge"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	_, err = repo.GetProject(ctx, "user_2", created.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetProject() with wrong userID error = %v, want %v", err, ErrNotFound)
	}
}

func TestMemoryRepository_DeleteProject(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateProject(ctx, domain.Project{UserID: "user_1", Name: "Nudge"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if err := repo.DeleteProject(ctx, "user_1", created.ID); err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}

	if _, err := repo.GetProject(ctx, "user_1", created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetProject() after delete error = %v, want %v", err, ErrNotFound)
	}

	projects, err := repo.ListProjects(ctx, "user_1")
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	for _, p := range projects {
		if p.ID == created.ID {
			t.Errorf("ListProjects() still contains deleted project %q", created.ID)
		}
	}
}

func TestMemoryRepository_DeleteProject_NotFound(t *testing.T) {
	repo := NewMemoryRepository()

	err := repo.DeleteProject(context.Background(), "user_1", "does-not-exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteProject() error = %v, want %v", err, ErrNotFound)
	}
}

func TestMemoryRepository_DeleteProject_WrongUser(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateProject(ctx, domain.Project{UserID: "user_1", Name: "Nudge"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if err := repo.DeleteProject(ctx, "user_2", created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteProject() with wrong userID error = %v, want %v", err, ErrNotFound)
	}

	if _, err := repo.GetProject(ctx, "user_1", created.ID); err != nil {
		t.Errorf("GetProject() after failed delete error = %v, want project still present", err)
	}
}

func TestMemoryRepository_UpdateProject(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateProject(ctx, domain.Project{
		UserID:      "user_1",
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
	updated, err := repo.UpdateProject(ctx, "user_1", domain.Project{
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

	got, err := repo.GetProject(ctx, "user_1", created.ID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if !reflect.DeepEqual(got, updated) {
		t.Errorf("GetProject() after update = %+v, want %+v", got, updated)
	}
}

func TestMemoryRepository_UpdateProject_NotFound(t *testing.T) {
	repo := NewMemoryRepository()

	_, err := repo.UpdateProject(context.Background(), "user_1", domain.Project{ID: "does-not-exist"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateProject() error = %v, want %v", err, ErrNotFound)
	}
}

func TestMemoryRepository_UpdateProject_WrongUser(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateProject(ctx, domain.Project{UserID: "user_1", Name: "Nudge"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	_, err = repo.UpdateProject(ctx, "user_2", domain.Project{ID: created.ID, Status: "done"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateProject() with wrong userID error = %v, want %v", err, ErrNotFound)
	}
}

func TestMemoryRepository_ListProjects(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	if _, err := repo.CreateProject(ctx, domain.Project{UserID: "user_1", Name: "First"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := repo.CreateProject(ctx, domain.Project{UserID: "user_1", Name: "Second"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := repo.CreateProject(ctx, domain.Project{UserID: "user_2", Name: "Someone else's"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	projects, err := repo.ListProjects(ctx, "user_1")
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(projects) != 2 {
		t.Errorf("ListProjects() returned %d projects, want 2", len(projects))
	}
	for _, p := range projects {
		if p.UserID != "user_1" {
			t.Errorf("ListProjects(\"user_1\") leaked project owned by %q", p.UserID)
		}
	}
}

func TestMemoryRepository_ListProjects_Empty(t *testing.T) {
	repo := NewMemoryRepository()

	projects, err := repo.ListProjects(context.Background(), "user_1")
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

func TestMemoryRepository_CreateTask(t *testing.T) {
	repo := NewMemoryRepository()

	created, err := repo.CreateTask(context.Background(), "user_1", domain.Task{
		ProjectID:          "proj_1",
		Title:              "Add login command",
		Description:        "Device-flow login for the CLI",
		Status:             "pending",
		AcceptanceCriteria: []string{"running `nudge login` prints a device code"},
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	if created.ID == "" {
		t.Error("ID = \"\", want a generated ID")
	}
	if created.ProjectID != "proj_1" || created.Title != "Add login command" {
		t.Errorf("CreateTask() = %+v, want ProjectID/Title preserved from input", created)
	}
}

func TestMemoryRepository_CreateTask_ExplicitID(t *testing.T) {
	repo := NewMemoryRepository()

	created, err := repo.CreateTask(context.Background(), "user_1", domain.Task{ID: "task_1", ProjectID: "proj_1"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if created.ID != "task_1" {
		t.Errorf("ID = %q, want the caller-supplied ID %q", created.ID, "task_1")
	}
}

func TestMemoryRepository_CreateTask_DuplicateID(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	if _, err := repo.CreateTask(ctx, "user_1", domain.Task{ID: "task_1", ProjectID: "proj_1", Title: "First"}); err != nil {
		t.Fatalf("first CreateTask() error = %v", err)
	}

	_, err := repo.CreateTask(ctx, "user_1", domain.Task{ID: "task_1", ProjectID: "proj_1", Title: "Second"})
	if !errors.Is(err, ErrDuplicateID) {
		t.Fatalf("second CreateTask() error = %v, want %v", err, ErrDuplicateID)
	}
}

func TestMemoryRepository_GetTask_Found(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateTask(ctx, "user_1", domain.Task{ProjectID: "proj_1", Title: "Add login command"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	got, err := repo.GetTask(ctx, "user_1", "proj_1", created.ID)
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}
	if !reflect.DeepEqual(got, created) {
		t.Errorf("GetTask() = %+v, want %+v", got, created)
	}
}

func TestMemoryRepository_GetTask_NotFound(t *testing.T) {
	repo := NewMemoryRepository()

	_, err := repo.GetTask(context.Background(), "user_1", "proj_1", "does-not-exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetTask() error = %v, want %v", err, ErrNotFound)
	}
}

func TestMemoryRepository_GetTask_WrongUser(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateTask(ctx, "user_1", domain.Task{ProjectID: "proj_1", Title: "Add login command"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	_, err = repo.GetTask(ctx, "user_2", "proj_1", created.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetTask() with wrong userID error = %v, want %v", err, ErrNotFound)
	}
}

func TestMemoryRepository_GetTask_WrongProject(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateTask(ctx, "user_1", domain.Task{ProjectID: "proj_1", Title: "Add login command"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	_, err = repo.GetTask(ctx, "user_1", "proj_2", created.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetTask() with wrong projectID error = %v, want %v", err, ErrNotFound)
	}
}

func TestMemoryRepository_ListTasks(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	want := make(map[string]bool)
	for _, title := range []string{"First", "Second"} {
		created, err := repo.CreateTask(ctx, "user_1", domain.Task{ProjectID: "proj_1", Title: title})
		if err != nil {
			t.Fatalf("CreateTask() error = %v", err)
		}
		want[created.ID] = true
	}

	if _, err := repo.CreateTask(ctx, "user_1", domain.Task{ProjectID: "proj_2", Title: "Different project"}); err != nil {
		t.Fatalf("CreateTask() for other project error = %v", err)
	}
	if _, err := repo.CreateTask(ctx, "user_2", domain.Task{ProjectID: "proj_1", Title: "Different user"}); err != nil {
		t.Fatalf("CreateTask() for other user error = %v", err)
	}

	tasks, err := repo.ListTasks(ctx, "user_1", "proj_1")
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(tasks) != len(want) {
		t.Errorf("ListTasks() returned %d tasks, want %d", len(tasks), len(want))
	}
	for _, task := range tasks {
		if !want[task.ID] {
			t.Errorf("ListTasks() returned unexpected task %q", task.ID)
		}
	}
}

func TestMemoryRepository_ListTasks_Empty(t *testing.T) {
	repo := NewMemoryRepository()

	tasks, err := repo.ListTasks(context.Background(), "user_1", "proj_1")
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if tasks == nil {
		t.Error("ListTasks() = nil, want an empty (non-nil) slice")
	}
	if len(tasks) != 0 {
		t.Errorf("ListTasks() returned %d tasks, want 0", len(tasks))
	}
}

func TestMemoryRepository_CreateChangeset(t *testing.T) {
	repo := NewMemoryRepository()

	created, err := repo.CreateChangeset(context.Background(), domain.Changeset{
		ProjectID:   "proj_1",
		UserID:      "user_1",
		Skill:       "decompose_task",
		BaseVersion: 1,
		Status:      domain.ChangesetProposed,
		ProposedTasks: []domain.ProposedTask{
			{Title: "Add login command", Description: "Device-flow login for the CLI"},
		},
	})
	if err != nil {
		t.Fatalf("CreateChangeset() error = %v", err)
	}

	if created.ID == "" {
		t.Error("ID = \"\", want a generated ID")
	}
	if created.ProjectID != "proj_1" || created.Skill != "decompose_task" {
		t.Errorf("CreateChangeset() = %+v, want ProjectID/Skill preserved from input", created)
	}
	if created.Status != domain.ChangesetProposed {
		t.Errorf("Status = %q, want %q", created.Status, domain.ChangesetProposed)
	}
	if created.CreatedAt.IsZero() {
		t.Errorf("CreateChangeset() = %+v, want CreatedAt set", created)
	}
}

func TestMemoryRepository_CreateChangeset_ExplicitID(t *testing.T) {
	repo := NewMemoryRepository()

	created, err := repo.CreateChangeset(context.Background(), domain.Changeset{ID: "cs_1", ProjectID: "proj_1", UserID: "user_1"})
	if err != nil {
		t.Fatalf("CreateChangeset() error = %v", err)
	}
	if created.ID != "cs_1" {
		t.Errorf("ID = %q, want the caller-supplied ID %q", created.ID, "cs_1")
	}
}

func TestMemoryRepository_CreateChangeset_DuplicateID(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	if _, err := repo.CreateChangeset(ctx, domain.Changeset{ID: "cs_1", ProjectID: "proj_1", UserID: "user_1"}); err != nil {
		t.Fatalf("first CreateChangeset() error = %v", err)
	}

	_, err := repo.CreateChangeset(ctx, domain.Changeset{ID: "cs_1", ProjectID: "proj_1", UserID: "user_1"})
	if !errors.Is(err, ErrDuplicateID) {
		t.Fatalf("second CreateChangeset() error = %v, want %v", err, ErrDuplicateID)
	}
}

func TestMemoryRepository_GetChangeset_Found(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateChangeset(ctx, domain.Changeset{ProjectID: "proj_1", UserID: "user_1", Skill: "decompose_task"})
	if err != nil {
		t.Fatalf("CreateChangeset() error = %v", err)
	}

	got, err := repo.GetChangeset(ctx, "user_1", "proj_1", created.ID)
	if err != nil {
		t.Fatalf("GetChangeset() error = %v", err)
	}
	if !reflect.DeepEqual(got, created) {
		t.Errorf("GetChangeset() = %+v, want %+v", got, created)
	}
}

func TestMemoryRepository_GetChangeset_NotFound(t *testing.T) {
	repo := NewMemoryRepository()

	_, err := repo.GetChangeset(context.Background(), "user_1", "proj_1", "does-not-exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetChangeset() error = %v, want %v", err, ErrNotFound)
	}
}

func TestMemoryRepository_GetChangeset_WrongUser(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateChangeset(ctx, domain.Changeset{ProjectID: "proj_1", UserID: "user_1"})
	if err != nil {
		t.Fatalf("CreateChangeset() error = %v", err)
	}

	_, err = repo.GetChangeset(ctx, "user_2", "proj_1", created.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetChangeset() with wrong userID error = %v, want %v", err, ErrNotFound)
	}
}

func TestMemoryRepository_GetChangeset_WrongProject(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateChangeset(ctx, domain.Changeset{ProjectID: "proj_1", UserID: "user_1"})
	if err != nil {
		t.Fatalf("CreateChangeset() error = %v", err)
	}

	_, err = repo.GetChangeset(ctx, "user_1", "proj_2", created.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetChangeset() with wrong projectID error = %v, want %v", err, ErrNotFound)
	}
}

func TestMemoryRepository_UpdateChangesetStatus(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateChangeset(ctx, domain.Changeset{ProjectID: "proj_1", UserID: "user_1", Status: domain.ChangesetProposed})
	if err != nil {
		t.Fatalf("CreateChangeset() error = %v", err)
	}

	updated, err := repo.UpdateChangesetStatus(ctx, "user_1", "proj_1", created.ID, domain.ChangesetAccepted)
	if err != nil {
		t.Fatalf("UpdateChangesetStatus() error = %v", err)
	}
	if updated.Status != domain.ChangesetAccepted {
		t.Errorf("Status = %q, want %q", updated.Status, domain.ChangesetAccepted)
	}

	got, err := repo.GetChangeset(ctx, "user_1", "proj_1", created.ID)
	if err != nil {
		t.Fatalf("GetChangeset() error = %v", err)
	}
	if got.Status != domain.ChangesetAccepted {
		t.Errorf("GetChangeset() after update Status = %q, want %q", got.Status, domain.ChangesetAccepted)
	}
}

func TestMemoryRepository_UpdateChangesetStatus_NotFound(t *testing.T) {
	repo := NewMemoryRepository()

	_, err := repo.UpdateChangesetStatus(context.Background(), "user_1", "proj_1", "does-not-exist", domain.ChangesetAccepted)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateChangesetStatus() error = %v, want %v", err, ErrNotFound)
	}
}

func TestMemoryRepository_UpdateChangesetStatus_WrongUser(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateChangeset(ctx, domain.Changeset{ProjectID: "proj_1", UserID: "user_1", Status: domain.ChangesetProposed})
	if err != nil {
		t.Fatalf("CreateChangeset() error = %v", err)
	}

	_, err = repo.UpdateChangesetStatus(ctx, "user_2", "proj_1", created.ID, domain.ChangesetAccepted)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateChangesetStatus() with wrong userID error = %v, want %v", err, ErrNotFound)
	}
}
