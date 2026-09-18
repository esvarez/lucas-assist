// Package llm wraps the OpenAI client and the strict-mode JSON schema
// helper shared by every skill.
package llm

import (
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// Client is constructed once via Init and reused across Lambda
// invocations.
var Client openai.Client

// Init constructs Client from an explicit API key. Every entrypoint that
// runs skills must call it once before serving requests, and before
// Client is otherwise used — cmd/local reads OPENAI_API_KEY directly;
// cmd/skills resolves it from SSM Parameter Store first (architecture.md
// §16). Kept explicit rather than a package-level var reading the
// environment, since the Lambda case needs a network call to resolve the
// key before the client can be built.
func Init(apiKey string) {
	Client = openai.NewClient(option.WithAPIKey(apiKey))
}
