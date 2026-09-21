//go:build integration

package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// TestDynamoRepository_CreateChangeset_CreateProject documents #101's
// ProposedProject addition: a create_project changeset has no ProjectID
// (the noProjectSegment placeholder covers it, same as agentRunSK) and
// carries ProposedProject instead of ProposedTasks.
func TestDynamoRepository_CreateChangeset_CreateProject(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()

	created, err := repo.CreateChangeset(ctx, domain.Changeset{
		UserID: userID,
		Skill:  "create_project",
		Status: domain.ChangesetProposed,
		ProposedProject: &domain.ProposedProject{
			Name: "Nudge",
			Goal: "Ship the POC",
		},
	})
	if err != nil {
		t.Fatalf("CreateChangeset() error = %v", err)
	}

	got, err := repo.GetChangeset(ctx, userID, "", created.ID)
	if err != nil {
		t.Fatalf("GetChangeset() error = %v", err)
	}
	if got.ProposedProject == nil || got.ProposedProject.Name != "Nudge" || got.ProposedProject.Goal != "Ship the POC" {
		t.Errorf("GetChangeset().ProposedProject = %+v, want the proposed project preserved", got.ProposedProject)
	}
	if len(got.ProposedTasks) != 0 {
		t.Errorf("GetChangeset().ProposedTasks = %+v, want none for a create_project changeset", got.ProposedTasks)
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

func TestDynamoRepository_CreateAgentRun(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()
	projectID := "proj-" + domain.NewID()

	created, err := repo.CreateAgentRun(ctx, domain.AgentRun{
		UserID:    userID,
		ProjectID: projectID,
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

	got, err := repo.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(testTable),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: userPK(userID)},
			"SK": &types.AttributeValueMemberS{Value: agentRunSK(projectID, created.ID)},
		},
	})
	if err != nil {
		t.Fatalf("GetItem() error = %v", err)
	}
	if got.Item == nil {
		t.Fatalf("GetItem() found no item for agent run %q", created.ID)
	}
}

// TestDynamoRepository_CreateAgentRun_EmptyProjectID documents the
// ProjectID-empty scheme for create_project runs (architecture.md §3, issue
// #96): the run is stored under a fixed placeholder SK segment rather than
// a real project ID, and remains reachable by GetAgentRun with "" as the
// projectID argument.
func TestDynamoRepository_CreateAgentRun_EmptyProjectID(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()

	created, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: userID, Skill: "create_project"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}
	if created.ProjectID != "" {
		t.Errorf("ProjectID = %q, want empty for a create_project run", created.ProjectID)
	}

	got, err := repo.GetAgentRun(ctx, userID, "", created.ID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if !reflect.DeepEqual(got, created) {
		t.Errorf("GetAgentRun() = %+v, want %+v", got, created)
	}
}

func TestDynamoRepository_GetAgentRun_NotFound(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()

	_, err := repo.GetAgentRun(ctx, testUserID(), "proj-"+domain.NewID(), "missing-"+domain.NewID())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetAgentRun() error = %v, want %v", err, ErrNotFound)
	}
}

func TestDynamoRepository_GetAgentRun_WrongUser(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	projectID := "proj-" + domain.NewID()

	created, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: testUserID(), ProjectID: projectID})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	_, err = repo.GetAgentRun(ctx, testUserID(), projectID, created.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetAgentRun() with wrong userID error = %v, want %v", err, ErrNotFound)
	}
}

func TestDynamoRepository_LeaseAgentRun(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()
	projectID := "proj-" + domain.NewID()

	created, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: userID, ProjectID: projectID})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	leaseUntil := time.Now().UTC().Add(time.Minute)
	leased, err := repo.LeaseAgentRun(ctx, userID, projectID, created.ID, "worker_1", leaseUntil)
	if err != nil {
		t.Fatalf("LeaseAgentRun() error = %v", err)
	}

	if leased.Status != domain.AgentRunRunning {
		t.Errorf("Status = %q, want %q", leased.Status, domain.AgentRunRunning)
	}
	if leased.WorkerID != "worker_1" {
		t.Errorf("WorkerID = %q, want %q", leased.WorkerID, "worker_1")
	}
	if leased.Attempt != 1 {
		t.Errorf("Attempt = %d, want 1 after first lease", leased.Attempt)
	}
}

