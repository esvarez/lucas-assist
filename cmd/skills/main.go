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
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"github.com/esvarez/lucas-assist/internal/agent"
	"github.com/esvarez/lucas-assist/internal/agent/skills"
	"github.com/esvarez/lucas-assist/internal/llm"
	"github.com/esvarez/lucas-assist/internal/skillsapi"
)

// adapter is constructed once at package init and reused across
// invocations — never inside the handler.
var adapter *httpadapter.HandlerAdapterV2

func init() {
	ctx := context.Background()

	apiKey, err := resolveOpenAIAPIKey(ctx)
	if err != nil {
		log.Fatalf("resolve OpenAI API key: %v", err)
	}
	llm.Init(apiKey)

	registry := agent.NewRegistry(
		skills.DecomposeTaskSkill{},
		skills.CreateProjectSkill{},
	)
	adapter = httpadapter.NewV2(skillsapi.NewHandler(registry))
}

// resolveOpenAIAPIKey reads the key from SSM Parameter Store (architecture.md
// §16): a SecureString created out-of-band, never passed through the SAM
// template as plaintext. OPENAI_API_KEY_PARAM names the parameter;
// SkillsFunction's role holds ssm:GetParameter and kms:Decrypt scoped to
// it (template.yaml). ApiFunction has neither the env var nor the IAM
// permission — only the skill role reads this credential (architecture.md
// §14).
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

func handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	return adapter.ProxyWithContext(ctx, req)
}

func main() {
	lambda.Start(handler)
}
