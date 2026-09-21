package store

import (
	"context"
	"encoding/json"
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

// agentRunNoProjectSegment is the fixed placeholder used in place of
// <pid> for a create_project run, which has no ProjectID yet (that's the
// whole point of the run — see domain.AgentRun). Every other skill's run
// carries a real ProjectID and uses it directly.
//
// This is the concrete answer to the open question in issue #96: rather
// than giving project-less runs a second, un-prefixed item shape (which
// would need its own Query/Get code path for what's only ever one skill),
// they get the same `P#<segment>#RUN#<run_id>` item as every other run,
// just with a fixed segment instead of a real project ID. It's still
// trivially distinguishable from a real project ID since NewID never
// produces this exact string.
const agentRunNoProjectSegment = "NOPROJECT"

func agentRunProjectSegment(projectID string) string {
	if projectID == "" {
		return agentRunNoProjectSegment
	}
	return projectID
}

// agentRunSK builds an AgentRun's sort key. architecture.md §8 documents
// this as `P#<pid>#RUN#<timestamp>#<run_id>`; the timestamp segment lives
// inside run_id itself instead (domain.NewRunID), so the key GetAgentRun
// needs is derivable from (projectID, runID) alone, no separate lookup for
// the timestamp required — see NewRunID's doc comment.
func agentRunSK(projectID, runID string) string {
	return agentRunListSKPrefix(projectID) + runID
}

// agentRunListSKPrefix is the begins_with prefix that scopes a Query to one
// project's (or the create_project placeholder's) agent runs. Because
// run_id starts with a zero-padded timestamp, such a Query also returns
// runs in chronological order.
func agentRunListSKPrefix(projectID string) string {
	return projectScopeSKPrefix + agentRunProjectSegment(projectID) + "#RUN#"
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

// agentRunItem is the DynamoDB item shape for an agent run.
type agentRunItem struct {
	PK          string          `dynamodbav:"PK"`
	SK          string          `dynamodbav:"SK"`
	ID          string          `dynamodbav:"id"`
	UserID      string          `dynamodbav:"user_id"`
	ProjectID   string          `dynamodbav:"project_id,omitempty"`
	Skill       string          `dynamodbav:"skill"`
	Status      string          `dynamodbav:"status"`
	Input       json.RawMessage `dynamodbav:"input,omitempty"`
	Attempt     int             `dynamodbav:"attempt"`
	WorkerID    string          `dynamodbav:"worker_id,omitempty"`
	LeaseUntil  *time.Time      `dynamodbav:"lease_until,omitempty"`
	Error       string          `dynamodbav:"error,omitempty"`
	ChangesetID string          `dynamodbav:"changeset_id,omitempty"`
	CreatedAt   time.Time       `dynamodbav:"created_at"`
	UpdatedAt   time.Time       `dynamodbav:"updated_at"`
}

func toAgentRunItem(r domain.AgentRun) agentRunItem {
	return agentRunItem{
		PK:          userPK(r.UserID),
		SK:          agentRunSK(r.ProjectID, r.ID),
		ID:          r.ID,
		UserID:      r.UserID,
		ProjectID:   r.ProjectID,
		Skill:       r.Skill,
		Status:      string(r.Status),
		Input:       r.Input,
		Attempt:     r.Attempt,
		WorkerID:    r.WorkerID,
		LeaseUntil:  r.LeaseUntil,
		Error:       r.Error,
		ChangesetID: r.ChangesetID,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

func (i agentRunItem) toDomain() domain.AgentRun {
	return domain.AgentRun{
		ID:          i.ID,
		UserID:      i.UserID,
		ProjectID:   i.ProjectID,
		Skill:       i.Skill,
		Status:      domain.AgentRunStatus(i.Status),
		Input:       i.Input,
		Attempt:     i.Attempt,
		WorkerID:    i.WorkerID,
		LeaseUntil:  i.LeaseUntil,
		Error:       i.Error,
		ChangesetID: i.ChangesetID,
		CreatedAt:   i.CreatedAt,
		UpdatedAt:   i.UpdatedAt,
	}
}

// CreateAgentRun writes a run item via a conditional PutItem
// (attribute_not_exists(PK)) so an existing run is never overwritten. It
// always sets Status to AgentRunQueued and resets Attempt/WorkerID/
// LeaseUntil/Error, regardless of what the caller passed in run.
func (r *DynamoRepository) CreateAgentRun(ctx context.Context, run domain.AgentRun) (domain.AgentRun, error) {
	if run.ID == "" {
		run.ID = domain.NewRunID()
	}

	now := time.Now().UTC()
	run.Status = domain.AgentRunQueued
	run.Attempt = 0
	run.WorkerID = ""
	run.LeaseUntil = nil
	run.Error = ""
	run.CreatedAt = now
	run.UpdatedAt = now

	item, err := attributevalue.MarshalMap(toAgentRunItem(run))
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("marshal agent run item: %w", err)
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.table),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			return domain.AgentRun{}, ErrDuplicateID
		}
		return domain.AgentRun{}, fmt.Errorf("put agent run item: %w", err)
	}

	return run, nil
}

// GetAgentRun fetches a run by userID, projectID, and runID. Returns
// ErrNotFound if no such item exists — including when it belongs to a
// different user or a different project, since that item lives under a
// different PK/SK entirely.
func (r *DynamoRepository) GetAgentRun(ctx context.Context, userID, projectID, runID string) (domain.AgentRun, error) {
	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: userPK(userID)},
			"SK": &types.AttributeValueMemberS{Value: agentRunSK(projectID, runID)},
		},
	})
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("get agent run item: %w", err)
	}
	if out.Item == nil {
		return domain.AgentRun{}, ErrNotFound
	}

	var item agentRunItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return domain.AgentRun{}, fmt.Errorf("unmarshal agent run item: %w", err)
	}

	return item.toDomain(), nil
}