// TestDynamoRepository_LeaseAgentRun_Race documents the lease race
// (architecture.md §15, issue #96 acceptance criteria): two lease attempts
// on the same queued run must not both succeed.
func TestDynamoRepository_LeaseAgentRun_Race(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()
	projectID := "proj-" + domain.NewID()

	created, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: userID, ProjectID: projectID})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	leaseUntil := time.Now().UTC().Add(time.Minute)
	if _, err := repo.LeaseAgentRun(ctx, userID, projectID, created.ID, "worker_1", leaseUntil); err != nil {
		t.Fatalf("first LeaseAgentRun() error = %v", err)
	}

	_, err = repo.LeaseAgentRun(ctx, userID, projectID, created.ID, "worker_2", leaseUntil)
	if !errors.Is(err, ErrRunLeased) {
		t.Fatalf("second LeaseAgentRun() error = %v, want %v", err, ErrRunLeased)
	}

	got, err := repo.GetAgentRun(ctx, userID, projectID, created.ID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if got.WorkerID != "worker_1" {
		t.Errorf("WorkerID = %q, want %q (the losing lease attempt must not have applied)", got.WorkerID, "worker_1")
	}
}

// TestDynamoRepository_LeaseAgentRun_ReclaimExpired documents reclaiming an
// expired lease (architecture.md §15, issue #96 acceptance criteria): a
// worker that never completed its attempt before LeaseUntil passed must not
// block a later attempt from leasing the run.
func TestDynamoRepository_LeaseAgentRun_ReclaimExpired(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()
	projectID := "proj-" + domain.NewID()

	created, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: userID, ProjectID: projectID})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	expiredLease := time.Now().UTC().Add(-time.Minute)
	if _, err := repo.LeaseAgentRun(ctx, userID, projectID, created.ID, "worker_1", expiredLease); err != nil {
		t.Fatalf("first LeaseAgentRun() error = %v", err)
	}

	newLease := time.Now().UTC().Add(time.Minute)
	reclaimed, err := repo.LeaseAgentRun(ctx, userID, projectID, created.ID, "worker_2", newLease)
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

func TestDynamoRepository_LeaseAgentRun_NotFound(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()

	_, err := repo.LeaseAgentRun(ctx, testUserID(), "proj-"+domain.NewID(), "missing-"+domain.NewID(), "worker_1", time.Now().Add(time.Minute))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("LeaseAgentRun() error = %v, want %v", err, ErrNotFound)
	}
}

func TestDynamoRepository_CompleteAgentRun(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()
	projectID := "proj-" + domain.NewID()

	created, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: userID, ProjectID: projectID})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}
	if _, err := repo.LeaseAgentRun(ctx, userID, projectID, created.ID, "worker_1", time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("LeaseAgentRun() error = %v", err)
	}

	completed, err := repo.CompleteAgentRun(ctx, userID, projectID, created.ID, "cs_1")
	if err != nil {
		t.Fatalf("CompleteAgentRun() error = %v", err)
	}
	if completed.Status != domain.AgentRunCompleted {
		t.Errorf("Status = %q, want %q", completed.Status, domain.AgentRunCompleted)
	}
	if completed.ChangesetID != "cs_1" {
		t.Errorf("ChangesetID = %q, want %q", completed.ChangesetID, "cs_1")
	}

	got, err := repo.GetAgentRun(ctx, userID, projectID, created.ID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if got.Status != domain.AgentRunCompleted {
		t.Errorf("GetAgentRun() after complete Status = %q, want %q", got.Status, domain.AgentRunCompleted)
	}
	if got.ChangesetID != "cs_1" {
		t.Errorf("GetAgentRun() after complete ChangesetID = %q, want %q", got.ChangesetID, "cs_1")
	}
}

