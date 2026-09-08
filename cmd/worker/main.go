// Command worker is the Worker Lambda entrypoint for Step Functions task
// handlers (architecture.md §12): conversation ingestion and large
// decompositions run as Step Functions workflows with worker Lambdas
// rather than inline in the API or Skill Lambdas. Scaffolding only — the
// state machine definition and ingest_conversation itself are last in the
// MVP build order (architecture.md §"MVP build order") and land as
// separate work. No VPC — see architecture.md §9.
package main

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-lambda-go/lambda"
)

// handler echoes the task event back unchanged. A Step Functions Task
// state's input has no fixed shape, so json.RawMessage is the honest
// signature until a real task payload is defined alongside the state
// machine.
func handler(ctx context.Context, event json.RawMessage) (json.RawMessage, error) {
	return event, nil
}

func main() {
	lambda.Start(handler)
}
