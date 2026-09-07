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
	return newTestDynamoRepositoryWithTable(t, testTable)
}

// newIsolatedTestDynamoRepository provisions a fresh, never-before-used
// table so the caller can rely on it starting out empty — the shared
// testTable accumulates items across test runs against a persistent
// DynamoDB Local instance.
func newIsolatedTestDynamoRepository(t *testing.T) *DynamoRepository {
	t.Helper()
	return newTestDynamoRepositoryWithTable(t, testTable+"-"+domain.NewID())
}

func newTestDynamoRepositoryWithTable(t *testing.T, table string) *DynamoRepository {
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

	if err := EnsureTable(ctx, client, table); err != nil {
		t.Fatalf("EnsureTable() error = %v", err)
	}

	return NewDynamoRepository(client, table)
}

func TestDynamoRepository_CreateProject(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()

	created, err := repo.CreateProject(ctx, domain.Project{
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
			"PK": &types.AttributeValueMemberS{Value: projectPK(created.ID)},
			"SK": &types.AttributeValueMemberS{Value: metaSK},
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

	id := "dup-" + domain.NewID()

	if _, err := repo.CreateProject(ctx, domain.Project{ID: id, Name: "First"}); err != nil {
		t.Fatalf("first CreateProject() error = %v", err)
	}

	_, err := repo.CreateProject(ctx, domain.Project{ID: id, Name: "Second"})
	if !errors.Is(err, ErrDuplicateID) {
		t.Fatalf("second CreateProject() error = %v, want %v", err, ErrDuplicateID)
	}
}

func TestDynamoRepository_GetProject(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()

	created, err := repo.CreateProject(ctx, domain.Project{
		Name:        "Nudge",
		Goal:        "Ship the POC",
		Constraints: []string{"no VPC", "no SSR"},
		Status:      "active",
	})
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

func TestDynamoRepository_GetProject_NotFound(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()

	_, err := repo.GetProject(ctx, "missing-"+domain.NewID())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetProject() error = %v, want %v", err, ErrNotFound)
	}
}

func TestDynamoRepository_ListProjects(t *testing.T) {
	repo := newTestDynamoRepository(t)
	ctx := context.Background()

	want := make(map[string]bool)
	for _, name := range []string{"Nudge", "Widget"} {
		created, err := repo.CreateProject(ctx, domain.Project{Name: name})
		if err != nil {
			t.Fatalf("CreateProject() error = %v", err)
		}
		want[created.ID] = true
	}

	got, err := repo.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}

	found := 0
	for _, p := range got {
		if want[p.ID] {
			found++
		}
	}
	if found != len(want) {
		t.Errorf("ListProjects() found %d/%d created projects in result of length %d", found, len(want), len(got))
	}
}

func TestDynamoRepository_ListProjects_Empty(t *testing.T) {
	repo := newIsolatedTestDynamoRepository(t)
	ctx := context.Background()

	got, err := repo.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListProjects() = %+v, want empty", got)
	}
}
