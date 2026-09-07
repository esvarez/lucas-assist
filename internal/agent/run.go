package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go"

	"github.com/esvarez/lucas-assist/internal/llm"
)

// Model is used for every skill call for now. Split per-skill once cost or
// quality needs actually diverge (architecture.md §6).
const Model = openai.ChatModelGPT4o

// newChatCompletion is a seam over llm.Client.Chat.Completions.New so
// tests can stub the OpenAI call instead of hitting the network.
var newChatCompletion = func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	return llm.Client.Chat.Completions.New(ctx, params)
}

// Run executes a skill end to end: BuildContext assembles the chat
// messages, one OpenAI call is made with the skill's ResponseFormat and
// Tools, and Parse turns the raw output into the skill's result.
func Run(ctx context.Context, s Skill, rawInput json.RawMessage) (any, error) {
	messages, err := s.BuildContext(ctx, rawInput)
	if err != nil {
		return nil, fmt.Errorf("%s: build context: %w", s.Name(), err)
	}

	responseFormat := s.ResponseFormat()
	completion, err := newChatCompletion(ctx, openai.ChatCompletionNewParams{
		Model:    Model,
		Messages: messages,
		Tools:    s.Tools(),
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &responseFormat,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("%s: chat completion: %w", s.Name(), err)
	}

	result, err := s.Parse(json.RawMessage(completion.Choices[0].Message.Content))
	if err != nil {
		return nil, fmt.Errorf("%s: parse: %w", s.Name(), err)
	}
	return result, nil
}