func TestDynamoRepository_CompleteAgentRun_NotFound(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()

	_, err := repo.CompleteAgentRun(ctx, testUserID(), "proj-"+domain.NewID(), "missing-"+domain.NewID(), "cs_1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("CompleteAgentRun() error = %v, want %v", err, ErrNotFound)
	}
}

func TestDynamoRepository_FailAgentRun(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()
	projectID := "proj-" + domain.NewID()

	created, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: userID, ProjectID: projectID})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}
	if _, err := repo.LeaseAgentRun(ctx, userID, projectID, created.ID, "worker_1", time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("LeaseAgentRun() error = %v", err)
	}

	failed, err := repo.FailAgentRun(ctx, userID, projectID, created.ID, "provider timeout")
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

func TestDynamoRepository_FailAgentRun_NotFound(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()

	_, err := repo.FailAgentRun(ctx, testUserID(), "proj-"+domain.NewID(), "missing-"+domain.NewID(), "boom")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("FailAgentRun() error = %v, want %v", err, ErrNotFound)
	}
}

func TestDynamoRepository_NeedsInputAgentRun(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()
	projectID := "proj-" + domain.NewID()

	created, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: userID, ProjectID: projectID})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}
	if _, err := repo.LeaseAgentRun(ctx, userID, projectID, created.ID, "worker_1", time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("LeaseAgentRun() error = %v", err)
	}

	questions := []string{"What is the task actually about?"}
	got, err := repo.NeedsInputAgentRun(ctx, userID, projectID, created.ID, questions)
	if err != nil {
		t.Fatalf("NeedsInputAgentRun() error = %v", err)
	}
	if got.Status != domain.AgentRunNeedsInput {
		t.Errorf("Status = %q, want %q", got.Status, domain.AgentRunNeedsInput)
	}
	if len(got.Questions) != 1 || got.Questions[0] != questions[0] {
		t.Errorf("Questions = %#v, want %#v", got.Questions, questions)
	}
}

