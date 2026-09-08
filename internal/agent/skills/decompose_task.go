// Package skills holds one file per agent skill.
package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/openai/openai-go"

	"github.com/esvarez/lucas-assist/internal/agent"
	"github.com/esvarez/lucas-assist/internal/domain"
	"github.com/esvarez/lucas-assist/internal/llm"
)

// Domain picks which system prompt frames the decomposition.
type Domain string

const (
	DomainGeneral  Domain = "general"
	DomainSoftware Domain = "software"
)

// Clarification is one prior round's question with the user's answer
// attached — a settled decision, not more free text to parse. "No",
// "none", "no preference", and "out of scope" are answers, not missing
// information (see #41).
type Clarification struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// DecomposeInput is what the caller knows about the task to break down.
// There's no datastore yet, so this comes straight from the request body.
type DecomposeInput struct {
	TaskTitle       string `json:"task_title"`
	TaskDescription string `json:"task_description"`
	// Domain selects the system prompt. Empty or unrecognized falls back
	// to DomainGeneral.
	Domain Domain `json:"domain"`

	// ClarificationRound is 0 on the first call. A caller resubmitting
	// with Clarifications answered increments it. Round 1+ must never
	// come back needs_clarification (see systemPrompt) — asking twice
	// isn't available to the model.
	ClarificationRound int `json:"clarification_round"`

	// Clarifications are the prior round's questions with answers
	// attached, so the model sees them as resolved instead of as more
	// prose to question again.
	Clarifications []Clarification `json:"clarifications"`
}

// DecomposeResult is the model's proposed changeset. Subtasks, Questions,
// and Assumptions are always present in the JSON (never omitted), with
// unused branches explicitly null — strict mode disallows anyOf at the
// schema root, so the discriminated union is a status field with nullable
// branches instead.
type DecomposeResult struct {
	Status      string                `json:"status" jsonschema:"enum=ok,enum=needs_clarification,description=ok if the task was clear enough to decompose; needs_clarification if not"`
	Subtasks    []domain.ProposedTask `json:"subtasks" jsonschema:"nullable"`
	Assumptions []string              `json:"assumptions" jsonschema:"nullable,description=Specific choices made on the user's behalf because they were unspecified (e.g. 'SQLite via mattn/go-sqlite3 for local storage; no sync or server component'). Null when status is needs_clarification."`
	Questions   []string              `json:"questions" jsonschema:"nullable,description=Questions to ask the user before this task can be decomposed. Only used on clarification_round 0, and only when what is being built can't be identified at all."`
}

const decomposeSystemPromptGeneral = `You break any personal task into small, concrete subtasks someone can act on one at a time, each with a title, description, and acceptance criteria. The task could be about anything — a household chore, a move, a career change, an event to plan. Don't assume a domain.

Order the subtasks the way they should be done, and size each to something doable in one sitting — fold anything smaller into a neighbor, and split anything that would need its own decomposition. Never emit a subtask that's only planning ("research options", "decide on a venue") — make the decision yourself, record it in assumptions, and write the subtask that acts on it.

Always return status "ok" with 3-7 subtasks, plus assumptions: every choice you made that the user didn't specify. Be specific — "Booking a mid-size venue for about 30 people," not "assuming a standard venue." Disclose what you deliberately left out, not only what you put in. "No", "none", "no preference", and "out of scope" are decisions, not missing information — record the resulting choice as an assumption, never ask about it again.

Only return status "needs_clarification" — and only on clarification_round 0 — if you cannot identify what the task actually is at all, not merely how to do it. At most 3 questions, and only if the answer would change which subtasks exist, not merely their contents.

If clarification_round is greater than 0, you MUST return status "ok". Asking again is not available to you — where information is still missing, choose a sensible default and record it in assumptions. Never re-ask anything already present in clarifications.`

