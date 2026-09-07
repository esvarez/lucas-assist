package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/esvarez/lucas-assist/internal/domain"
)

// metaSK is the sort key for a project's own item — see architecture.md §7
// ("Base | PROJECT#<id> | META / TASK#<uuid> / ...").
const metaSK = "META"

// DynamoRepository is a DynamoDB-backed Repository. Each project is stored
// as a single META item under PK=PROJECT#<id>, SK=META in a single table
// (architecture.md §7).
type DynamoRepository struct {
	client *dynamodb.Client
	table  string
}

// NewDynamoRepository wraps an existing DynamoDB client and table name.
// The client should be constructed once (e.g. via NewDynamoDBClient) and
// reused across invocations.
func NewDynamoRepository(client *dynamodb.Client, table string) *DynamoRepository {
	return &DynamoRepository{client: client, table: table}
}

// projectItem is the DynamoDB item shape for a project's META item. It's
// kept separate from domain.Project so the key attributes (PK/SK) don't
// leak into the domain model.
type projectItem struct {
	PK          string     `dynamodbav:"PK"`
	SK          string     `dynamodbav:"SK"`
	ID          string     `dynamodbav:"id"`
	Name        string     `dynamodbav:"name"`
	Goal        string     `dynamodbav:"goal"`
	Deadline    *time.Time `dynamodbav:"deadline,omitempty"`
	Constraints []string   `dynamodbav:"constraints,omitempty"`
	Status      string     `dynamodbav:"status"`
	CreatedAt   time.Time  `dynamodbav:"created_at"`
	UpdatedAt   time.Time  `dynamodbav:"updated_at"`
}

func projectPK(id string) string {
	return "PROJECT#" + id
}

func toProjectItem(p domain.Project) projectItem {
	return projectItem{
		PK:          projectPK(p.ID),
		SK:          metaSK,
		ID:          p.ID,
		Name:        p.Name,
		Goal:        p.Goal,
		Deadline:    p.Deadline,
		Constraints: p.Constraints,
		Status:      p.Status,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

func (i projectItem) toDomain() domain.Project {
	return domain.Project{
		ID:          i.ID,
		Name:        i.Name,
		Goal:        i.Goal,
		Deadline:    i.Deadline,
		Constraints: i.Constraints,
		Status:      i.Status,
		CreatedAt:   i.CreatedAt,
		UpdatedAt:   i.UpdatedAt,
	}
}

// CreateProject writes a project's META item via a conditional PutItem
// (attribute_not_exists(PK)) so an existing project is never overwritten.
func (r *DynamoRepository) CreateProject(ctx context.Context, p domain.Project) (domain.Project, error) {
	if p.ID == "" {
		p.ID = domain.NewID()
	}

	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now

	item, err := attributevalue.MarshalMap(toProjectItem(p))
	if err != nil {
		return domain.Project{}, fmt.Errorf("marshal project item: %w", err)
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.table),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			return domain.Project{}, ErrDuplicateID
		}
		return domain.Project{}, fmt.Errorf("put project item: %w", err)
	}

	return p, nil
}

// GetProject fetches a project's META item by ID. Returns ErrNotFound if no
// such item exists.
func (r *DynamoRepository) GetProject(ctx context.Context, id string) (domain.Project, error) {
	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: projectPK(id)},
			"SK": &types.AttributeValueMemberS{Value: metaSK},
		},
	})
	if err != nil {
		return domain.Project{}, fmt.Errorf("get project item: %w", err)
	}
	if out.Item == nil {
		return domain.Project{}, ErrNotFound
	}

	var item projectItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return domain.Project{}, fmt.Errorf("unmarshal project item: %w", err)
	}

	return item.toDomain(), nil
}

// ListProjects scans the table for all META items and returns them as
// projects. This is an MVP implementation; architecture.md §7 designs a
// GSI1 (PK=USER#<id>) for a per-user project list, but that requires a
// UserID field that doesn't exist yet.
func (r *DynamoRepository) ListProjects(ctx context.Context) ([]domain.Project, error) {
	projects := make([]domain.Project, 0)

	paginator := dynamodb.NewScanPaginator(r.client, &dynamodb.ScanInput{
		TableName:        aws.String(r.table),
		FilterExpression: aws.String("SK = :meta"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":meta": &types.AttributeValueMemberS{Value: metaSK},
		},
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("scan project items: %w", err)
		}

		var items []projectItem
		if err := attributevalue.UnmarshalListOfMaps(page.Items, &items); err != nil {
			return nil, fmt.Errorf("unmarshal project items: %w", err)
		}
		for _, item := range items {
			projects = append(projects, item.toDomain())
		}
	}

	return projects, nil
}