func TestDynamoRepository_NeedsInputAgentRun_NotFound(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()

	_, err := repo.NeedsInputAgentRun(ctx, testUserID(), "proj-"+domain.NewID(), "missing-"+domain.NewID(), []string{"?"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("NeedsInputAgentRun() error = %v, want %v", err, ErrNotFound)
	}
}

// newTestAcceptableChangeset creates a project and a proposed changeset
// against it, ready to accept — shared setup for the AcceptChangeset tests
// below.
func newTestAcceptableChangeset(t *testing.T, repo *DynamoRepository, userID string, proposedTasks []domain.ProposedTask) (domain.Project, domain.Changeset) {
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

func TestDynamoRepository_AcceptChangeset(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()

	project, changeset := newTestAcceptableChangeset(t, repo, userID, []domain.ProposedTask{
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
	if len(result.Tasks) != 2 {
		t.Fatalf("len(Tasks) = %d, want 2", len(result.Tasks))
	}
	if result.Tasks[0].Title != "First" || result.Tasks[1].Title != "Second" {
		t.Errorf("Tasks = %+v, want titles preserved in order", result.Tasks)
	}
	for i, task := range result.Tasks {
		if task.Status != "todo" {
			t.Errorf("Tasks[%d].Status = %q, want \"todo\"", i, task.Status)
		}
	}
	if result.Event.ID == "" || result.Event.Type != domain.EventChangesetAccepted {
		t.Errorf("Event = %+v, want a generated ID and Type %q", result.Event, domain.EventChangesetAccepted)
	}

	gotProject, err := repo.GetProject(ctx, userID, project.ID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if gotProject.Version != project.Version+1 {
		t.Errorf("stored Project.Version = %d, want %d", gotProject.Version, project.Version+1)
	}

	for _, task := range result.Tasks {
		if _, err := repo.GetTask(ctx, userID, project.ID, task.ID); err != nil {
			t.Errorf("GetTask(%q) error = %v, want the task to be persisted", task.ID, err)
		}
	}

	gotChangeset, err := repo.GetChangeset(ctx, userID, project.ID, changeset.ID)
	if err != nil {
		t.Fatalf("GetChangeset() error = %v", err)
	}
	if gotChangeset.Status != domain.ChangesetApplied {
		t.Errorf("Changeset.Status = %q, want %q", gotChangeset.Status, domain.ChangesetApplied)
	}
}

// TestDynamoRepository_AcceptChangeset_VersionConflict documents the
// architecture.md §8 optimistic-concurrency path for accept: a changeset
// whose baseVersion no longer matches the project's stored version is
// rejected with ErrConflict via the transaction's ConditionalCheckFailed,
// and the changeset moves to "conflict" — not retried automatically
// (AGENTS.MD).
func TestDynamoRepository_AcceptChangeset_VersionConflict(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()

	project, changeset := newTestAcceptableChangeset(t, repo, userID, []domain.ProposedTask{{Title: "First"}})

	if _, err := repo.UpdateProject(ctx, userID, domain.Project{ID: project.ID, Goal: "changed", Version: project.Version}); err != nil {
		t.Fatalf("UpdateProject() error = %v", err)
	}

	_, err := repo.AcceptChangeset(ctx, project, changeset, "idem-key-1")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("AcceptChangeset() error = %v, want %v", err, ErrConflict)
	}

	gotChangeset, err := repo.GetChangeset(ctx, userID, project.ID, changeset.ID)
	if err != nil {
		t.Fatalf("GetChangeset() error = %v", err)
	}
	if gotChangeset.Status != domain.ChangesetConflict {
		t.Errorf("Changeset.Status = %q, want %q", gotChangeset.Status, domain.ChangesetConflict)
	}

	tasks, err := repo.ListTasks(ctx, userID, project.ID)
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("ListTasks() = %+v, want no tasks created on a conflicting accept", tasks)
	}
}

// TestDynamoRepository_AcceptChangeset_ConcurrentAcceptDoesNotOverwriteApplied
// documents the race two concurrent AcceptChangeset calls on the *same*
// changeset would hit: both read the changeset as "proposed" before either
// commits, one wins the transaction (changeset -> applied, tasks created,
// project version bumped), and the loser must not then stomp that "applied"
// status back to "conflict" via a non-transactional write. Modeled here as
// two sequential calls against the same in-flight (pre-accept) changeset
// value, which reproduces the loser's exact code path once the winner has
// already committed.
func TestDynamoRepository_AcceptChangeset_ConcurrentAcceptDoesNotOverwriteApplied(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()

	project, changeset := newTestAcceptableChangeset(t, repo, userID, []domain.ProposedTask{{Title: "First"}})

	if _, err := repo.AcceptChangeset(ctx, project, changeset, "idem-key-winner"); err != nil {
		t.Fatalf("winner AcceptChangeset() error = %v", err)
	}

	_, err := repo.AcceptChangeset(ctx, project, changeset, "idem-key-loser")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("loser AcceptChangeset() error = %v, want %v", err, ErrConflict)
	}

	gotChangeset, err := repo.GetChangeset(ctx, userID, project.ID, changeset.ID)
	if err != nil {
		t.Fatalf("GetChangeset() error = %v", err)
	}
	if gotChangeset.Status != domain.ChangesetApplied {
		t.Errorf("Changeset.Status = %q, want %q — the loser must not overwrite the winner's applied status", gotChangeset.Status, domain.ChangesetApplied)
	}

	tasks, err := repo.ListTasks(ctx, userID, project.ID)
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("ListTasks() = %+v, want the winner's single task, none from the loser", tasks)
	}
}

// TestDynamoRepository_AcceptChangeset_IdempotentReplay documents
// architecture.md §15: replaying an accept with the same idempotency key
// returns the original result instead of re-applying it (no duplicate
// tasks, no second version bump).
func TestDynamoRepository_AcceptChangeset_IdempotentReplay(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()

	project, changeset := newTestAcceptableChangeset(t, repo, userID, []domain.ProposedTask{{Title: "First"}})

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

	tasks, err := repo.ListTasks(ctx, userID, project.ID)
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("ListTasks() returned %d tasks, want 1 (replay must not re-apply)", len(tasks))
	}

	gotProject, err := repo.GetProject(ctx, userID, project.ID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if gotProject.Version != project.Version+1 {
		t.Errorf("Project.Version = %d, want %d (replay must not bump it twice)", gotProject.Version, project.Version+1)
	}
}

// TestDynamoRepository_AcceptChangeset_IdempotencyKeyReused documents the
// other half of architecture.md §15: reusing an idempotency key against a
// different request is rejected, not silently answered with the first
// request's result.
func TestDynamoRepository_AcceptChangeset_IdempotencyKeyReused(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()

	project, changeset := newTestAcceptableChangeset(t, repo, userID, []domain.ProposedTask{{Title: "First"}})
	if _, err := repo.AcceptChangeset(ctx, project, changeset, "idem-key-1"); err != nil {
		t.Fatalf("first AcceptChangeset() error = %v", err)
	}

	_, otherChangeset := newTestAcceptableChangeset(t, repo, userID, []domain.ProposedTask{{Title: "Unrelated"}})

	_, err := repo.AcceptChangeset(ctx, project, otherChangeset, "idem-key-1")
	if !errors.Is(err, ErrIdempotencyKeyReused) {
		t.Fatalf("AcceptChangeset() with reused key error = %v, want %v", err, ErrIdempotencyKeyReused)
	}
}

// TestDynamoRepository_AcceptChangeset_NotProposed documents that a
// changeset in any status other than "proposed" can't be accepted with a
// fresh idempotency key — distinct from the idempotent-replay case (same
// key, already applied), which must still succeed.
func TestDynamoRepository_AcceptChangeset_NotProposed(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()

	project, changeset := newTestAcceptableChangeset(t, repo, userID, []domain.ProposedTask{{Title: "First"}})
	changeset, err := repo.UpdateChangesetStatus(ctx, userID, project.ID, changeset.ID, domain.ChangesetRejected)
	if err != nil {
		t.Fatalf("UpdateChangesetStatus() error = %v", err)
	}

	_, err = repo.AcceptChangeset(ctx, project, changeset, "idem-key-1")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("AcceptChangeset() on a rejected changeset error = %v, want %v", err, ErrConflict)
	}

	gotChangeset, err := repo.GetChangeset(ctx, userID, project.ID, changeset.ID)
	if err != nil {
		t.Fatalf("GetChangeset() error = %v", err)
	}
	if gotChangeset.Status != domain.ChangesetRejected {
		t.Errorf("Changeset.Status = %q, want unchanged %q", gotChangeset.Status, domain.ChangesetRejected)
	}
}

// TestDynamoRepository_AcceptChangeset_OversizedChangeset documents ADR
// 009: a changeset over the 50-mutation cap is a business rule the
// changeset-accept endpoint enforces (internal/api/changesets.go), not
// AcceptChangeset itself — this test exercises the primitive with a
// changeset that exceeds it, confirming AcceptChangeset has no cap of its
// own baked in silently and would otherwise attempt the write.
func TestDynamoRepository_AcceptChangeset_OversizedChangeset(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()
	userID := testUserID()

	tasks := make([]domain.ProposedTask, 51)
	for i := range tasks {
		tasks[i] = domain.ProposedTask{Title: fmt.Sprintf("Task %d", i)}
	}
	project, changeset := newTestAcceptableChangeset(t, repo, userID, tasks)

	result, err := repo.AcceptChangeset(ctx, project, changeset, "idem-key-1")
	if err != nil {
		t.Fatalf("AcceptChangeset() error = %v", err)
	}
	if len(result.Tasks) != 51 {
		t.Fatalf("len(Tasks) = %d, want 51 — the 50-mutation cap is enforced by the HTTP handler, not this primitive", len(result.Tasks))
	}
}
