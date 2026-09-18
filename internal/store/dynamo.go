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

// metaSKPrefix is the sort-key prefix for a project's own item — see
// architecture.md §8's key schema table (`PK=USER#<uid>`, `SK=META#<pid>`).
const metaSKPrefix = "META#"

// projectScopeSKPrefix is the sort-key prefix shared by every item scoped
// to one project — Task, Changeset, and so on — e.g. `PK=USER#<uid>`,
// `SK=P#<pid>#TASK#<task_id>` (architecture.md §8). Sharing the `P#<pid>#`
// prefix with the project's own META item is what lets one paginated Query
// retrieve a whole project's items together.
const projectScopeSKPrefix = "P#"

// DynamoRepository is a DynamoDB-backed Repository. Each project is stored
// as a single META item under PK=USER#<uid>, SK=META#<pid> in a single
// table, partitioned by user (architecture.md §8). This structurally scopes
// every query to its caller's data — a query built from the wrong userID
// simply can't see another user's items.
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
	UserID      string     `dynamodbav:"user_id"`
	ID          string     `dynamodbav:"id"`
	Name        string     `dynamodbav:"name"`
	Goal        string     `dynamodbav:"goal"`
	Deadline    *time.Time `dynamodbav:"deadline,omitempty"`
	Constraints []string   `dynamodbav:"constraints,omitempty"`
	Status      string     `dynamodbav:"status"`
	Version     int        `dynamodbav:"version"`
	CreatedAt   time.Time  `dynamodbav:"created_at"`
	UpdatedAt   time.Time  `dynamodbav:"updated_at"`
}

func userPK(userID string) string {
	return "USER#" + userID
}

func projectSK(id string) string {
	return metaSKPrefix + id
}

func taskSK(projectID, taskID string) string {
	return taskListSKPrefix(projectID) + taskID
}

// taskListSKPrefix is the begins_with prefix that scopes a Query to one
// project's tasks (architecture.md §8).
func taskListSKPrefix(projectID string) string {
	return projectScopeSKPrefix + projectID + "#TASK#"
}

func changesetSK(projectID, changesetID string) string {
	return changesetListSKPrefix(projectID) + changesetID
}

// changesetListSKPrefix is the begins_with prefix that scopes a Query to
// one project's changesets (architecture.md §8).
func changesetListSKPrefix(projectID string) string {
	return projectScopeSKPrefix + projectID + "#CHANGESET#"
}

