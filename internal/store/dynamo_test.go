//go:build integration

package store

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/esvarez/lucas-assist/internal/domain"
)

// Runs against DynamoDB Local (see docker-compose.yml). `make
// test-integration` starts it and sets DYNAMODB_ENDPOINT.
const testTable = "nudge-integration-test"

func newTestDynamoRepository(t *testing.T) *DynamoRepository {
	t.Helper()

	endpoint := os.Getenv("DYNAMODB_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}

	ctx := context.Background()
	client, err := NewLocalDynamoDBClient(ctx, endpoint)
	if err != nil {
		t.Fatalf("NewDynamoDBClient() error = %v", err)
	}

	if err := EnsureTable(ctx, client, testTable); err != nil {
		t.Fatalf("EnsureTable() error = %v", err)
	}

	return NewDynamoRepository(client, testTable)
}

// testUserID returns a userID unique to this test run. Since the table is
// partitioned by user, a fresh userID is on its own empty partition even
// against the shared testTable, which persists across runs of a long-lived
// DynamoDB Local instance.
func testUserID() string {
	return "user-" + domain.NewID()
}

func TestDynamoRepository_CreateProject(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()

	created, err := repo.CreateProject(ctx, domain.Project{
		UserID:      userID,
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

	got, err := repo.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(testTable),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: userPK(userID)},
			"SK": &types.AttributeValueMemberS{Value: projectSK(created.ID)},
		},
	})
	if err != nil {
		t.Fatalf("GetItem() error = %v", err)
	}
	if got.Item == nil {
		t.Fatalf("GetItem() found no item for project %q", created.ID)
	}
}

func TestDynamoRepository_CreateProject_DuplicateID(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()

	id := "dup-" + domain.NewID()

	if _, err := repo.CreateProject(ctx, domain.Project{UserID: userID, ID: id, Name: "First"}); err != nil {
		t.Fatalf("first CreateProject() error = %v", err)
	}

	_, err := repo.CreateProject(ctx, domain.Project{UserID: userID, ID: id, Name: "Second"})
	if !errors.Is(err, ErrDuplicateID) {
		t.Fatalf("second CreateProject() error = %v, want %v", err, ErrDuplicateID)
	}
}

func TestDynamoRepository_GetProject(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()

	created, err := repo.CreateProject(ctx, domain.Project{
		UserID:      userID,
		Name:        "Nudge",
		Goal:        "Ship the POC",
		Constraints: []string{"no VPC", "no SSR"},
		Status:      "active",
	})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	got, err := repo.GetProject(ctx, userID, created.ID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if !reflect.DeepEqual(got, created) {
		t.Errorf("GetProject() = %+v, want %+v", got, created)
	}
}

func TestDynamoRepository_GetProject_NotFound(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()

	_, err := repo.GetProject(ctx, testUserID(), "missing-"+domain.NewID())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetProject() error = %v, want %v", err, ErrNotFound)
	}
}

