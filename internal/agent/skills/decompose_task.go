// Package skills holds one file per agent skill.
package skills

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/shared"

	"github.com/esvarez/lucas-assist/internal/domain"
	"github.com/esvarez/lucas-assist/internal/llm"
)

// Domain picks which system prompt frames the decomposition.
type Domain string

const (
	DomainGeneral  Domain = "general"
	DomainSoftware Domain = "software"
)

// DecomposeInput is what the caller knows about the task to break down.
// The POC has no datastore, so this is passed in directly rather than
// assembled from project context.
type DecomposeInput struct {
	TaskTitle       string
	TaskDescription string
	// Domain selects the system prompt. Empty or unrecognized falls back
	// to DomainGeneral.
	Domain Domain
}

// DecomposeResult is the model's proposed changeset. Both Subtasks and
// Questions are always present in the JSON (never omitted), with the
// unused branch explicitly null — strict mode disallows anyOf at the
// schema root, so the discriminated union is a status field with nullable
// branches instead.
type DecomposeResult struct {
	Status    string                `json:"status" jsonschema:"enum=ok,enum=needs_clarification,description=ok if the task was clear enough to decompose; needs_clarification if not"`
	Subtasks  []domain.ProposedTask `json:"subtasks" jsonschema:"nullable"`
	Questions []string              `json:"questions" jsonschema:"nullable,description=Questions to ask the user before this task can be decomposed"`
}

const decomposeSystemPromptGeneral = `You break any personal task into small, concrete subtasks someone can act on one at a time. The task could be about anything — a household chore, a move, a career change, an event to plan. Don't assume a domain.

If the task is specific enough, return status "ok" with 3-7 subtasks, each with a clear title, description, and acceptance criteria. Order them the way they should be done.

If the task is too vague to decompose responsibly, return status "needs_clarification" with the questions you'd need answered first. Do not guess.`

const decomposeSystemPromptSoftware = `You break a software development task into small, concrete subtasks an indie developer can ship one at a time.

If the task is specific enough, return status "ok" with 3-7 subtasks, each with a clear title, description, and acceptance criteria. Order them the way they should be done — think in terms of a buildable, testable increment per subtask.

If the task is too vague to decompose responsibly, return status "needs_clarification" with the questions you'd need answered first. Do not guess at stack, scope, or requirements.`

func systemPrompt(d Domain) string {
	if d == DomainSoftware {
		return decomposeSystemPromptSoftware
	}
	return decomposeSystemPromptGeneral
}

// newChatCompletion is a seam over llm.Client.Chat.Completions.New so tests
// can stub the OpenAI call instead of hitting the network.
var newChatCompletion = func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	return llm.Client.Chat.Completions.New(ctx, params)
}

// Decompose calls OpenAI to break TaskTitle/TaskDescription into subtasks.
func Decompose(ctx context.Context, in DecomposeInput) (DecomposeResult, error) {
	completion, err := newChatCompletion(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModelGPT4o,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt(in.Domain)),
			openai.UserMessage(fmt.Sprintf("Title: %s\n\nDescription: %s", in.TaskTitle, in.TaskDescription)),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
				JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "decompose_task_result",
					Strict: openai.Bool(true),
					Schema: llm.StrictSchema(&DecomposeResult{}),
				},
			},
		},
	})
	if err != nil {
		return DecomposeResult{}, fmt.Errorf("decompose_task: chat completion: %w", err)
	}

	var result DecomposeResult
	if err := json.Unmarshal([]byte(completion.Choices[0].Message.Content), &result); err != nil {
		return DecomposeResult{}, fmt.Errorf("decompose_task: unmarshal model output: %w", err)
	}
	return result, nil
}