func toProjectItem(p domain.Project) projectItem {
	return projectItem{
		PK:          userPK(p.UserID),
		SK:          projectSK(p.ID),
		UserID:      p.UserID,
		ID:          p.ID,
		Name:        p.Name,
		Goal:        p.Goal,
		Deadline:    p.Deadline,
		Constraints: p.Constraints,
		Status:      p.Status,
		Version:     p.Version,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

func (i projectItem) toDomain() domain.Project {
	return domain.Project{
		UserID:      i.UserID,
		ID:          i.ID,
		Name:        i.Name,
		Goal:        i.Goal,
		Deadline:    i.Deadline,
		Constraints: i.Constraints,
		Status:      i.Status,
		Version:     i.Version,
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
	p.Version = 1

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

// GetProject fetches a project's META item by userID and ID. Returns
// ErrNotFound if no such item exists — including when id belongs to a
// project owned by a different user, since that item lives under a
// different PK entirely.
func (r *DynamoRepository) GetProject(ctx context.Context, userID, id string) (domain.Project, error) {
	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: userPK(userID)},
			"SK": &types.AttributeValueMemberS{Value: projectSK(id)},
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

// UpdateProject updates a project's mutable fields (Goal, Deadline,
// Constraints, Status) via a conditional UpdateItem, bumps UpdatedAt, and
// enforces optimistic concurrency: p.Version must equal the item's current
// stored version, or the update is rejected instead of applied. On success
// the stored version increments by one.
//
// One UpdateItem call can't tell you which half of a compound condition
// failed, so ReturnValuesOnConditionCheckFailure asks DynamoDB to include
// the item's current attributes (if any) on failure: no item at all means
// the project doesn't exist or belongs to a different user (ErrNotFound,
// same as before), an item with a different version means the write lost
// the optimistic-concurrency race (ErrConflict) — never retried
// automatically (AGENTS.MD: "do not retry the write").
func (r *DynamoRepository) UpdateProject(ctx context.Context, userID string, p domain.Project) (domain.Project, error) {
	now := time.Now().UTC()

	goalAV, err := attributevalue.Marshal(p.Goal)
	if err != nil {
		return domain.Project{}, fmt.Errorf("marshal goal: %w", err)
	}
	constraintsAV, err := attributevalue.Marshal(p.Constraints)
	if err != nil {
		return domain.Project{}, fmt.Errorf("marshal constraints: %w", err)
	}
	statusAV, err := attributevalue.Marshal(p.Status)
	if err != nil {
		return domain.Project{}, fmt.Errorf("marshal status: %w", err)
	}
	updatedAtAV, err := attributevalue.Marshal(now)
	if err != nil {
		return domain.Project{}, fmt.Errorf("marshal updated_at: %w", err)
	}
	expectedVersionAV, err := attributevalue.Marshal(p.Version)
	if err != nil {
		return domain.Project{}, fmt.Errorf("marshal expected version: %w", err)
	}
	newVersionAV, err := attributevalue.Marshal(p.Version + 1)
	if err != nil {
		return domain.Project{}, fmt.Errorf("marshal new version: %w", err)
	}

	// Several of these are DynamoDB reserved words (confirmed the hard way:
	// "constraints" and "status" both are) and can't appear literally in an
	// update expression, so every attribute name here gets an alias.
	updateExpr := "SET #goal = :goal, #constraints = :constraints, #status = :status, #updated_at = :updated_at, #version = :new_version"
	names := map[string]string{
		"#goal":        "goal",
		"#constraints": "constraints",
		"#status":      "status",
		"#updated_at":  "updated_at",
		"#version":     "version",
	}
	values := map[string]types.AttributeValue{
		":goal":             goalAV,
		":constraints":      constraintsAV,
		":status":           statusAV,
		":updated_at":       updatedAtAV,
		":new_version":      newVersionAV,
		":expected_version": expectedVersionAV,
	}

	if p.Deadline != nil {
		deadlineAV, err := attributevalue.Marshal(p.Deadline)
		if err != nil {
			return domain.Project{}, fmt.Errorf("marshal deadline: %w", err)
		}
		updateExpr += ", #deadline = :deadline"
		names["#deadline"] = "deadline"
		values[":deadline"] = deadlineAV
	} else {
		updateExpr += " REMOVE #deadline"
		names["#deadline"] = "deadline"
	}

	out, err := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: userPK(userID)},
			"SK": &types.AttributeValueMemberS{Value: projectSK(p.ID)},
		},
		UpdateExpression:                    aws.String(updateExpr),
		ConditionExpression:                 aws.String("attribute_exists(PK) AND #version = :expected_version"),
		ExpressionAttributeNames:            names,
		ExpressionAttributeValues:           values,
		ReturnValues:                        types.ReturnValueAllNew,
		ReturnValuesOnConditionCheckFailure: types.ReturnValuesOnConditionCheckFailureAllOld,
	})
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			if len(condErr.Item) == 0 {
				return domain.Project{}, ErrNotFound
			}
			return domain.Project{}, ErrConflict
		}
		return domain.Project{}, fmt.Errorf("update project item: %w", err)
	}

	var item projectItem
	if err := attributevalue.UnmarshalMap(out.Attributes, &item); err != nil {
		return domain.Project{}, fmt.Errorf("unmarshal project item: %w", err)
	}

	return item.toDomain(), nil
}

// DeleteProject deletes a project's META item via a conditional DeleteItem
// (attribute_exists(PK)). The condition fails — returning ErrNotFound —
// both when the project doesn't exist at all and when userID doesn't match
// its actual owner, since that project's item lives under a different PK
// entirely.
func (r *DynamoRepository) DeleteProject(ctx context.Context, userID, id string) error {
	_, err := r.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(r.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: userPK(userID)},
			"SK": &types.AttributeValueMemberS{Value: projectSK(id)},
		},
		ConditionExpression: aws.String("attribute_exists(PK)"),
	})
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			return ErrNotFound
		}
		return fmt.Errorf("delete project item: %w", err)
	}

	return nil
}

