// Command worker is the Agent Worker Lambda (architecture.md §1/§10/§11):
// it consumes AgentRun job messages off AgentJobsQueue, leases the
// referenced run, calls the model via agent.Run, and saves the result as a
// Changeset. The processing logic itself lives in internal/worker so it's
// testable against store.MemoryRepository without a real SQS event or
// OpenAI call — this file is just the Lambda-thin wrapper, mirroring how
// cmd/api and cmd/skills wrap internal/api and internal/skillsapi.
//
// This is the only function that holds the OpenAI credential
// (architecture.md §14) — SkillsFunction stopped needing it once dispatch
// became asynchronous (#99). No VPC (architecture.md §9).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"

	"github.com/esvarez/lucas-assist/internal/agent"
	"github.com/esvarez/lucas-assist/internal/agent/skills"
	"github.com/esvarez/lucas-assist/internal/domain"
	"github.com/esvarez/lucas-assist/internal/llm"
	"github.com/esvarez/lucas-assist/internal/queue"
	"github.com/esvarez/lucas-assist/internal/store"
	"github.com/esvarez/lucas-assist/internal/worker"
)

// processor is constructed once at package init and reused across
// invocations — never inside the handler.
var processor *worker.Processor

func init() {
	ctx := context.Background()

	apiKey, err := resolveOpenAIAPIKey(ctx)
	if err != nil {
		log.Fatalf("resolve OpenAI API key: %v", err)
	}
	llm.Init(apiKey)

	dynamoClient, err := store.NewDynamoDBClient(ctx)
	if err != nil {
		log.Fatalf("new dynamodb client: %v", err)
	}
	repo := store.NewDynamoRepository(dynamoClient, os.Getenv("DYNAMODB_TABLE"))

	registry := agent.NewRegistry(
		skills.DecomposeTaskSkill{},
		skills.CreateProjectSkill{},
	)

	// A fresh ID per cold start is a fine granularity for lease
	// attribution: within one execution environment, SQS-triggered
	// invocations run one at a time, so there's never a concurrent
	// conflict between messages sharing this WorkerID.
	processor = worker.NewProcessor(repo, registry, domain.NewID())
}

// resolveOpenAIAPIKey reads the key from SSM Parameter Store
// (architecture.md §16): a SecureString created out-of-band, never passed
// through the SAM template as plaintext. OPENAI_API_KEY_PARAM names the
// parameter; WorkerFunction's role holds ssm:GetParameter and kms:Decrypt
// scoped to it (template.yaml).
func resolveOpenAIAPIKey(ctx context.Context) (string, error) {
	paramName := os.Getenv("OPENAI_API_KEY_PARAM")
	if paramName == "" {
		return "", fmt.Errorf("OPENAI_API_KEY_PARAM is not set")
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("load AWS config: %w", err)
	}

	out, err := ssm.NewFromConfig(cfg).GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(paramName),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return "", fmt.Errorf("get parameter %q: %w", paramName, err)
	}
	return *out.Parameter.Value, nil
}

// handler processes every record in the batch and joins their errors.
// template.yaml sets BatchSize: 1, so in practice this is always exactly
// one record — kept as a loop rather than assuming that, since nothing
// here depends on it. Returning a non-nil error fails the whole
// invocation, which (at BatchSize 1) just means SQS redelivers this one
// message; Processor.ProcessRun's own duplicate-delivery handling makes
// that safe.
func handler(ctx context.Context, event events.SQSEvent) error {
	var errs []error
	for _, record := range event.Records {
		var msg queue.Message
		if err := json.Unmarshal([]byte(record.Body), &msg); err != nil {
			errs = append(errs, fmt.Errorf("record %s: unmarshal message: %w", record.MessageId, err))
			continue
		}
		if err := processor.ProcessRun(ctx, msg.UserID, msg.ProjectID, msg.RunID); err != nil {
			errs = append(errs, fmt.Errorf("record %s (run %s): %w", record.MessageId, msg.RunID, err))
		}
	}
	return errors.Join(errs...)
}

func main() {
	lambda.Start(handler)
}
