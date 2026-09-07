package skills

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go"

	"github.com/esvarez/lucas-assist/internal/agent"
	"github.com/esvarez/lucas-assist/internal/domain"
	"github.com/esvarez/lucas-assist/internal/llm"
)

// CreateProjectInput is the raw natural-language project description the
// user gave, plus anything else they supplied up front.
type CreateProjectInput struct {
	Description string `json:"description"`
}

// CreateProjectResult is the model's proposed project card. Both Project
// and Questions are always present in the JSON (never omitted), with the
// unused branch explicitly null — same discriminated-union pattern as
// DecomposeResult (strict mode disallows anyOf at the schema root).
//
// Nothing here is persisted: per AGENTS.MD rule 1, only an explicit user
// accept against POST /projects commits a project.
type CreateProjectResult struct {
	Status    string                  `json:"status" jsonschema:"enum=ok,enum=needs_clarification,description=ok if there is enough to propose a project card; needs_clarification if not"`
	Project   *domain.ProposedProject `json:"project" jsonschema:"nullable"`
	Questions []string                `json:"questions" jsonschema:"nullable,description=Questions to ask the user before a project card can be proposed"`
}

const createProjectSystemPrompt = `You turn a natural-language description of a project into a structured project card: name, goal, deadline, and constraints.

If the description gives you enough to commit to a name and a goal, return status "ok" with the project card. Deadline and constraints are null/empty when the user didn't mention them — never invent one.

If the description is too vague to commit to even a name and a goal, return status "needs_clarification" with the questions you'd need answered first. Do not guess.`

// CreateProjectSkill implements agent.Skill for create_project. It only
// proposes a project card — nothing is persisted here (AGENTS.MD rule 1);
// an explicit user accept against POST /projects is what commits it.
type CreateProjectSkill struct{}

// Name implements agent.Skill.
func (CreateProjectSkill) Name() string { return "create_project" }

// BuildContext implements agent.Skill.
func (CreateProjectSkill) BuildContext(_ context.Context, rawInput json.RawMessage) ([]openai.ChatCompletionMessageParamUnion, error) {
	var in CreateProjectInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return nil, fmt.Errorf("create_project: unmarshal input: %w: %w", agent.ErrInvalidInput, err)
	}

	return []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(createProjectSystemPrompt),
		openai.UserMessage(in.Description),
	}, nil
}

// ResponseFormat implements agent.Skill.
func (CreateProjectSkill) ResponseFormat() openai.ResponseFormatJSONSchemaParam {
	return openai.ResponseFormatJSONSchemaParam{
		JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
			Name:   "create_project_result",
			Strict: openai.Bool(true),
			Schema: llm.StrictSchema(&CreateProjectResult{}),
		},
	}
}

// Tools implements agent.Skill. create_project needs no context-retrieval
// tools — there's no datastore to query.
func (CreateProjectSkill) Tools() []openai.ChatCompletionToolParam { return nil }

// Parse implements agent.Skill.
func (CreateProjectSkill) Parse(raw json.RawMessage) (any, error) {
	var result CreateProjectResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("create_project: unmarshal model output: %w", err)
	}
	return result, nil
}