// ListProjects queries the user's partition for all META items and returns
// them as projects (architecture.md §8: `PK = USER#<uid> AND
// begins_with(SK, "META#")`). No GSI is needed — the base table already
// scopes the result to userID.
func (r *DynamoRepository) ListProjects(ctx context.Context, userID string) ([]domain.Project, error) {
	projects := make([]domain.Project, 0)

	paginator := dynamodb.NewQueryPaginator(r.client, &dynamodb.QueryInput{
		TableName:              aws.String(r.table),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :skPrefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":       &types.AttributeValueMemberS{Value: userPK(userID)},
			":skPrefix": &types.AttributeValueMemberS{Value: metaSKPrefix},
		},
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("query project items: %w", err)
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

// taskItem is the DynamoDB item shape for a task. UserID is carried here
// even though domain.Task has no such field — it's needed to build PK, the
// same reason store.Repository's Task methods take userID as an explicit
// parameter instead of reading it off the domain struct.
type taskItem struct {
	PK                 string   `dynamodbav:"PK"`
	SK                 string   `dynamodbav:"SK"`
	UserID             string   `dynamodbav:"user_id"`
	ProjectID          string   `dynamodbav:"project_id"`
	ID                 string   `dynamodbav:"id"`
	ParentID           string   `dynamodbav:"parent_id,omitempty"`
	Title              string   `dynamodbav:"title"`
	Description        string   `dynamodbav:"description,omitempty"`
	Status             string   `dynamodbav:"status"`
	Order              int      `dynamodbav:"order"`
	AcceptanceCriteria []string `dynamodbav:"acceptance_criteria,omitempty"`
}

func toTaskItem(userID string, t domain.Task) taskItem {
	return taskItem{
		PK:                 userPK(userID),
		SK:                 taskSK(t.ProjectID, t.ID),
		UserID:             userID,
		ProjectID:          t.ProjectID,
		ID:                 t.ID,
		ParentID:           t.ParentID,
		Title:              t.Title,
		Description:        t.Description,
		Status:             t.Status,
		Order:              t.Order,
		AcceptanceCriteria: t.AcceptanceCriteria,
	}
}

func (i taskItem) toDomain() domain.Task {
	return domain.Task{
		ID:                 i.ID,
		ProjectID:          i.ProjectID,
		ParentID:           i.ParentID,
		Title:              i.Title,
		Description:        i.Description,
		Status:             i.Status,
		Order:              i.Order,
		AcceptanceCriteria: i.AcceptanceCriteria,
	}
}

// CreateTask writes a task item via a conditional PutItem
// (attribute_not_exists(PK)) so an existing task is never overwritten.
func (r *DynamoRepository) CreateTask(ctx context.Context, userID string, t domain.Task) (domain.Task, error) {
	if t.ID == "" {
		t.ID = domain.NewID()
	}

	item, err := attributevalue.MarshalMap(toTaskItem(userID, t))
	if err != nil {
		return domain.Task{}, fmt.Errorf("marshal task item: %w", err)
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.table),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			return domain.Task{}, ErrDuplicateID
		}
		return domain.Task{}, fmt.Errorf("put task item: %w", err)
	}

	return t, nil
}

// GetTask fetches a task by userID, projectID, and ID. Returns ErrNotFound
// if no such item exists — including when the task belongs to a different
// user or a different project, since that item lives under a different
// PK/SK entirely.
func (r *DynamoRepository) GetTask(ctx context.Context, userID, projectID, taskID string) (domain.Task, error) {
	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: userPK(userID)},
			"SK": &types.AttributeValueMemberS{Value: taskSK(projectID, taskID)},
		},
	})
	if err != nil {
		return domain.Task{}, fmt.Errorf("get task item: %w", err)
	}
	if out.Item == nil {
		return domain.Task{}, ErrNotFound
	}

	var item taskItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return domain.Task{}, fmt.Errorf("unmarshal task item: %w", err)
	}

	return item.toDomain(), nil
}

// ListTasks queries the user's partition for one project's task items
// (architecture.md §8: `PK = USER#<uid> AND begins_with(SK,
// "P#<pid>#TASK#")`). No GSI is needed — the base table already scopes the
// result to userID, and the SK prefix further scopes it to one project.
func (r *DynamoRepository) ListTasks(ctx context.Context, userID, projectID string) ([]domain.Task, error) {
	tasks := make([]domain.Task, 0)

	paginator := dynamodb.NewQueryPaginator(r.client, &dynamodb.QueryInput{
		TableName:              aws.String(r.table),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :skPrefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":       &types.AttributeValueMemberS{Value: userPK(userID)},
			":skPrefix": &types.AttributeValueMemberS{Value: taskListSKPrefix(projectID)},
		},
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("query task items: %w", err)
		}

		var items []taskItem
		if err := attributevalue.UnmarshalListOfMaps(page.Items, &items); err != nil {
			return nil, fmt.Errorf("unmarshal task items: %w", err)
		}
		for _, item := range items {
			tasks = append(tasks, item.toDomain())
		}
	}

	return tasks, nil
}

