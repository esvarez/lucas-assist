package skills

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/esvarez/lucas-assist/internal/agent"
	"github.com/esvarez/lucas-assist/internal/domain"
	"github.com/esvarez/lucas-assist/internal/llm"
	"github.com/esvarez/lucas-assist/internal/store"
)

func TestSystemPrompt(t *testing.T) {
	cases := []struct {
		name   string
		domain Domain
		want   string
	}{
		{"software", DomainSoftware, decomposeSystemPromptSoftware},
		{"general", DomainGeneral, decomposeSystemPromptGeneral},
		{"empty falls back to general", Domain(""), decomposeSystemPromptGeneral},
		{"unrecognized falls back to general", Domain("gardening"), decomposeSystemPromptGeneral},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := systemPrompt(tc.domain); got != tc.want {
				t.Errorf("systemPrompt(%q) = %q, want %q", tc.domain, got, tc.want)
			}
		})
	}
}

// TestDecomposeResultSchemaStrictMode asserts the DecomposeResult schema
// round-trips through llm.StrictSchema into the shape OpenAI's strict mode
// requires: additionalProperties:false and every property required,
// recursively, with optional fields expressed as a nullable anyOf.
func TestDecomposeResultSchemaStrictMode(t *testing.T) {
	schema := llm.StrictSchema(&DecomposeResult{})

	assertObjectStrict(t, "root", schema, []string{"status", "subtasks", "assumptions", "questions"})

	properties, _ := schema["properties"].(map[string]any)

	for _, nullableField := range []string{"subtasks", "assumptions", "questions"} {
		anyOf, ok := properties[nullableField].(map[string]any)["anyOf"].([]any)
		if !ok || len(anyOf) != 2 {
			t.Fatalf("properties.%s: want a 2-branch anyOf, got %#v", nullableField, properties[nullableField])
		}
		if branch, ok := anyOf[1].(map[string]any); !ok || branch["type"] != "null" {
			t.Errorf("properties.%s: second anyOf branch should be {type: null}, got %#v", nullableField, anyOf[1])
		}
	}

	subtasksArray := properties["subtasks"].(map[string]any)["anyOf"].([]any)[0].(map[string]any)
	if subtasksArray["type"] != "array" {
		t.Fatalf("properties.subtasks anyOf[0]: want type array, got %#v", subtasksArray)
	}
	subtaskItem, ok := subtasksArray["items"].(map[string]any)
	if !ok {
		t.Fatalf("properties.subtasks anyOf[0]: missing items schema")
	}
	assertObjectStrict(t, "subtasks item (ProposedTask)", subtaskItem, []string{"title", "description", "acceptance_criteria"})

	if v := schema["$schema"]; v != nil {
		t.Errorf(`schema should not carry "$schema" (OpenAI rejects unknown keywords), got %v`, v)
	}
	if _, hasOneOf := findKey(schema, "oneOf"); hasOneOf {
		t.Errorf(`schema should not contain "oneOf" (rewritten to "anyOf" for strict mode)`)
	}
}

