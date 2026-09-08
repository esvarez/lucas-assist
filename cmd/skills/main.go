// Command skills is the consolidated Skill Lambda entrypoint, exposed via
// a Function URL (architecture.md's POC scope; API Gateway routing to
// this Lambda is a separate infra concern, tracked apart from this).
// Dispatches to the agent.Skill named in the request body — adding a
// skill means adding it to the registry below, not a new binary. The
// dispatch handler itself lives in internal/skillsapi, shared with
// cmd/local, so it isn't duplicated (mirrors cmd/api/internal/api).
package main

import (
	"context"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"github.com/esvarez/lucas-assist/internal/agent"
	"github.com/esvarez/lucas-assist/internal/agent/skills"
	"github.com/esvarez/lucas-assist/internal/skillsapi"
)

// adapter is constructed once at package init and reused across
// invocations — never inside the handler.
var adapter *httpadapter.HandlerAdapterV2

func init() {
	registry := agent.NewRegistry(
		skills.DecomposeTaskSkill{},
		skills.CreateProjectSkill{},
	)
	adapter = httpadapter.NewV2(skillsapi.NewHandler(registry))
}

func handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	return adapter.ProxyWithContext(ctx, req)
}

func main() {
	lambda.Start(handler)
}
