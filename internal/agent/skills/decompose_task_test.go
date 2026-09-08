package skills

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/esvarez/lucas-assist/internal/agent"
	"github.com/esvarez/lucas-assist/internal/llm"
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
