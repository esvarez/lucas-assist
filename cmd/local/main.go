// Command local runs the same router/handlers as cmd/api and cmd/skills
// behind a plain http.Server on :8080 — no SAM local emulation. Only the
// lambda.Start wrapper differs between those Lambdas and this
// (architecture.md §11).
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"

	"github.com/esvarez/lucas-assist/internal/agent"
	"github.com/esvarez/lucas-assist/internal/agent/skills"
	"github.com/esvarez/lucas-assist/internal/api"
	"github.com/esvarez/lucas-assist/internal/auth"
	"github.com/esvarez/lucas-assist/internal/queue"
	"github.com/esvarez/lucas-assist/internal/skillsapi"
	"github.com/esvarez/lucas-assist/internal/store"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	ctx := context.Background()

	endpoint := getenv("DYNAMODB_ENDPOINT", "http://localhost:8000")
	client, err := store.NewLocalDynamoDBClient(ctx, endpoint)
	if err != nil {
		log.Fatalf("new dynamodb client: %v", err)
	}

	table := getenv("DYNAMODB_TABLE", "nudge-local")
	if err := store.EnsureTable(ctx, client, table); err != nil {
		log.Fatalf("ensure table %q: %v", table, err)
	}

	repo := store.NewDynamoRepository(client, table)
	router := api.NewRouter(repo)

	// Skill dispatch is asynchronous (architecture.md §1/§10, issue #99):
	// it creates a queued AgentRun and enqueues it rather than calling
	// OpenAI inline, so there's no local OpenAI wiring here anymore either
	// — see cmd/skills for the same change. Unlike DynamoDB, there's no
	// local SQS emulator yet, so AGENT_JOBS_QUEUE_URL must point at a real
	// queue (e.g. a personal dev deployment's) for POST /skills to work
	// against this server; the real AWS credential chain applies, same as
	// production.
	sqsClient, err := queue.NewClient(ctx)
	if err != nil {
		log.Fatalf("new sqs client: %v", err)
	}
	enqueuer := queue.NewEnqueuer(sqsClient, os.Getenv("AGENT_JOBS_QUEUE_URL"))

	registry := agent.NewRegistry(
		skills.NewDecomposeTaskSkill(repo),
		skills.CreateProjectSkill{},
	)
	router.Handle("POST /skills", skillsapi.NewHandler(registry, repo, enqueuer))

	// No API Gateway JWT authorizer sits in front of this plain
	// http.Server, so there's no already-verified claim to read the way
	// LambdaJWTResolver does in production. CognitoGetUserResolver
	// verifies the presented bearer token by asking the real Cognito User
	// Pool directly (cognito-idp:GetUser) instead — same posture as
	// everything else cmd/local does against real AWS services (SQS is
	// already real, not emulated; only DynamoDB is local).
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("load AWS config: %v", err)
	}
	cognitoClient := cognitoidentityprovider.NewFromConfig(awsCfg)
	protected := auth.Middleware(auth.NewCognitoGetUserResolver(cognitoClient))(router)

	addr := ":8080"
	log.Printf("listening on %s (dynamodb endpoint %s, table %s)", addr, endpoint, table)
	log.Fatal(http.ListenAndServe(addr, protected))
}