const decomposeSystemPromptSoftware = `You break a software development task into small, concrete subtasks an indie developer can ship one at a time, each with a title, description, and acceptance criteria.

Order by dependency, and make the first subtask produce something runnable. Slice vertically ("create a task and see it in the list"), never horizontally ("build all the data models"). Size each subtask to one focused session, roughly one to four hours — fold anything smaller into a neighbor, and split anything that would need its own decomposition. Never emit a subtask that is only planning ("research the options", "design the schema") — make the decision yourself, record it in assumptions, and write the subtask that builds the thing.

Always return status "ok" with 3-7 subtasks, plus assumptions: every choice you made that the user didn't specify — stack, storage, library, scope boundary, level of polish. Be specific and name names — "Using SQLite via mattn/go-sqlite3 for local storage; no sync or server component," not "assuming a standard stack." Disclose what you deliberately left out, not only what you put in. "No", "none", "no preference", and "out of scope" are decisions, not missing information — record the resulting choice as an assumption, never ask about it again.

Stay inside the task you were given. Don't add subtasks for deployment, CI, monitoring, or documentation unless the task mentions them — name them in assumptions as out of scope instead.

Only return status "needs_clarification" — and only on clarification_round 0 — if you cannot identify what is being built at all, not merely how. A missing stack, scale, audience, or polish level is never grounds to ask; those are assumptions. At most 3 questions, and only if the answer would change which subtasks exist, not merely their contents.

If clarification_round is greater than 0, you MUST return status "ok". Asking again is not available to you — where information is still missing, choose a sensible default and record it in assumptions. Never re-ask anything already present in clarifications.`

func systemPrompt(d Domain) string {
	if d == DomainSoftware {
		return decomposeSystemPromptSoftware
	}
	return decomposeSystemPromptGeneral
}

// buildUserMessage renders the task and, if this is a follow-up round,
// the prior round's answered clarifications — marked explicitly as
// settled so the model doesn't re-derive or re-question them (#41).
func buildUserMessage(in DecomposeInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Title: %s\n\nDescription: %s", in.TaskTitle, in.TaskDescription)

	if len(in.Clarifications) > 0 {
		b.WriteString("\n\nClarification round: ")
		b.WriteString(strconv.Itoa(in.ClarificationRound))
		b.WriteString("\n\nThese questions have already been answered. Treat every answer below as a settled decision, including \"no\", \"none\", \"no preference\", and \"out of scope\" — do not ask any of them again:\n")
		for _, c := range in.Clarifications {
			fmt.Fprintf(&b, "- Q: %s\n  A: %s\n", c.Question, c.Answer)
		}
	}

	return b.String()
}

// DecomposeTaskSkill implements agent.Skill for decompose_task.
type DecomposeTaskSkill struct{}

// Name implements agent.Skill.
func (DecomposeTaskSkill) Name() string { return "decompose_task" }

// BuildContext implements agent.Skill: strictly decodes the request body
// (unknown fields rejected — a caller's field-name typo should be a loud
// 400, not a silently-dropped value, see #41) and assembles the chat
// messages — stable content (system prompt) first, variable content (the
// task, and any prior clarifications) last (architecture.md §6).
func (DecomposeTaskSkill) BuildContext(_ context.Context, rawInput json.RawMessage) ([]openai.ChatCompletionMessageParamUnion, error) {
	var in DecomposeInput
	dec := json.NewDecoder(bytes.NewReader(rawInput))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return nil, fmt.Errorf("decompose_task: unmarshal input: %w: %w", agent.ErrInvalidInput, err)
	}

	return []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt(in.Domain)),
		openai.UserMessage(buildUserMessage(in)),
	}, nil
}

// ResponseFormat implements agent.Skill.
func (DecomposeTaskSkill) ResponseFormat() openai.ResponseFormatJSONSchemaParam {
	return openai.ResponseFormatJSONSchemaParam{
		JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
			Name:   "decompose_task_result",
			Strict: openai.Bool(true),
			Schema: llm.StrictSchema(&DecomposeResult{}),
		},
	}
}

// Tools implements agent.Skill. decompose_task needs no context-retrieval
// tools yet — there's no datastore to query.
func (DecomposeTaskSkill) Tools() []openai.ChatCompletionToolParam { return nil }

// Parse implements agent.Skill.
func (DecomposeTaskSkill) Parse(raw json.RawMessage) (any, error) {
	var result DecomposeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decompose_task: unmarshal model output: %w", err)
	}
	return result, nil
}