// LeaseAgentRun conditionally moves a run from queued to running, or
// reclaims a running run whose lease has already expired. The condition
// mirrors MemoryRepository's: attribute_exists(#lease_until) guards the
// expiry comparison so a queued run (which has no lease_until attribute at
// all) can't make that branch error instead of simply evaluating false.
//
// ReturnValuesOnConditionCheckFailure distinguishes "no such run"
// (ErrNotFound, no item at all) from "run exists but isn't leasable right
// now" (ErrRunLeased) the same way UpdateProject distinguishes ErrNotFound
// from ErrConflict.
func (r *DynamoRepository) LeaseAgentRun(ctx context.Context, userID, projectID, runID, workerID string, leaseUntil time.Time) (domain.AgentRun, error) {
	now := time.Now().UTC()

	runningAV, err := attributevalue.Marshal(string(domain.AgentRunRunning))
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("marshal running status: %w", err)
	}
	queuedAV, err := attributevalue.Marshal(string(domain.AgentRunQueued))
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("marshal queued status: %w", err)
	}
	workerAV, err := attributevalue.Marshal(workerID)
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("marshal worker id: %w", err)
	}
	leaseUntilAV, err := attributevalue.Marshal(leaseUntil)
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("marshal lease until: %w", err)
	}
	nowAV, err := attributevalue.Marshal(now)
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("marshal now: %w", err)
	}
	updatedAtAV, err := attributevalue.Marshal(now)
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("marshal updated_at: %w", err)
	}

	out, err := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: userPK(userID)},
			"SK": &types.AttributeValueMemberS{Value: agentRunSK(projectID, runID)},
		},
		UpdateExpression: aws.String("SET #status = :running, #worker_id = :worker_id, #lease_until = :lease_until, #updated_at = :updated_at ADD #attempt :one"),
		ConditionExpression: aws.String(
			"attribute_exists(PK) AND (#status = :queued OR (#status = :running AND attribute_exists(#lease_until) AND #lease_until < :now))",
		),
		ExpressionAttributeNames: map[string]string{
			"#status":      "status",
			"#worker_id":   "worker_id",
			"#lease_until": "lease_until",
			"#updated_at":  "updated_at",
			"#attempt":     "attempt",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":running":     runningAV,
			":queued":      queuedAV,
			":worker_id":   workerAV,
			":lease_until": leaseUntilAV,
			":updated_at":  updatedAtAV,
			":now":         nowAV,
			":one":         &types.AttributeValueMemberN{Value: "1"},
		},
		ReturnValues:                        types.ReturnValueAllNew,
		ReturnValuesOnConditionCheckFailure: types.ReturnValuesOnConditionCheckFailureAllOld,
	})
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			if len(condErr.Item) == 0 {
				return domain.AgentRun{}, ErrNotFound
			}
			return domain.AgentRun{}, ErrRunLeased
		}
		return domain.AgentRun{}, fmt.Errorf("lease agent run item: %w", err)
	}

	var item agentRunItem
	if err := attributevalue.UnmarshalMap(out.Attributes, &item); err != nil {
		return domain.AgentRun{}, fmt.Errorf("unmarshal agent run item: %w", err)
	}

	return item.toDomain(), nil
}

