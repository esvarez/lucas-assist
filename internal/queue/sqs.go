// Package queue enqueues AgentRun jobs onto SQS (architecture.md §10/§11).
// It only knows how to send — the Agent Worker that reads these messages
// back off the queue (#101) uses aws-lambda-go/events.SQSEvent instead,
// which is already a dependency and needs nothing from this package.
package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// Message is the JSON body of a queued job message. UserID and ProjectID
// travel alongside RunID because the Agent Worker (#101) needs them to
// look the run up at all — domain.AgentRun's DynamoDB key is built from
// (userID, projectID, runID), not runID alone (the same reason GET
// /agent-runs/{id}, #100, takes project_id as a query parameter). Kept as
// a struct rather than separate scalar args so the envelope can grow
// later without a wire-format break.
type Message struct {
	RunID     string `json:"run_id"`
	UserID    string `json:"user_id"`
	ProjectID string `json:"project_id,omitempty"`
}

// sendMessageAPI is the slice of *sqs.Client this package actually calls.
// Enqueuer holds this interface, not the concrete client, so tests can
// stub SendMessage instead of making a real AWS call (AGENTS.MD: no
// network calls in CI) — the same seam OpenAI calls use
// (internal/agent/run.go's newChatCompletion), adapted for a client with
// methods instead of a free function.
type sendMessageAPI interface {
	SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
}

// Enqueuer sends AgentRun job messages to one SQS queue.
type Enqueuer struct {
	client   sendMessageAPI
	queueURL string
}

// NewEnqueuer wraps an existing SQS client and queue URL. The client
// should be constructed once (e.g. via NewClient) and reused across
// invocations, same convention as the DynamoDB and OpenAI clients
// (AGENTS.MD's Go conventions).
func NewEnqueuer(client sendMessageAPI, queueURL string) *Enqueuer {
	return &Enqueuer{client: client, queueURL: queueURL}
}

// EnqueueRun sends a job message referencing runID onto the queue.
func (e *Enqueuer) EnqueueRun(ctx context.Context, userID, projectID, runID string) error {
	body, err := json.Marshal(Message{RunID: runID, UserID: userID, ProjectID: projectID})
	if err != nil {
		return fmt.Errorf("marshal message for run %q: %w", runID, err)
	}

	_, err = e.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(e.queueURL),
		MessageBody: aws.String(string(body)),
	})
	if err != nil {
		return fmt.Errorf("enqueue run %q: %w", runID, err)
	}
	return nil
}

// NewClient builds an SQS client targeting the real AWS endpoint, resolved
// via the default AWS credential chain (IAM auth, no VPC — architecture.md
// §9).
func NewClient(ctx context.Context) (*sqs.Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	return sqs.NewFromConfig(cfg), nil
}
