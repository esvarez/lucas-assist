// Package llm wraps the OpenAI client and the strict-mode JSON schema
// helper shared by every skill.
package llm

import "github.com/openai/openai-go"

// Client is constructed once at package init and reused across Lambda
// invocations. It reads OPENAI_API_KEY from the environment.
var Client = openai.NewClient()