// CompleteAgentRun sets a run's status to completed and records
// changesetID via a conditional UpdateItem (attribute_exists(PK)). Like
// UpdateChangesetStatus, this is an unconditional status set — enforcing
// that a run was actually leased first belongs to the worker, not this
// primitive.
func (r *DynamoRepository) CompleteAgentRun(ctx context.Context, userID, projectID, runID, changesetID string) (domain.AgentRun, error) {
	return r.setAgentRunTerminalStatus(ctx, userID, projectID, runID, domain.AgentRunCompleted, "", changesetID)
}

// FailAgentRun sets a run's status to failed and records errMsg, via the
// same conditional UpdateItem as CompleteAgentRun. changesetID is always
// empty — a failed run never produced one.
func (r *DynamoRepository) FailAgentRun(ctx context.Context, userID, projectID, runID, errMsg string) (domain.AgentRun, error) {
	return r.setAgentRunTerminalStatus(ctx, userID, projectID, runID, domain.AgentRunFailed, errMsg, "")
}

func (r *DynamoRepository) setAgentRunTerminalStatus(ctx context.Context, userID, projectID, runID string, status domain.AgentRunStatus, errMsg, changesetID string) (domain.AgentRun, error) {
	now := time.Now().UTC()

	statusAV, err := attributevalue.Marshal(string(status))
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("marshal status: %w", err)
	}
	errAV, err := attributevalue.Marshal(errMsg)
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("marshal error: %w", err)
	}
	changesetIDAV, err := attributevalue.Marshal(changesetID)
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("marshal changeset id: %w", err)
	}
	updatedAtAV, err := attributevalue.Marshal(now)
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("marshal updated_at: %w", err)
	}

	out, err := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: userPK(userID)},
			"SK": &types.AttributeValueMemberS{Value: agentRunSK(projectID, runID)},
		},
		UpdateExpression:    aws.String("SET #status = :status, #error = :error, #changeset_id = :changeset_id, #updated_at = :updated_at"),
		ConditionExpression: aws.String("attribute_exists(PK)"),
		ExpressionAttributeNames: map[string]string{
			"#status":       "status",
			"#error":        "error",
			"#changeset_id": "changeset_id",
			"#updated_at":   "updated_at",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status":       statusAV,
			":error":        errAV,
			":changeset_id": changesetIDAV,
			":updated_at":   updatedAtAV,
		},
		ReturnValues: types.ReturnValueAllNew,
	})
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			return domain.AgentRun{}, ErrNotFound
		}
		return domain.AgentRun{}, fmt.Errorf("update agent run item: %w", err)
	}

	var item agentRunItem
	if err := attributevalue.UnmarshalMap(out.Attributes, &item); err != nil {
		return domain.AgentRun{}, fmt.Errorf("unmarshal agent run item: %w", err)
	}

	return item.toDomain(), nil
}
