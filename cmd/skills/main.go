// Command skills is the consolidated Skill Lambda entrypoint, exposed via
// a Function URL (architecture.md's POC scope; API Gateway routing to
// this Lambda is a separate infra concern, tracked apart from this).
// Dispatches to the agent.Skill named in the request body — adding a
// skill means adding it to the registry below, not a new binary. The
// dispatch handler itself lives in internal/skillsapi, shared with
// cmd/local, so it isn't duplicated (mirrors cmd/api/internal/api).
//
// Dispatch is asynchronous (architecture.md §1/§10, issue #99): this
// Lambda only validates the request, creates a queued AgentRun, and
// enqueues it — it never calls OpenAI itself, so unlike before #99 it
// holds no OpenAI credential. The Agent Worker (#101) is what leases the
// run and makes the model call, matching architecture.md §14's "only the
// Worker reads the OpenAI credential."
package main

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"github.com/esvarez/lucas-assist/internal/agent"
	"github.com/esvarez/lucas-assist/internal/agent/skills"
	"github.com/esvarez/lucas-assist/internal/queue"
	"github.com/esvarez/lucas-assist/internal/skillsapi"
	"github.com/esvarez/lucas-assist/internal/store"
)

// adapter is constructed once at package init and reused across
// invocations — never inside the handler.
var adapter *httpadapter.HandlerAdapterV2

func init() {
	ctx := context.Background()

	dynamoClient, err := store.NewDynamoDBClient(ctx)
	if err != nil {
		log.Fatalf("new dynamodb client: %v", err)
	}
	repo := store.NewDynamoRepository(dynamoClient, os.Getenv("DYNAMODB_TABLE"))

	sqsClient, err := queue.NewClient(ctx)
	if err != nil {
		log.Fatalf("new sqs client: %v", err)
	}
	enqueuer := queue.NewEnqueuer(sqsClient, os.Getenv("AGENT_JOBS_QUEUE_URL"))

	registry := agent.NewRegistry(
		skills.DecomposeTaskSkill{},
		skills.CreateProjectSkill{},
	)
	adapter = httpadapter.NewV2(skillsapi.NewHandler(registry, repo, enqueuer))
}

func handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	return adapter.ProxyWithContext(ctx, req)
}

func main() {
	lambda.Start(handler)
}
