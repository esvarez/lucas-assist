// Package skills holds one file per agent skill.
package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/openai/openai-go"

	"github.com/esvarez/lucas-assist/internal/agent"
	"github.com/esvarez/lucas-assist/internal/domain"
	"github.com/esvarez/lucas-assist/internal/llm"
	"github.com/esvarez/lucas-assist/internal/store"
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
type DecomposeInput struct {
	TaskTitle       string `json:"task_title"`
	TaskDescription string `json:"task_description"`
	// Domain selects the system prompt. Empty or unrecognized falls back
	// to DomainGeneral.
	Domain Domain `json:"domain"`

	// ProjectID is optional — when set, BuildContext loads the project and
	// its existing tasks and includes them in the prompt (architecture.md
	// §7's project card + active task subset), so the model doesn't
	// propose a subtask duplicating one that already exists. Omit it for
	// ad-hoc decomposition with no project behind it yet.
	ProjectID string `json:"project_id"`

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
	Questions   []string              `json:"questions" jsonschema:"nullable,description=Questions to ask the user before this task can be decomposed. Only used on clarification_round 0 and only when what is being built can't be identified at all."`
}

const decomposeSystemPromptGeneral = `You break any personal task into small, concrete subtasks someone can act on one at a time, each with a title, description, and acceptance criteria. The task could be about anything — a household chore, a move, a career change, an event to plan. Don't assume a domain.

Order the subtasks the way they should be done, and size each to something doable in one sitting — fold anything smaller into a neighbor, and split anything that would need its own decomposition. Never emit a subtask that's only planning ("research options", "decide on a venue") — make the decision yourself, record it in assumptions, and write the subtask that acts on it.

Always return status "ok" with 3-7 subtasks, plus assumptions: every choice you made that the user didn't specify. Be specific — "Booking a mid-size venue for about 30 people," not "assuming a standard venue." Disclose what you deliberately left out, not only what you put in. "No", "none", "no preference", and "out of scope" are decisions, not missing information — record the resulting choice as an assumption, never ask about it again.

Only return status "needs_clarification" — and only on clarification_round 0 — if you cannot identify what the task actually is at all, not merely how to do it. At most 3 questions, and only if the answer would change which subtasks exist, not merely their contents.

If clarification_round is greater than 0, you MUST return status "ok". Asking again is not available to you — where information is still missing, choose a sensible default and record it in assumptions. Never re-ask anything already present in clarifications.

If existing tasks for this project are listed below, none of your subtasks may duplicate one — check titles and intent, not just exact wording — and decompose only the work not already covered. This overrides the 3-7 count above: return fewer than 3 if that's all the remaining work supports, rather than padding, inventing busywork, or restating an existing task.

Write every title, description, acceptance criterion, assumption, and question in the same language as the task title and description you were given — not necessarily English.`

const decomposeSystemPromptSoftware = `You break a software development task into small, concrete subtasks an indie developer can ship one at a time, each with a title, description, and acceptance criteria.

Order by dependency, and make the first subtask produce something runnable. Slice vertically ("create a task and see it in the list"), never horizontally ("build all the data models"). Size each subtask to one focused session, roughly one to four hours — fold anything smaller into a neighbor, and split anything that would need its own decomposition. Never emit a subtask that is only planning ("research the options", "design the schema") — make the decision yourself, record it in assumptions, and write the subtask that builds the thing.

Always return status "ok" with 3-7 subtasks, plus assumptions: every choice you made that the user didn't specify — stack, storage, library, scope boundary, level of polish. Be specific and name names — "Using SQLite via mattn/go-sqlite3 for local storage; no sync or server component," not "assuming a standard stack." Disclose what you deliberately left out, not only what you put in. "No", "none", "no preference", and "out of scope" are decisions, not missing information — record the resulting choice as an assumption, never ask about it again.

Stay inside the task you were given. Don't add subtasks for deployment, CI, monitoring, or documentation unless the task mentions them — name them in assumptions as out of scope instead.

Only return status "needs_clarification" — and only on clarification_round 0 — if you cannot identify what is being built at all, not merely how. A missing stack, scale, audience, or polish level is never grounds to ask; those are assumptions. At most 3 questions, and only if the answer would change which subtasks exist, not merely their contents.

If clarification_round is greater than 0, you MUST return status "ok". Asking again is not available to you — where information is still missing, choose a sensible default and record it in assumptions. Never re-ask anything already present in clarifications.

If existing tasks for this project are listed below, none of your subtasks may duplicate one — check titles and intent, not just exact wording — and decompose only the work not already covered. This overrides the 3-7 count above: return fewer than 3 if that's all the remaining work supports, rather than padding, inventing busywork, or restating an existing task.

Write every title, description, acceptance criterion, assumption, and question in the same language as the task title and description you were given — not necessarily English.`

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
	fmt.Fprintf(&b, "\n\nClarification round: %d", in.ClarificationRound)

	if len(in.Clarifications) > 0 {
		b.WriteString("\n\nThese questions have already been answered. Treat every answer below as a settled decision, including \"no\", \"none\", \"no preference\", and \"out of scope\" — do not ask any of them again:\n")
		for _, c := range in.Clarifications {
			fmt.Fprintf(&b, "- Q: %s\n  A: %s\n", c.Question, c.Answer)
		}
	}

	return b.String()
}