// changesetItem is the DynamoDB item shape for a changeset. Unlike
// taskItem, UserID here just mirrors domain.Changeset.UserID rather than
// being carried solely for key-building — Changeset owns its UserID
// directly.
type changesetItem struct {
	PK            string                `dynamodbav:"PK"`
	SK            string                `dynamodbav:"SK"`
	UserID        string                `dynamodbav:"user_id"`
	ID            string                `dynamodbav:"id"`
	ProjectID     string                `dynamodbav:"project_id"`
	Skill         string                `dynamodbav:"skill"`
	BaseVersion   int                   `dynamodbav:"base_version"`
	Status        string                `dynamodbav:"status"`
	ProposedTasks []domain.ProposedTask `dynamodbav:"proposed_tasks,omitempty"`
	CreatedAt     time.Time             `dynamodbav:"created_at"`
}

func toChangesetItem(c domain.Changeset) changesetItem {
	return changesetItem{
		PK:            userPK(c.UserID),
		SK:            changesetSK(c.ProjectID, c.ID),
		UserID:        c.UserID,
		ID:            c.ID,
		ProjectID:     c.ProjectID,
		Skill:         c.Skill,
		BaseVersion:   c.BaseVersion,
		Status:        string(c.Status),
		ProposedTasks: c.ProposedTasks,
		CreatedAt:     c.CreatedAt,
	}
}

func (i changesetItem) toDomain() domain.Changeset {
	return domain.Changeset{
		ID:            i.ID,
		ProjectID:     i.ProjectID,
		UserID:        i.UserID,
		Skill:         i.Skill,
		BaseVersion:   i.BaseVersion,
		Status:        domain.ChangesetStatus(i.Status),
		ProposedTasks: i.ProposedTasks,
		CreatedAt:     i.CreatedAt,
	}
}

// CreateChangeset writes a changeset item via a conditional PutItem
// (attribute_not_exists(PK)) so an existing changeset is never overwritten.
func (r *DynamoRepository) CreateChangeset(ctx context.Context, c domain.Changeset) (domain.Changeset, error) {
	if c.ID == "" {
		c.ID = domain.NewID()
	}
	c.CreatedAt = time.Now().UTC()

	item, err := attributevalue.MarshalMap(toChangesetItem(c))
	if err != nil {
		return domain.Changeset{}, fmt.Errorf("marshal changeset item: %w", err)
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.table),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			return domain.Changeset{}, ErrDuplicateID
		}
		return domain.Changeset{}, fmt.Errorf("put changeset item: %w", err)
	}

	return c, nil
}

// GetChangeset fetches a changeset by userID, projectID, and ID. Returns
// ErrNotFound if no such item exists — including when it belongs to a
// different user or a different project, since that item lives under a
// different PK/SK entirely.
func (r *DynamoRepository) GetChangeset(ctx context.Context, userID, projectID, changesetID string) (domain.Changeset, error) {
	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: userPK(userID)},
			"SK": &types.AttributeValueMemberS{Value: changesetSK(projectID, changesetID)},
		},
	})
	if err != nil {
		return domain.Changeset{}, fmt.Errorf("get changeset item: %w", err)
	}
	if out.Item == nil {
		return domain.Changeset{}, ErrNotFound
	}

	var item changesetItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return domain.Changeset{}, fmt.Errorf("unmarshal changeset item: %w", err)
	}

	return item.toDomain(), nil
}

// UpdateChangesetStatus sets a changeset's status via a conditional
// UpdateItem (attribute_exists(PK)). This is an unconditional status set —
// enforcing which prior states may transition to which next state belongs
// to the changeset-accept endpoint, not this primitive.
func (r *DynamoRepository) UpdateChangesetStatus(ctx context.Context, userID, projectID, changesetID string, status domain.ChangesetStatus) (domain.Changeset, error) {
	statusAV, err := attributevalue.Marshal(string(status))
	if err != nil {
		return domain.Changeset{}, fmt.Errorf("marshal status: %w", err)
	}

	out, err := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: userPK(userID)},
			"SK": &types.AttributeValueMemberS{Value: changesetSK(projectID, changesetID)},
		},
		UpdateExpression:          aws.String("SET #status = :status"),
		ConditionExpression:       aws.String("attribute_exists(PK)"),
		ExpressionAttributeNames:  map[string]string{"#status": "status"},
		ExpressionAttributeValues: map[string]types.AttributeValue{":status": statusAV},
		ReturnValues:              types.ReturnValueAllNew,
	})
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			return domain.Changeset{}, ErrNotFound
		}
		return domain.Changeset{}, fmt.Errorf("update changeset item: %w", err)
	}

	var item changesetItem
	if err := attributevalue.UnmarshalMap(out.Attributes, &item); err != nil {
		return domain.Changeset{}, fmt.Errorf("unmarshal changeset item: %w", err)
	}

	return item.toDomain(), nil
}
