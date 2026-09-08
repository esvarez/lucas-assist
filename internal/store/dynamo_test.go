//go:build integration

package store

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

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
