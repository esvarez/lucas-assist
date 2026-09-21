package store

import (
	"context"
	"encoding/json"
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

	if created.Version != 1 {
		t.Errorf("Version = %d, want 1 on create", created.Version)
	}

	deadline := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	updated, err := repo.UpdateProject(ctx, "user_1", domain.Project{
		ID:          created.ID,
		Name:        "should be ignored",
		Goal:        "Ship v2",
		Deadline:    &deadline,
		Constraints: []string{"no VPC", "no SSR"},
		Status:      "done",
		Version:     created.Version,
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
	if updated.Version != created.Version+1 {
		t.Errorf("Version = %d, want %d (incremented)", updated.Version, created.Version+1)
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

// TestMemoryRepository_UpdateProject_VersionConflict documents the
// optimistic-concurrency path (architecture.md §8): a stale Version is
// rejected with ErrConflict rather than silently applied or retried.
func TestMemoryRepository_UpdateProject_VersionConflict(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateProject(ctx, domain.Project{UserID: "user_1", Name: "Nudge", Goal: "v1"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if _, err := repo.UpdateProject(ctx, "user_1", domain.Project{ID: created.ID, Goal: "v2", Version: created.Version}); err != nil {
		t.Fatalf("first UpdateProject() error = %v", err)
	}

	// Retrying with the now-stale original version must not apply.
	_, err = repo.UpdateProject(ctx, "user_1", domain.Project{ID: created.ID, Goal: "v3 (stale)", Version: created.Version})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("UpdateProject() with stale version error = %v, want %v", err, ErrConflict)
	}

	got, err := repo.GetProject(ctx, "user_1", created.ID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if got.Goal != "v2" {
		t.Errorf("Goal = %q, want %q (the conflicting write must not have applied)", got.Goal, "v2")
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

	_, err = repo.UpdateProject(ctx, "user_2", domain.Project{ID: created.ID, Status: "done", Version: created.Version})
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

func TestMemoryRepository_CreateAgentRun(t *testing.T) {
	repo := NewMemoryRepository()

	created, err := repo.CreateAgentRun(context.Background(), domain.AgentRun{
		UserID:    "user_1",
		ProjectID: "proj_1",
		Skill:     "decompose_task",
		Input:     json.RawMessage(`{"task_id":"task_1"}`),
	})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	if created.ID == "" {
		t.Error("ID = \"\", want a generated ID")
	}
	if created.Status != domain.AgentRunQueued {
		t.Errorf("Status = %q, want %q", created.Status, domain.AgentRunQueued)
	}
	if created.Attempt != 0 {
		t.Errorf("Attempt = %d, want 0 on create", created.Attempt)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Errorf("CreateAgentRun() = %+v, want CreatedAt/UpdatedAt set", created)
	}
}

// TestMemoryRepository_CreateAgentRun_EmptyProjectID documents the
// ProjectID-empty scheme for create_project runs (architecture.md §3, issue
// #96): CreateAgentRun accepts and preserves an empty ProjectID rather than
// generating one or rejecting the run.
func TestMemoryRepository_CreateAgentRun_EmptyProjectID(t *testing.T) {
	repo := NewMemoryRepository()

	created, err := repo.CreateAgentRun(context.Background(), domain.AgentRun{UserID: "user_1", Skill: "create_project"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}
	if created.ProjectID != "" {
		t.Errorf("ProjectID = %q, want empty for a create_project run", created.ProjectID)
	}

	got, err := repo.GetAgentRun(context.Background(), "user_1", "", created.ID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if !reflect.DeepEqual(got, created) {
		t.Errorf("GetAgentRun() = %+v, want %+v", got, created)
	}
}

func TestMemoryRepository_CreateAgentRun_IgnoresCallerStatus(t *testing.T) {
	repo := NewMemoryRepository()

	created, err := repo.CreateAgentRun(context.Background(), domain.AgentRun{
		UserID:    "user_1",
		ProjectID: "proj_1",
		Status:    domain.AgentRunCompleted,
		Attempt:   5,
	})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}
	if created.Status != domain.AgentRunQueued {
		t.Errorf("Status = %q, want %q regardless of caller input", created.Status, domain.AgentRunQueued)
	}
	if created.Attempt != 0 {
		t.Errorf("Attempt = %d, want 0 regardless of caller input", created.Attempt)
	}
}

func TestMemoryRepository_GetAgentRun_NotFound(t *testing.T) {
	repo := NewMemoryRepository()

	_, err := repo.GetAgentRun(context.Background(), "user_1", "proj_1", "does-not-exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetAgentRun() error = %v, want %v", err, ErrNotFound)
	}
}

func TestMemoryRepository_GetAgentRun_WrongUser(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: "user_1", ProjectID: "proj_1"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	_, err = repo.GetAgentRun(ctx, "user_2", "proj_1", created.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetAgentRun() with wrong userID error = %v, want %v", err, ErrNotFound)
	}
}

func TestMemoryRepository_GetAgentRun_WrongProject(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: "user_1", ProjectID: "proj_1"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	_, err = repo.GetAgentRun(ctx, "user_1", "proj_2", created.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetAgentRun() with wrong projectID error = %v, want %v", err, ErrNotFound)
	}
}

func TestMemoryRepository_LeaseAgentRun(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: "user_1", ProjectID: "proj_1"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	leaseUntil := time.Now().UTC().Add(time.Minute)
	leased, err := repo.LeaseAgentRun(ctx, "user_1", "proj_1", created.ID, "worker_1", leaseUntil)
	if err != nil {
		t.Fatalf("LeaseAgentRun() error = %v", err)
	}

	if leased.Status != domain.AgentRunRunning {
		t.Errorf("Status = %q, want %q", leased.Status, domain.AgentRunRunning)
	}
	if leased.WorkerID != "worker_1" {
		t.Errorf("WorkerID = %q, want %q", leased.WorkerID, "worker_1")
	}
	if leased.LeaseUntil == nil || !leased.LeaseUntil.Equal(leaseUntil) {
		t.Errorf("LeaseUntil = %v, want %v", leased.LeaseUntil, leaseUntil)
	}
	if leased.Attempt != 1 {
		t.Errorf("Attempt = %d, want 1 after first lease", leased.Attempt)
	}
}

// TestMemoryRepository_LeaseAgentRun_Race documents the lease race
// (architecture.md §15, issue #96 acceptance criteria): two lease attempts
// on the same queued run must not both succeed.
func TestMemoryRepository_LeaseAgentRun_Race(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: "user_1", ProjectID: "proj_1"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	leaseUntil := time.Now().UTC().Add(time.Minute)

	if _, err := repo.LeaseAgentRun(ctx, "user_1", "proj_1", created.ID, "worker_1", leaseUntil); err != nil {
		t.Fatalf("first LeaseAgentRun() error = %v", err)
	}

	_, err = repo.LeaseAgentRun(ctx, "user_1", "proj_1", created.ID, "worker_2", leaseUntil)
	if !errors.Is(err, ErrRunLeased) {
		t.Fatalf("second LeaseAgentRun() error = %v, want %v", err, ErrRunLeased)
	}

	got, err := repo.GetAgentRun(ctx, "user_1", "proj_1", created.ID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if got.WorkerID != "worker_1" {
		t.Errorf("WorkerID = %q, want %q (the losing lease attempt must not have applied)", got.WorkerID, "worker_1")
	}
	if got.Attempt != 1 {
		t.Errorf("Attempt = %d, want 1 (only the winning attempt counts)", got.Attempt)
	}
}

// TestMemoryRepository_LeaseAgentRun_ReclaimExpired documents reclaiming an
// expired lease (architecture.md §15, issue #96 acceptance criteria): a
// worker that never completed its attempt before LeaseUntil passed must not
// block a later attempt from leasing the run.
func TestMemoryRepository_LeaseAgentRun_ReclaimExpired(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: "user_1", ProjectID: "proj_1"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	expiredLease := time.Now().UTC().Add(-time.Minute)
	if _, err := repo.LeaseAgentRun(ctx, "user_1", "proj_1", created.ID, "worker_1", expiredLease); err != nil {
		t.Fatalf("first LeaseAgentRun() error = %v", err)
	}

	newLease := time.Now().UTC().Add(time.Minute)
	reclaimed, err := repo.LeaseAgentRun(ctx, "user_1", "proj_1", created.ID, "worker_2", newLease)
	if err != nil {
		t.Fatalf("LeaseAgentRun() reclaiming expired lease error = %v", err)
	}

	if reclaimed.WorkerID != "worker_2" {
		t.Errorf("WorkerID = %q, want %q after reclaiming", reclaimed.WorkerID, "worker_2")
	}
	if reclaimed.Attempt != 2 {
		t.Errorf("Attempt = %d, want 2 after reclaiming", reclaimed.Attempt)
	}
}

func TestMemoryRepository_LeaseAgentRun_NotFound(t *testing.T) {
	repo := NewMemoryRepository()

	_, err := repo.LeaseAgentRun(context.Background(), "user_1", "proj_1", "does-not-exist", "worker_1", time.Now().Add(time.Minute))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("LeaseAgentRun() error = %v, want %v", err, ErrNotFound)
	}
}

func TestMemoryRepository_LeaseAgentRun_TerminalRunNotLeasable(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: "user_1", ProjectID: "proj_1"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}
	if _, err := repo.CompleteAgentRun(ctx, "user_1", "proj_1", created.ID); err != nil {
		t.Fatalf("CompleteAgentRun() error = %v", err)
	}

	_, err = repo.LeaseAgentRun(ctx, "user_1", "proj_1", created.ID, "worker_1", time.Now().Add(time.Minute))
	if !errors.Is(err, ErrRunLeased) {
		t.Fatalf("LeaseAgentRun() on completed run error = %v, want %v", err, ErrRunLeased)
	}
}

func TestMemoryRepository_CompleteAgentRun(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: "user_1", ProjectID: "proj_1"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}
	if _, err := repo.LeaseAgentRun(ctx, "user_1", "proj_1", created.ID, "worker_1", time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("LeaseAgentRun() error = %v", err)
	}

	completed, err := repo.CompleteAgentRun(ctx, "user_1", "proj_1", created.ID)
	if err != nil {
		t.Fatalf("CompleteAgentRun() error = %v", err)
	}
	if completed.Status != domain.AgentRunCompleted {
		t.Errorf("Status = %q, want %q", completed.Status, domain.AgentRunCompleted)
	}
}

func TestMemoryRepository_CompleteAgentRun_NotFound(t *testing.T) {
	repo := NewMemoryRepository()

	_, err := repo.CompleteAgentRun(context.Background(), "user_1", "proj_1", "does-not-exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("CompleteAgentRun() error = %v, want %v", err, ErrNotFound)
	}
}

func TestMemoryRepository_FailAgentRun(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: "user_1", ProjectID: "proj_1"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}
	if _, err := repo.LeaseAgentRun(ctx, "user_1", "proj_1", created.ID, "worker_1", time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("LeaseAgentRun() error = %v", err)
	}

	failed, err := repo.FailAgentRun(ctx, "user_1", "proj_1", created.ID, "provider timeout")
	if err != nil {
		t.Fatalf("FailAgentRun() error = %v", err)
	}
	if failed.Status != domain.AgentRunFailed {
		t.Errorf("Status = %q, want %q", failed.Status, domain.AgentRunFailed)
	}
	if failed.Error != "provider timeout" {
		t.Errorf("Error = %q, want %q", failed.Error, "provider timeout")
	}
}

func TestMemoryRepository_FailAgentRun_NotFound(t *testing.T) {
	repo := NewMemoryRepository()

	_, err := repo.FailAgentRun(context.Background(), "user_1", "proj_1", "does-not-exist", "boom")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("FailAgentRun() error = %v, want %v", err, ErrNotFound)
	}
}

// newAcceptableChangeset creates a project and a proposed changeset against
// it, ready to accept — shared setup for the AcceptChangeset tests below.
func newAcceptableChangeset(t *testing.T, repo *MemoryRepository, userID string, proposedTasks []domain.ProposedTask) (domain.Project, domain.Changeset) {
	t.Helper()
	ctx := context.Background()

	project, err := repo.CreateProject(ctx, domain.Project{UserID: userID, Name: "Nudge", Goal: "Ship the POC"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	changeset, err := repo.CreateChangeset(ctx, domain.Changeset{
		ProjectID:     project.ID,
		UserID:        userID,
		Skill:         "decompose_task",
		BaseVersion:   project.Version,
		Status:        domain.ChangesetProposed,
		ProposedTasks: proposedTasks,
	})
	if err != nil {
		t.Fatalf("CreateChangeset() error = %v", err)
	}

	return project, changeset
}

func TestMemoryRepository_AcceptChangeset(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	project, changeset := newAcceptableChangeset(t, repo, "user_1", []domain.ProposedTask{
		{Title: "First", Description: "Do the first thing", AcceptanceCriteria: []string{"it's done"}},
		{Title: "Second", Description: "Do the second thing"},
	})

	result, err := repo.AcceptChangeset(ctx, project, changeset, "idem-key-1")
	if err != nil {
		t.Fatalf("AcceptChangeset() error = %v", err)
	}

	if result.Project.Version != project.Version+1 {
		t.Errorf("Project.Version = %d, want %d (incremented)", result.Project.Version, project.Version+1)
	}
	if result.Project.Name != project.Name {
		t.Errorf("Project.Name = %q, want unchanged %q", result.Project.Name, project.Name)
	}

	if len(result.Tasks) != 2 {
		t.Fatalf("len(Tasks) = %d, want 2", len(result.Tasks))
	}
	for i, task := range result.Tasks {
		if task.ID == "" {
			t.Errorf("Tasks[%d].ID = \"\", want a generated ID", i)
		}
		if task.ProjectID != project.ID {
			t.Errorf("Tasks[%d].ProjectID = %q, want %q", i, task.ProjectID, project.ID)
		}
		if task.Order != i {
			t.Errorf("Tasks[%d].Order = %d, want %d", i, task.Order, i)
		}
		if task.Status == "" {
			t.Errorf("Tasks[%d].Status = \"\", want a default status", i)
		}
	}
	if result.Tasks[0].Title != "First" || result.Tasks[1].Title != "Second" {
		t.Errorf("Tasks = %+v, want titles preserved in order", result.Tasks)
	}
	if !reflect.DeepEqual(result.Tasks[0].AcceptanceCriteria, []string{"it's done"}) {
		t.Errorf("Tasks[0].AcceptanceCriteria = %v, want preserved from the proposal", result.Tasks[0].AcceptanceCriteria)
	}

	if result.Event.ID == "" {
		t.Error("Event.ID = \"\", want a generated ID")
	}
	if result.Event.ProjectID != project.ID || result.Event.ChangesetID != changeset.ID {
		t.Errorf("Event = %+v, want ProjectID/ChangesetID matching the accepted changeset", result.Event)
	}
	if result.Event.Type != domain.EventChangesetAccepted {
		t.Errorf("Event.Type = %q, want %q", result.Event.Type, domain.EventChangesetAccepted)
	}
	if result.Event.BaseVersion != changeset.BaseVersion {
		t.Errorf("Event.BaseVersion = %d, want %d", result.Event.BaseVersion, changeset.BaseVersion)
	}

	gotProject, err := repo.GetProject(ctx, "user_1", project.ID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if gotProject.Version != project.Version+1 {
		t.Errorf("stored Project.Version = %d, want %d", gotProject.Version, project.Version+1)
	}

	for _, task := range result.Tasks {
		if _, err := repo.GetTask(ctx, "user_1", project.ID, task.ID); err != nil {
			t.Errorf("GetTask(%q) error = %v, want the task to be persisted", task.ID, err)
		}
	}

	gotChangeset, err := repo.GetChangeset(ctx, "user_1", project.ID, changeset.ID)
	if err != nil {
		t.Fatalf("GetChangeset() error = %v", err)
	}
	if gotChangeset.Status != domain.ChangesetApplied {
		t.Errorf("Changeset.Status = %q, want %q", gotChangeset.Status, domain.ChangesetApplied)
	}
}

// TestMemoryRepository_AcceptChangeset_VersionConflict documents the
// architecture.md §8 optimistic-concurrency path for accept: a changeset
// whose baseVersion no longer matches the project's stored version is
// rejected with ErrConflict, and the changeset moves to "conflict" — not
// retried automatically (AGENTS.MD).
func TestMemoryRepository_AcceptChangeset_VersionConflict(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	project, changeset := newAcceptableChangeset(t, repo, "user_1", []domain.ProposedTask{{Title: "First"}})

	// Someone else's edit lands first, moving the project's real version
	// past the changeset's recorded baseVersion.
	if _, err := repo.UpdateProject(ctx, "user_1", domain.Project{ID: project.ID, Goal: "changed", Version: project.Version}); err != nil {
		t.Fatalf("UpdateProject() error = %v", err)
	}

	_, err := repo.AcceptChangeset(ctx, project, changeset, "idem-key-1")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("AcceptChangeset() error = %v, want %v", err, ErrConflict)
	}

	gotChangeset, err := repo.GetChangeset(ctx, "user_1", project.ID, changeset.ID)
	if err != nil {
		t.Fatalf("GetChangeset() error = %v", err)
	}
	if gotChangeset.Status != domain.ChangesetConflict {
		t.Errorf("Changeset.Status = %q, want %q", gotChangeset.Status, domain.ChangesetConflict)
	}

	tasks, err := repo.ListTasks(ctx, "user_1", project.ID)
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("ListTasks() = %+v, want no tasks created on a conflicting accept", tasks)
	}
}

// TestMemoryRepository_AcceptChangeset_IdempotentReplay documents
// architecture.md §15: replaying an accept with the same idempotency key
// returns the original result instead of re-applying it (no duplicate
// tasks, no second version bump).
func TestMemoryRepository_AcceptChangeset_IdempotentReplay(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	project, changeset := newAcceptableChangeset(t, repo, "user_1", []domain.ProposedTask{{Title: "First"}})

	first, err := repo.AcceptChangeset(ctx, project, changeset, "idem-key-1")
	if err != nil {
		t.Fatalf("first AcceptChangeset() error = %v", err)
	}

	second, err := repo.AcceptChangeset(ctx, project, changeset, "idem-key-1")
	if err != nil {
		t.Fatalf("second AcceptChangeset() error = %v", err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Errorf("second AcceptChangeset() = %+v, want the identical cached result %+v", second, first)
	}

	tasks, err := repo.ListTasks(ctx, "user_1", project.ID)
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("ListTasks() returned %d tasks, want 1 (replay must not re-apply)", len(tasks))
	}

	gotProject, err := repo.GetProject(ctx, "user_1", project.ID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if gotProject.Version != project.Version+1 {
		t.Errorf("Project.Version = %d, want %d (replay must not bump it twice)", gotProject.Version, project.Version+1)
	}
}

// TestMemoryRepository_AcceptChangeset_IdempotencyKeyReused documents the
// other half of architecture.md §15: reusing an idempotency key against a
// different request is rejected, not silently answered with the first
// request's result.
func TestMemoryRepository_AcceptChangeset_IdempotencyKeyReused(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	project, changeset := newAcceptableChangeset(t, repo, "user_1", []domain.ProposedTask{{Title: "First"}})
	if _, err := repo.AcceptChangeset(ctx, project, changeset, "idem-key-1"); err != nil {
		t.Fatalf("first AcceptChangeset() error = %v", err)
	}

	_, otherChangeset := newAcceptableChangeset(t, repo, "user_1", []domain.ProposedTask{{Title: "Unrelated"}})

	_, err := repo.AcceptChangeset(ctx, project, otherChangeset, "idem-key-1")
	if !errors.Is(err, ErrIdempotencyKeyReused) {
		t.Fatalf("AcceptChangeset() with reused key error = %v, want %v", err, ErrIdempotencyKeyReused)
	}
}

// TestMemoryRepository_AcceptChangeset_NotProposed documents that a
// changeset in any status other than "proposed" can't be accepted with a
// fresh idempotency key — distinct from the idempotent-replay case (same
// key, already applied), which must still succeed.
func TestMemoryRepository_AcceptChangeset_NotProposed(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()

	project, changeset := newAcceptableChangeset(t, repo, "user_1", []domain.ProposedTask{{Title: "First"}})
	changeset, err := repo.UpdateChangesetStatus(ctx, "user_1", project.ID, changeset.ID, domain.ChangesetRejected)
	if err != nil {
		t.Fatalf("UpdateChangesetStatus() error = %v", err)
	}

	_, err = repo.AcceptChangeset(ctx, project, changeset, "idem-key-1")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("AcceptChangeset() on a rejected changeset error = %v, want %v", err, ErrConflict)
	}

	gotChangeset, err := repo.GetChangeset(ctx, "user_1", project.ID, changeset.ID)
	if err != nil {
		t.Fatalf("GetChangeset() error = %v", err)
	}
	if gotChangeset.Status != domain.ChangesetRejected {
		t.Errorf("Changeset.Status = %q, want unchanged %q", gotChangeset.Status, domain.ChangesetRejected)
	}

	tasks, err := repo.ListTasks(ctx, "user_1", project.ID)
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("ListTasks() = %+v, want no tasks created", tasks)
	}
}

func TestMemoryRepository_AcceptChangeset_ProjectNotFound(t *testing.T) {
	repo := NewMemoryRepository()

	_, err := repo.AcceptChangeset(context.Background(),
		domain.Project{ID: "does-not-exist"},
		domain.Changeset{ID: "cs_1", ProjectID: "does-not-exist", UserID: "user_1", BaseVersion: 1},
		"idem-key-1",
	)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("AcceptChangeset() error = %v, want %v", err, ErrNotFound)
	}
}
