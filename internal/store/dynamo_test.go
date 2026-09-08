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
			"PK": &types.AttributeValueMemberS{Value: projectPK(userID)},
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

	deadline := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	updated, err := repo.UpdateProject(ctx, domain.Project{
		UserID:      userID,
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

	got, err := repo.GetProject(ctx, userID, created.ID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if !reflect.DeepEqual(got, updated) {
		t.Errorf("GetProject() after update = %+v, want %+v", got, updated)
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

	updated, err := repo.UpdateProject(ctx, domain.Project{UserID: userID, ID: created.ID, Status: "active"})
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

	_, err := repo.UpdateProject(ctx, domain.Project{UserID: testUserID(), ID: "missing-" + domain.NewID()})
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

	_, err = repo.UpdateProject(ctx, domain.Project{UserID: testUserID(), ID: created.ID, Status: "done"})
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
