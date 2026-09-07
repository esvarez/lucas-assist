//go:build integration

package store

import (
	"context"
	"errors"
	"os"
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
	client, err := NewDynamoDBClient(ctx, endpoint)
	if err != nil {
		t.Fatalf("NewDynamoDBClient() error = %v", err)
	}

	if err := ensureTestTable(ctx, client); err != nil {
		t.Fatalf("ensureTestTable() error = %v", err)
	}

	return NewDynamoRepository(client, testTable)
}

func ensureTestTable(ctx context.Context, client *dynamodb.Client) error {
	_, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(testTable)})
	if err == nil {
		return nil
	}

	var notFound *types.ResourceNotFoundException
	if !errors.As(err, &notFound) {
		return err
	}

	_, err = client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:   aws.String(testTable),
		BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("PK"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("SK"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("PK"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("SK"), KeyType: types.KeyTypeRange},
		},
	})
	if err != nil {
		return err
	}

	waiter := dynamodb.NewTableExistsWaiter(client)
	return waiter.Wait(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(testTable)}, time.Minute)
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

	id := "dup-" + newID()

	if _, err := repo.CreateProject(ctx, domain.Project{ID: id, Name: "First"}); err != nil {
		t.Fatalf("first CreateProject() error = %v", err)
	}

	_, err := repo.CreateProject(ctx, domain.Project{ID: id, Name: "Second"})
	if !errors.Is(err, ErrDuplicateID) {
		t.Fatalf("second CreateProject() error = %v, want %v", err, ErrDuplicateID)
	}
}