func TestDynamoRepository_GetProject_WrongUser(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()

	created, err := repo.CreateProject(ctx, domain.Project{UserID: testUserID(), Name: "Nudge"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	_, err = repo.GetProject(ctx, testUserID(), created.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetProject() with wrong userID error = %v, want %v", err, ErrNotFound)
	}
}

func TestDynamoRepository_ListProjects(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()
	otherUserID := testUserID()

	want := make(map[string]bool)
	for _, name := range []string{"Nudge", "Widget"} {
		created, err := repo.CreateProject(ctx, domain.Project{UserID: userID, Name: name})
		if err != nil {
			t.Fatalf("CreateProject() error = %v", err)
		}
		want[created.ID] = true
	}

	if _, err := repo.CreateProject(ctx, domain.Project{UserID: otherUserID, Name: "Someone else's"}); err != nil {
		t.Fatalf("CreateProject() for other user error = %v", err)
	}

	got, err := repo.ListProjects(ctx, userID)
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}

	if len(got) != len(want) {
		t.Errorf("ListProjects() returned %d projects, want %d", len(got), len(want))
	}
	for _, p := range got {
		if !want[p.ID] {
			t.Errorf("ListProjects(%q) returned unexpected project %q (userID %q)", userID, p.ID, p.UserID)
		}
		if p.UserID != userID {
			t.Errorf("ListProjects(%q) leaked project owned by %q", userID, p.UserID)
		}
	}
}

func TestDynamoRepository_UpdateProject(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()

	created, err := repo.CreateProject(ctx, domain.Project{
		UserID:      userID,
		Name:        "Nudge",
		Goal:        "Ship the POC",
		Constraints: []string{"no VPC"},
		Status:      "active",
	})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if created.Version != 1 {
		t.Errorf("Version = %d, want 1 on create", created.Version)
	}

	deadline := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	updated, err := repo.UpdateProject(ctx, userID, domain.Project{
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

	got, err := repo.GetProject(ctx, userID, created.ID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if !reflect.DeepEqual(got, updated) {
		t.Errorf("GetProject() after update = %+v, want %+v", got, updated)
	}
}

// TestDynamoRepository_UpdateProject_VersionConflict documents the
// optimistic-concurrency path (architecture.md §8): a stale Version is
// rejected with ErrConflict rather than silently applied or retried.
func TestDynamoRepository_UpdateProject_VersionConflict(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()

	created, err := repo.CreateProject(ctx, domain.Project{UserID: userID, Name: "Nudge", Goal: "v1"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if _, err := repo.UpdateProject(ctx, userID, domain.Project{ID: created.ID, Goal: "v2", Version: created.Version}); err != nil {
		t.Fatalf("first UpdateProject() error = %v", err)
	}

	// Retrying with the now-stale original version must not apply.
	_, err = repo.UpdateProject(ctx, userID, domain.Project{ID: created.ID, Goal: "v3 (stale)", Version: created.Version})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("UpdateProject() with stale version error = %v, want %v", err, ErrConflict)
	}

	got, err := repo.GetProject(ctx, userID, created.ID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if got.Goal != "v2" {
		t.Errorf("Goal = %q, want %q (the conflicting write must not have applied)", got.Goal, "v2")
	}
}

func TestDynamoRepository_UpdateProject_ClearsDeadline(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()

	deadline := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	created, err := repo.CreateProject(ctx, domain.Project{UserID: userID, Name: "Nudge", Deadline: &deadline})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if created.Deadline == nil {
		t.Fatalf("CreateProject() = %+v, want Deadline set", created)
	}

	updated, err := repo.UpdateProject(ctx, userID, domain.Project{ID: created.ID, Status: "active", Version: created.Version})
	if err != nil {
		t.Fatalf("UpdateProject() error = %v", err)
	}
	if updated.Deadline != nil {
		t.Errorf("Deadline = %v, want nil after clearing", updated.Deadline)
	}

	got, err := repo.GetProject(ctx, userID, created.ID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if got.Deadline != nil {
		t.Errorf("GetProject() after update Deadline = %v, want nil", got.Deadline)
	}
}

func TestDynamoRepository_UpdateProject_NotFound(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()

	_, err := repo.UpdateProject(ctx, testUserID(), domain.Project{ID: "missing-" + domain.NewID()})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateProject() error = %v, want %v", err, ErrNotFound)
	}
}

func TestDynamoRepository_UpdateProject_WrongUser(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()

	created, err := repo.CreateProject(ctx, domain.Project{UserID: testUserID(), Name: "Nudge"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	_, err = repo.UpdateProject(ctx, testUserID(), domain.Project{ID: created.ID, Status: "done", Version: created.Version})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateProject() with wrong userID error = %v, want %v", err, ErrNotFound)
	}
}

func TestDynamoRepository_DeleteProject(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()

	created, err := repo.CreateProject(ctx, domain.Project{UserID: userID, Name: "Nudge"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if err := repo.DeleteProject(ctx, userID, created.ID); err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}

	if _, err := repo.GetProject(ctx, userID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetProject() after delete error = %v, want %v", err, ErrNotFound)
	}
}

func TestDynamoRepository_DeleteProject_NotFound(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()

	err := repo.DeleteProject(ctx, testUserID(), "missing-"+domain.NewID())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteProject() error = %v, want %v", err, ErrNotFound)
	}
}

func TestDynamoRepository_DeleteProject_WrongUser(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()

	created, err := repo.CreateProject(ctx, domain.Project{UserID: userID, Name: "Nudge"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if err := repo.DeleteProject(ctx, testUserID(), created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteProject() with wrong userID error = %v, want %v", err, ErrNotFound)
	}

	if _, err := repo.GetProject(ctx, userID, created.ID); err != nil {
		t.Errorf("GetProject() after failed delete error = %v, want project still present", err)
	}
}

func TestDynamoRepository_ListProjects_Empty(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()

	got, err := repo.ListProjects(ctx, testUserID())
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListProjects() = %+v, want empty", got)
	}
}

func TestDynamoRepository_CreateTask(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()
	projectID := "proj-" + domain.NewID()

	created, err := repo.CreateTask(ctx, userID, domain.Task{
		ProjectID:          projectID,
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
	if created.ProjectID != projectID || created.Title != "Add login command" {
		t.Errorf("CreateTask() = %+v, want ProjectID/Title preserved from input", created)
	}

	got, err := repo.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(testTable),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: userPK(userID)},
			"SK": &types.AttributeValueMemberS{Value: taskSK(projectID, created.ID)},
		},
	})
	if err != nil {
		t.Fatalf("GetItem() error = %v", err)
	}
	if got.Item == nil {
		t.Fatalf("GetItem() found no item for task %q", created.ID)
	}
}

func TestDynamoRepository_CreateTask_DuplicateID(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()
	projectID := "proj-" + domain.NewID()

	id := "dup-" + domain.NewID()

	if _, err := repo.CreateTask(ctx, userID, domain.Task{ID: id, ProjectID: projectID, Title: "First"}); err != nil {
		t.Fatalf("first CreateTask() error = %v", err)
	}

	_, err := repo.CreateTask(ctx, userID, domain.Task{ID: id, ProjectID: projectID, Title: "Second"})
	if !errors.Is(err, ErrDuplicateID) {
		t.Fatalf("second CreateTask() error = %v, want %v", err, ErrDuplicateID)
	}
}

func TestDynamoRepository_GetTask(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()
	projectID := "proj-" + domain.NewID()

	created, err := repo.CreateTask(ctx, userID, domain.Task{
		ProjectID:          projectID,
		Title:              "Add login command",
		AcceptanceCriteria: []string{"running `nudge login` prints a device code"},
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	got, err := repo.GetTask(ctx, userID, projectID, created.ID)
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}
	if !reflect.DeepEqual(got, created) {
		t.Errorf("GetTask() = %+v, want %+v", got, created)
	}
}

func TestDynamoRepository_GetTask_NotFound(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()

	_, err := repo.GetTask(ctx, testUserID(), "proj-"+domain.NewID(), "missing-"+domain.NewID())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetTask() error = %v, want %v", err, ErrNotFound)
	}
}

func TestDynamoRepository_GetTask_WrongUser(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	projectID := "proj-" + domain.NewID()

	created, err := repo.CreateTask(ctx, testUserID(), domain.Task{ProjectID: projectID, Title: "Add login command"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	_, err = repo.GetTask(ctx, testUserID(), projectID, created.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetTask() with wrong userID error = %v, want %v", err, ErrNotFound)
	}
}

func TestDynamoRepository_GetTask_WrongProject(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()

	created, err := repo.CreateTask(ctx, userID, domain.Task{ProjectID: "proj-" + domain.NewID(), Title: "Add login command"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	_, err = repo.GetTask(ctx, userID, "proj-"+domain.NewID(), created.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetTask() with wrong projectID error = %v, want %v", err, ErrNotFound)
	}
}

func TestDynamoRepository_ListTasks(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()
	projectID := "proj-" + domain.NewID()
	otherProjectID := "proj-" + domain.NewID()

	want := make(map[string]bool)
	for _, title := range []string{"First", "Second"} {
		created, err := repo.CreateTask(ctx, userID, domain.Task{ProjectID: projectID, Title: title})
		if err != nil {
			t.Fatalf("CreateTask() error = %v", err)
		}
		want[created.ID] = true
	}

	if _, err := repo.CreateTask(ctx, userID, domain.Task{ProjectID: otherProjectID, Title: "Different project"}); err != nil {
		t.Fatalf("CreateTask() for other project error = %v", err)
	}
	if _, err := repo.CreateTask(ctx, testUserID(), domain.Task{ProjectID: projectID, Title: "Different user"}); err != nil {
		t.Fatalf("CreateTask() for other user error = %v", err)
	}

	got, err := repo.ListTasks(ctx, userID, projectID)
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}

	if len(got) != len(want) {
		t.Errorf("ListTasks() returned %d tasks, want %d", len(got), len(want))
	}
	for _, task := range got {
		if !want[task.ID] {
			t.Errorf("ListTasks(%q, %q) returned unexpected task %q (projectID %q)", userID, projectID, task.ID, task.ProjectID)
		}
	}
}

func TestDynamoRepository_ListTasks_Empty(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()

	got, err := repo.ListTasks(ctx, testUserID(), "proj-"+domain.NewID())
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListTasks() = %+v, want empty", got)
	}
}

func TestDynamoRepository_CreateChangeset(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()
	projectID := "proj-" + domain.NewID()

	created, err := repo.CreateChangeset(ctx, domain.Changeset{
		ProjectID:   projectID,
		UserID:      userID,
		Skill:       "decompose_task",
		BaseVersion: 1,
		Status:      domain.ChangesetProposed,
		ProposedTasks: []domain.ProposedTask{
			{Title: "Add login command", Description: "Device-flow login for the CLI", AcceptanceCriteria: []string{"running `nudge login` prints a device code"}},
		},
	})
	if err != nil {
		t.Fatalf("CreateChangeset() error = %v", err)
	}

	if created.ID == "" {
		t.Error("ID = \"\", want a generated ID")
	}
	if created.ProjectID != projectID || created.Skill != "decompose_task" {
		t.Errorf("CreateChangeset() = %+v, want ProjectID/Skill preserved from input", created)
	}
	if created.CreatedAt.IsZero() {
		t.Errorf("CreateChangeset() = %+v, want CreatedAt set", created)
	}

	got, err := repo.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(testTable),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: userPK(userID)},
			"SK": &types.AttributeValueMemberS{Value: changesetSK(projectID, created.ID)},
		},
	})
	if err != nil {
		t.Fatalf("GetItem() error = %v", err)
	}
	if got.Item == nil {
		t.Fatalf("GetItem() found no item for changeset %q", created.ID)
	}
}

func TestDynamoRepository_CreateChangeset_DuplicateID(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()
	projectID := "proj-" + domain.NewID()

	id := "dup-" + domain.NewID()

	if _, err := repo.CreateChangeset(ctx, domain.Changeset{ID: id, ProjectID: projectID, UserID: userID}); err != nil {
		t.Fatalf("first CreateChangeset() error = %v", err)
	}

	_, err := repo.CreateChangeset(ctx, domain.Changeset{ID: id, ProjectID: projectID, UserID: userID})
	if !errors.Is(err, ErrDuplicateID) {
		t.Fatalf("second CreateChangeset() error = %v, want %v", err, ErrDuplicateID)
	}
}

func TestDynamoRepository_GetChangeset(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()
	projectID := "proj-" + domain.NewID()

	created, err := repo.CreateChangeset(ctx, domain.Changeset{
		ProjectID:   projectID,
		UserID:      userID,
		Skill:       "decompose_task",
		BaseVersion: 3,
		Status:      domain.ChangesetProposed,
		ProposedTasks: []domain.ProposedTask{
			{Title: "Add login command"},
		},
	})
	if err != nil {
		t.Fatalf("CreateChangeset() error = %v", err)
	}

	got, err := repo.GetChangeset(ctx, userID, projectID, created.ID)
	if err != nil {
		t.Fatalf("GetChangeset() error = %v", err)
	}
	if !reflect.DeepEqual(got, created) {
		t.Errorf("GetChangeset() = %+v, want %+v", got, created)
	}
}

func TestDynamoRepository_GetChangeset_NotFound(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()

	_, err := repo.GetChangeset(ctx, testUserID(), "proj-"+domain.NewID(), "missing-"+domain.NewID())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetChangeset() error = %v, want %v", err, ErrNotFound)
	}
}

func TestDynamoRepository_GetChangeset_WrongUser(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	projectID := "proj-" + domain.NewID()

	created, err := repo.CreateChangeset(ctx, domain.Changeset{ProjectID: projectID, UserID: testUserID()})
	if err != nil {
		t.Fatalf("CreateChangeset() error = %v", err)
	}

	_, err = repo.GetChangeset(ctx, testUserID(), projectID, created.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetChangeset() with wrong userID error = %v, want %v", err, ErrNotFound)
	}
}

func TestDynamoRepository_GetChangeset_WrongProject(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()

	created, err := repo.CreateChangeset(ctx, domain.Changeset{ProjectID: "proj-" + domain.NewID(), UserID: userID})
	if err != nil {
		t.Fatalf("CreateChangeset() error = %v", err)
	}

	_, err = repo.GetChangeset(ctx, userID, "proj-"+domain.NewID(), created.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetChangeset() with wrong projectID error = %v, want %v", err, ErrNotFound)
	}
}

func TestDynamoRepository_UpdateChangesetStatus(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()
	projectID := "proj-" + domain.NewID()

	created, err := repo.CreateChangeset(ctx, domain.Changeset{ProjectID: projectID, UserID: userID, Status: domain.ChangesetProposed})
	if err != nil {
		t.Fatalf("CreateChangeset() error = %v", err)
	}

	updated, err := repo.UpdateChangesetStatus(ctx, userID, projectID, created.ID, domain.ChangesetAccepted)
	if err != nil {
		t.Fatalf("UpdateChangesetStatus() error = %v", err)
	}
	if updated.Status != domain.ChangesetAccepted {
		t.Errorf("Status = %q, want %q", updated.Status, domain.ChangesetAccepted)
	}

	got, err := repo.GetChangeset(ctx, userID, projectID, created.ID)
	if err != nil {
		t.Fatalf("GetChangeset() error = %v", err)
	}
	if got.Status != domain.ChangesetAccepted {
		t.Errorf("GetChangeset() after update Status = %q, want %q", got.Status, domain.ChangesetAccepted)
	}
}

func TestDynamoRepository_UpdateChangesetStatus_NotFound(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()

	_, err := repo.UpdateChangesetStatus(ctx, testUserID(), "proj-"+domain.NewID(), "missing-"+domain.NewID(), domain.ChangesetAccepted)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateChangesetStatus() error = %v, want %v", err, ErrNotFound)
	}
}

func TestDynamoRepository_UpdateChangesetStatus_WrongUser(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	projectID := "proj-" + domain.NewID()

	created, err := repo.CreateChangeset(ctx, domain.Changeset{ProjectID: projectID, UserID: testUserID(), Status: domain.ChangesetProposed})
	if err != nil {
		t.Fatalf("CreateChangeset() error = %v", err)
	}

	_, err = repo.UpdateChangesetStatus(ctx, testUserID(), projectID, created.ID, domain.ChangesetAccepted)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateChangesetStatus() with wrong userID error = %v, want %v", err, ErrNotFound)
	}
}