func assertObjectStrict(t *testing.T, label string, obj map[string]any, wantProps []string) {
	t.Helper()

	if obj["additionalProperties"] != false {
		t.Errorf("%s: additionalProperties = %v, want false", label, obj["additionalProperties"])
	}

	required, ok := obj["required"].([]any)
	if !ok {
		t.Fatalf("%s: missing required array", label)
	}
	got := make([]string, len(required))
	for i, r := range required {
		got[i] = r.(string)
	}
	for _, want := range wantProps {
		if !contains(got, want) {
			t.Errorf("%s: required = %v, missing %q", label, got, want)
		}
	}
	if len(got) != len(wantProps) {
		t.Errorf("%s: required = %v, want exactly %v", label, got, wantProps)
	}
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// findKey searches a decoded-JSON tree for a map key, used to assert
// something is absent anywhere in the schema.
func findKey(node any, key string) (any, bool) {
	switch n := node.(type) {
	case map[string]any:
		if v, ok := n[key]; ok {
			return v, true
		}
		for _, v := range n {
			if found, ok := findKey(v, key); ok {
				return found, true
			}
		}
	case []any:
		for _, v := range n {
			if found, ok := findKey(v, key); ok {
				return found, true
			}
		}
	}
	return nil, false
}

func TestDecomposeTaskSkill_Name(t *testing.T) {
	if got := (DecomposeTaskSkill{}).Name(); got != "decompose_task" {
		t.Errorf("Name() = %q, want %q", got, "decompose_task")
	}
}

func TestDecomposeTaskSkill_BuildContext(t *testing.T) {
	raw := []byte(`{"task_title": "Move apartments", "task_description": "Moving across town next month", "domain": "general"}`)

	messages, err := (DecomposeTaskSkill{}).BuildContext(context.Background(), raw)
	if err != nil {
		t.Fatalf("BuildContext() error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("BuildContext() returned %d messages, want 2", len(messages))
	}

	sysMsg := messages[0].OfSystem.Content.OfString.Value
	if sysMsg != decomposeSystemPromptGeneral {
		t.Errorf("system message = %q, want the general prompt", sysMsg)
	}
	userMsg := messages[1].OfUser.Content.OfString.Value
	if !strings.Contains(userMsg, "Move apartments") {
		t.Errorf("user message = %q, want it to contain the task title", userMsg)
	}
}

func TestDecomposeTaskSkill_BuildContext_SoftwareDomain(t *testing.T) {
	raw := []byte(`{"task_title": "Add auth", "domain": "software"}`)

	messages, err := (DecomposeTaskSkill{}).BuildContext(context.Background(), raw)
	if err != nil {
		t.Fatalf("BuildContext() error = %v", err)
	}
	sysMsg := messages[0].OfSystem.Content.OfString.Value
	if sysMsg != decomposeSystemPromptSoftware {
		t.Errorf("system message = %q, want the software prompt", sysMsg)
	}
}

func TestDecomposeTaskSkill_BuildContext_InvalidInput(t *testing.T) {
	_, err := (DecomposeTaskSkill{}).BuildContext(context.Background(), []byte("not json"))
	if err == nil {
		t.Fatal("BuildContext() error = nil, want an unmarshal error")
	}
	if !errors.Is(err, agent.ErrInvalidInput) {
		t.Errorf("BuildContext() error = %v, want it to wrap agent.ErrInvalidInput so callers can map it to 400", err)
	}
}

// TestDecomposeTaskSkill_BuildContext_RejectsUnknownFields is the #41
// regression: a mistyped field name (here, "TaskDescription" instead of
// "task_description") used to be silently dropped by lenient JSON
// decoding — the caller's answers never reached the model at all, which
// is why round 1 re-asked the same questions. It must now be a loud
// error instead of silent data loss.
func TestDecomposeTaskSkill_BuildContext_RejectsUnknownFields(t *testing.T) {
	raw := []byte(`{"task_title": "IndieDev Task Tracker", "TaskDescription": "answers here", "domain": "software"}`)

	_, err := (DecomposeTaskSkill{}).BuildContext(context.Background(), raw)
	if err == nil {
		t.Fatal("BuildContext() error = nil, want an error for the unknown field")
	}
	if !errors.Is(err, agent.ErrInvalidInput) {
		t.Errorf("BuildContext() error = %v, want it to wrap agent.ErrInvalidInput so callers can map it to 400", err)
	}
}

// TestDecomposeTaskSkill_BuildContext_WithClarifications is the other
// #41 fix: a follow-up round's answers are passed as structured
// Clarifications, not folded into TaskDescription prose, and the message
// explicitly marks them as settled — including negative answers — so the
// model doesn't ask again.
func TestDecomposeTaskSkill_BuildContext_WithClarifications(t *testing.T) {
	raw := []byte(`{
		"task_title": "IndieDev Task Tracker",
		"task_description": "A CLI tool for indie developers to manage and track tasks",
		"domain": "software",
		"clarification_round": 1,
		"clarifications": [
			{"question": "Preferred technology stack?", "answer": "No preference"},
			{"question": "Authentication?", "answer": "Out of scope"}
		]
	}`)

	messages, err := (DecomposeTaskSkill{}).BuildContext(context.Background(), raw)
	if err != nil {
		t.Fatalf("BuildContext() error = %v", err)
	}

	userMsg := messages[1].OfUser.Content.OfString.Value
	for _, want := range []string{
		"Clarification round: 1",
		"already been answered",
		"Preferred technology stack?",
		"No preference",
		"Authentication?",
		"Out of scope",
	} {
		if !strings.Contains(userMsg, want) {
			t.Errorf("user message = %q, want it to contain %q", userMsg, want)
		}
	}
}

// TestDecomposeTaskSkill_BuildContext_WithProjectContext is #78's eval
// scenario (architecture.md §16): when a project_id is given, BuildContext
// must surface the project card and existing task titles so the model can
// see — and avoid proposing — a duplicate of a task that already exists.
func TestDecomposeTaskSkill_BuildContext_WithProjectContext(t *testing.T) {
	repo := store.NewMemoryRepository()
	proj, err := repo.CreateProject(context.Background(), domain.Project{
		UserID:      "user_1",
		Name:        "Kitchen remodel",
		Goal:        "A finished kitchen by spring",
		Constraints: []string{"Budget under $20k"},
	})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := repo.CreateTask(context.Background(), "user_1", domain.Task{
		ProjectID: proj.ID,
		Title:     "Order new cabinets",
	}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	ctx := agent.WithUserID(context.Background(), "user_1")
	raw := []byte(`{"task_title": "Finish the kitchen remodel", "domain": "general", "project_id": "` + proj.ID + `"}`)

	skill := NewDecomposeTaskSkill(repo)
	messages, err := skill.BuildContext(ctx, raw)
	if err != nil {
		t.Fatalf("BuildContext() error = %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("BuildContext() returned %d messages, want 3 (domain system prompt, project context, user message)", len(messages))
	}

	projMsg := messages[1].OfSystem.Content.OfString.Value
	for _, want := range []string{"Kitchen remodel", "A finished kitchen by spring", "Budget under $20k", "Order new cabinets", "do not propose a subtask duplicating"} {
		if !strings.Contains(projMsg, want) {
			t.Errorf("project context message = %q, want it to contain %q", projMsg, want)
		}
	}

	for _, prompt := range []string{decomposeSystemPromptGeneral, decomposeSystemPromptSoftware} {
		if !strings.Contains(prompt, "none of your subtasks may duplicate one") {
			t.Errorf("system prompt %q missing the anti-duplication instruction", prompt)
		}
	}
}

// TestDecomposeTaskSkill_BuildContext_ProjectContext_Deadline documents
// that a project's deadline reaches the model when set — formatted as a
// calendar date (UTC), matching the web client's own UTC-midnight
// convention for the stored value.
func TestDecomposeTaskSkill_BuildContext_ProjectContext_Deadline(t *testing.T) {
	repo := store.NewMemoryRepository()
	deadline := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	proj, err := repo.CreateProject(context.Background(), domain.Project{
		UserID:   "user_1",
		Name:     "Kitchen remodel",
		Goal:     "A finished kitchen by spring",
		Deadline: &deadline,
	})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	ctx := agent.WithUserID(context.Background(), "user_1")
	raw := []byte(`{"task_title": "Finish the kitchen remodel", "domain": "general", "project_id": "` + proj.ID + `"}`)

	skill := NewDecomposeTaskSkill(repo)
	messages, err := skill.BuildContext(ctx, raw)
	if err != nil {
		t.Fatalf("BuildContext() error = %v", err)
	}

	projMsg := messages[1].OfSystem.Content.OfString.Value
	if !strings.Contains(projMsg, "Deadline: 2026-03-15") {
		t.Errorf("project context message = %q, want it to contain the deadline", projMsg)
	}
}

// TestDecomposeTaskSkill_BuildContext_ProjectContext_NoDeadline documents
// that an unset deadline is omitted entirely rather than rendered as a
// zero-value date.
func TestDecomposeTaskSkill_BuildContext_ProjectContext_NoDeadline(t *testing.T) {
	repo := store.NewMemoryRepository()
	proj, err := repo.CreateProject(context.Background(), domain.Project{
		UserID: "user_1",
		Name:   "Kitchen remodel",
		Goal:   "A finished kitchen by spring",
	})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	ctx := agent.WithUserID(context.Background(), "user_1")
	raw := []byte(`{"task_title": "Finish the kitchen remodel", "domain": "general", "project_id": "` + proj.ID + `"}`)

	skill := NewDecomposeTaskSkill(repo)
	messages, err := skill.BuildContext(ctx, raw)
	if err != nil {
		t.Fatalf("BuildContext() error = %v", err)
	}

	projMsg := messages[1].OfSystem.Content.OfString.Value
	if strings.Contains(projMsg, "Deadline") {
		t.Errorf("project context message = %q, want no Deadline mention when unset", projMsg)
	}
}

func TestDecomposeTaskSkill_BuildContext_ProjectContext_NoTasksYet(t *testing.T) {
	repo := store.NewMemoryRepository()
	proj, err := repo.CreateProject(context.Background(), domain.Project{UserID: "user_1", Name: "New project", Goal: "Get started"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	ctx := agent.WithUserID(context.Background(), "user_1")
	raw := []byte(`{"task_title": "First task", "project_id": "` + proj.ID + `"}`)

	messages, err := NewDecomposeTaskSkill(repo).BuildContext(ctx, raw)
	if err != nil {
		t.Fatalf("BuildContext() error = %v", err)
	}
	projMsg := messages[1].OfSystem.Content.OfString.Value
	if !strings.Contains(projMsg, "no tasks yet") {
		t.Errorf("project context message = %q, want it to say the project has no tasks yet", projMsg)
	}
}

// TestDecomposeTaskSkill_BuildContext_ProjectContext_NoUserID is a
// server-side invariant, not a client mistake: skillsapi always attaches
// the verified user ID to ctx before calling BuildContext, so a missing
// one here means this service called BuildContext wrong, not that the
// caller sent bad input — the error must not wrap agent.ErrInvalidInput.
func TestDecomposeTaskSkill_BuildContext_ProjectContext_NoUserID(t *testing.T) {
	repo := store.NewMemoryRepository()
	raw := []byte(`{"task_title": "Some task", "project_id": "proj_1"}`)

	_, err := NewDecomposeTaskSkill(repo).BuildContext(context.Background(), raw)
	if err == nil {
		t.Fatal("BuildContext() error = nil, want an error when ctx carries no user id")
	}
	if errors.Is(err, agent.ErrInvalidInput) {
		t.Errorf("BuildContext() error = %v, want it NOT to wrap agent.ErrInvalidInput — this is a server bug, not a client mistake", err)
	}
}

func TestDecomposeTaskSkill_BuildContext_ProjectContext_NotFound(t *testing.T) {
	repo := store.NewMemoryRepository()
	ctx := agent.WithUserID(context.Background(), "user_1")
	raw := []byte(`{"task_title": "Some task", "project_id": "does-not-exist"}`)

	_, err := NewDecomposeTaskSkill(repo).BuildContext(ctx, raw)
	if err == nil {
		t.Fatal("BuildContext() error = nil, want a not-found error")
	}
	if !errors.Is(err, agent.ErrInvalidInput) {
		t.Errorf("BuildContext() error = %v, want it to wrap agent.ErrInvalidInput so callers can map it to 400", err)
	}
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("BuildContext() error = %v, want it to wrap store.ErrNotFound", err)
	}
}

func TestDecomposeTaskSkill_ResponseFormat(t *testing.T) {
	rf := (DecomposeTaskSkill{}).ResponseFormat()
	if !rf.JSONSchema.Strict.Value {
		t.Error("JSONSchema.Strict = false, want true")
	}
	if rf.JSONSchema.Name != "decompose_task_result" {
		t.Errorf("JSONSchema.Name = %q, want %q", rf.JSONSchema.Name, "decompose_task_result")
	}
}

func TestDecomposeTaskSkill_Tools(t *testing.T) {
	if tools := (DecomposeTaskSkill{}).Tools(); tools != nil {
		t.Errorf("Tools() = %v, want nil", tools)
	}
}

func TestDecomposeTaskSkill_Parse_OK(t *testing.T) {
	raw := []byte(`{
		"status": "ok",
		"subtasks": [
			{"title": "Pack boxes", "description": "Box up the kitchen", "acceptance_criteria": ["All kitchen items boxed"]}
		],
		"assumptions": ["No professional movers — assuming a DIY move"],
		"questions": null
	}`)

	got, err := (DecomposeTaskSkill{}).Parse(raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	result, ok := got.(DecomposeResult)
	if !ok {
		t.Fatalf("Parse() returned %T, want DecomposeResult", got)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q, want %q", result.Status, "ok")
	}
	if len(result.Subtasks) != 1 || result.Subtasks[0].Title != "Pack boxes" {
		t.Errorf("Subtasks = %#v, want one subtask titled %q", result.Subtasks, "Pack boxes")
	}
	if len(result.Assumptions) != 1 {
		t.Errorf("Assumptions = %#v, want one assumption", result.Assumptions)
	}
	if result.Questions != nil {
		t.Errorf("Questions = %#v, want nil", result.Questions)
	}
}

func TestDecomposeTaskSkill_Parse_NeedsClarification(t *testing.T) {
	raw := []byte(`{"status": "needs_clarification", "subtasks": null, "assumptions": null, "questions": ["What is the task actually about?"]}`)

	got, err := (DecomposeTaskSkill{}).Parse(raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	result := got.(DecomposeResult)
	if result.Status != "needs_clarification" {
		t.Errorf("Status = %q, want %q", result.Status, "needs_clarification")
	}
	if result.Subtasks != nil {
		t.Errorf("Subtasks = %#v, want nil", result.Subtasks)
	}
	if result.Assumptions != nil {
		t.Errorf("Assumptions = %#v, want nil", result.Assumptions)
	}
	if len(result.Questions) != 1 {
		t.Errorf("Questions = %#v, want one question", result.Questions)
	}
}

func TestDecomposeTaskSkill_Parse_InvalidOutput(t *testing.T) {
	_, err := (DecomposeTaskSkill{}).Parse([]byte("not json"))
	if err == nil {
		t.Fatal("Parse() error = nil, want an unmarshal error")
	}
}