// buildProjectContextMessage renders the project card and existing task
// titles (architecture.md §7's "always included" project card plus the
// "active task subset") so the model can see what already exists and
// avoid proposing a duplicate. It's a separate system message, placed
// after the domain system prompt but before the user's task — stable per
// project, so it changes far less often than the per-call task
// (architecture.md §6, prompt-caching).
func buildProjectContextMessage(proj domain.Project, tasks []domain.Task) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Project: %s\n\nGoal: %s", proj.Name, proj.Goal)
	if proj.Deadline != nil {
		fmt.Fprintf(&b, "\n\nDeadline: %s", proj.Deadline.UTC().Format("2006-01-02"))
	}
	if len(proj.Constraints) > 0 {
		b.WriteString("\n\nConstraints:\n")
		for _, c := range proj.Constraints {
			fmt.Fprintf(&b, "- %s\n", c)
		}
	}

	if len(tasks) > 0 {
		b.WriteString("\nExisting tasks for this project (do not propose a subtask duplicating any of these):\n")
		for _, t := range tasks {
			fmt.Fprintf(&b, "- %s\n", t.Title)
		}
	} else {
		b.WriteString("\nThis project has no tasks yet.\n")
	}

	return b.String()
}

// DecomposeTaskSkill implements agent.Skill for decompose_task.
type DecomposeTaskSkill struct {
	repo store.Repository
}

// NewDecomposeTaskSkill builds a DecomposeTaskSkill backed by repo, used by
// BuildContext to load the project and its existing tasks when the input
// carries a ProjectID.
func NewDecomposeTaskSkill(repo store.Repository) DecomposeTaskSkill {
	return DecomposeTaskSkill{repo: repo}
}

// Name implements agent.Skill.
func (DecomposeTaskSkill) Name() string { return "decompose_task" }

// BuildContext implements agent.Skill: strictly decodes the request body
// (unknown fields rejected — a caller's field-name typo should be a loud
// 400, not a silently-dropped value, see #41) and assembles the chat
// messages — stable content (system prompt, then project context) first,
// variable content (the task, and any prior clarifications) last
// (architecture.md §6).
//
// When the input carries a ProjectID, the caller must have attached the
// verified user ID to ctx via agent.WithUserID first — every
// store.Repository lookup is scoped by user (architecture.md §8), and
// unlike a malformed request body, a missing context user ID is this
// service's own bug, not the caller's.
func (d DecomposeTaskSkill) BuildContext(ctx context.Context, rawInput json.RawMessage) ([]openai.ChatCompletionMessageParamUnion, error) {
	var in DecomposeInput
	dec := json.NewDecoder(bytes.NewReader(rawInput))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return nil, fmt.Errorf("decompose_task: unmarshal input: %w: %w", agent.ErrInvalidInput, err)
	}

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt(in.Domain)),
	}

	if in.ProjectID != "" {
		userID, ok := agent.UserIDFromContext(ctx)
		if !ok {
			return nil, fmt.Errorf("decompose_task: build context: no user id in context")
		}

		proj, err := d.repo.GetProject(ctx, userID, in.ProjectID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, fmt.Errorf("decompose_task: project %q not found: %w: %w", in.ProjectID, agent.ErrInvalidInput, err)
			}
			return nil, fmt.Errorf("decompose_task: get project: %w", err)
		}

		tasks, err := d.repo.ListTasks(ctx, userID, in.ProjectID)
		if err != nil {
			return nil, fmt.Errorf("decompose_task: list tasks: %w", err)
		}

		messages = append(messages, openai.SystemMessage(buildProjectContextMessage(proj, tasks)))
	}

	messages = append(messages, openai.UserMessage(buildUserMessage(in)))
	return messages, nil
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
