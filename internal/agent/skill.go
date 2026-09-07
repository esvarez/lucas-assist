// Package agent holds the Skill abstraction and the generic dispatcher
// that drives every skill through it (architecture.md §4): adding a skill
// is one new file implementing Skill, not new OpenAI-calling plumbing.
package agent

import (
	"context"
	"encoding/json"

	"github.com/openai/openai-go"
)

// Skill is the uniform abstraction every skill implements.
type Skill interface {
	Name() string

	// BuildContext turns the raw per-skill request body into the chat
	// messages sent to the model — stable content first, variable content
	// last (architecture.md §6, prompt-caching). There's no datastore yet,
	// so "context" here is just the request body; once one exists, this is
	// where a skill would query it.
	BuildContext(ctx context.Context, rawInput json.RawMessage) ([]openai.ChatCompletionMessageParamUnion, error)

	// ResponseFormat is the strict json_schema shape the model's answer
	// must conform to.
	ResponseFormat() openai.ResponseFormatJSONSchemaParam

	// Tools are reserved for context retrieval only, never for carrying
	// the result.
	Tools() []openai.ChatCompletionToolParam

	// Parse turns the model's raw JSON content into the skill's result.
	Parse(raw json.RawMessage) (any, error)
}
