package skills

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/esvarez/lucas-assist/internal/agent"
	"github.com/esvarez/lucas-assist/internal/llm"
)

// TestCreateProjectResultSchemaStrictMode asserts the CreateProjectResult
// schema round-trips through llm.StrictSchema into the shape OpenAI's
// strict mode requires: additionalProperties:false and every property
// required, recursively, with optional fields expressed as a nullable
// anyOf (including the nested ProposedProject and its own Deadline field).
func TestCreateProjectResultSchemaStrictMode(t *testing.T) {
	schema := llm.StrictSchema(&CreateProjectResult{})

	assertObjectStrict(t, "root", schema, []string{"status", "project", "questions"})

	properties, _ := schema["properties"].(map[string]any)

	for _, nullableField := range []string{"project", "questions"} {
		anyOf, ok := properties[nullableField].(map[string]any)["anyOf"].([]any)
		if !ok || len(anyOf) != 2 {
			t.Fatalf("properties.%s: want a 2-branch anyOf, got %#v", nullableField, properties[nullableField])
		}
		if branch, ok := anyOf[1].(map[string]any); !ok || branch["type"] != "null" {
			t.Errorf("properties.%s: second anyOf branch should be {type: null}, got %#v", nullableField, anyOf[1])
		}
	}

	projectObj := properties["project"].(map[string]any)["anyOf"].([]any)[0].(map[string]any)
	assertObjectStrict(t, "project (ProposedProject)", projectObj, []string{"name", "goal", "deadline", "constraints"})

	deadlineAnyOf, ok := projectObj["properties"].(map[string]any)["deadline"].(map[string]any)["anyOf"].([]any)
	if !ok || len(deadlineAnyOf) != 2 {
		t.Fatalf("project.deadline: want a 2-branch anyOf, got %#v", projectObj["properties"].(map[string]any)["deadline"])
	}
	if branch, ok := deadlineAnyOf[1].(map[string]any); !ok || branch["type"] != "null" {
		t.Errorf("project.deadline: second anyOf branch should be {type: null}, got %#v", deadlineAnyOf[1])
	}

	if v := schema["$schema"]; v != nil {
		t.Errorf(`schema should not carry "$schema" (OpenAI rejects unknown keywords), got %v`, v)
	}
	if _, hasOneOf := findKey(schema, "oneOf"); hasOneOf {
		t.Errorf(`schema should not contain "oneOf" (rewritten to "anyOf" for strict mode)`)
	}
}

func TestCreateProjectSkill_Name(t *testing.T) {
	if got := (CreateProjectSkill{}).Name(); got != "create_project" {
		t.Errorf("Name() = %q, want %q", got, "create_project")
	}
}

func TestCreateProjectSkill_BuildContext(t *testing.T) {
	raw := []byte(`{"description": "A CLI tool for indie developers to track tasks, shipping by end of Q2"}`)

	messages, err := (CreateProjectSkill{}).BuildContext(context.Background(), raw)
	if err != nil {
		t.Fatalf("BuildContext() error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("BuildContext() returned %d messages, want 2", len(messages))
	}

	sysMsg := messages[0].OfSystem.Content.OfString.Value
	if sysMsg != createProjectSystemPrompt {
		t.Errorf("system message = %q, want the create_project system prompt", sysMsg)
	}
	userMsg := messages[1].OfUser.Content.OfString.Value
	if !strings.Contains(userMsg, "track tasks") {
		t.Errorf("user message = %q, want it to contain the description", userMsg)
	}
}

func TestCreateProjectSkill_BuildContext_InvalidInput(t *testing.T) {
	_, err := (CreateProjectSkill{}).BuildContext(context.Background(), []byte("not json"))
	if err == nil {
		t.Fatal("BuildContext() error = nil, want an unmarshal error")
	}
	if !errors.Is(err, agent.ErrInvalidInput) {
		t.Errorf("BuildContext() error = %v, want it to wrap agent.ErrInvalidInput so callers can map it to 400", err)
	}
}

func TestCreateProjectSkill_ResponseFormat(t *testing.T) {
	rf := (CreateProjectSkill{}).ResponseFormat()
	if !rf.JSONSchema.Strict.Value {
		t.Error("JSONSchema.Strict = false, want true")
	}
	if rf.JSONSchema.Name != "create_project_result" {
		t.Errorf("JSONSchema.Name = %q, want %q", rf.JSONSchema.Name, "create_project_result")
	}
}

func TestCreateProjectSkill_Tools(t *testing.T) {
	if tools := (CreateProjectSkill{}).Tools(); tools != nil {
		t.Errorf("Tools() = %v, want nil", tools)
	}
}

func TestCreateProjectSkill_Parse_OK(t *testing.T) {
	raw := []byte(`{
		"status": "ok",
		"project": {
			"name": "Nudge",
			"goal": "Ship a POC that validates the Task struct",
			"deadline": null,
			"constraints": ["Go backend", "no VPC"]
		},
		"questions": null
	}`)

	got, err := (CreateProjectSkill{}).Parse(raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	result, ok := got.(CreateProjectResult)
	if !ok {
		t.Fatalf("Parse() returned %T, want CreateProjectResult", got)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q, want %q", result.Status, "ok")
	}
	if result.Project == nil {
		t.Fatal("Project = nil, want a proposed project")
	}
	if result.Project.Name != "Nudge" {
		t.Errorf("Project.Name = %q, want %q", result.Project.Name, "Nudge")
	}
	if result.Project.Deadline != nil {
		t.Errorf("Project.Deadline = %v, want nil", result.Project.Deadline)
	}
	if result.Questions != nil {
		t.Errorf("Questions = %#v, want nil", result.Questions)
	}
}

func TestCreateProjectSkill_Parse_NeedsClarification(t *testing.T) {
	raw := []byte(`{"status": "needs_clarification", "project": null, "questions": ["What should this project ship?"]}`)

	got, err := (CreateProjectSkill{}).Parse(raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	result := got.(CreateProjectResult)
	if result.Status != "needs_clarification" {
		t.Errorf("Status = %q, want %q", result.Status, "needs_clarification")
	}
	if result.Project != nil {
		t.Errorf("Project = %#v, want nil", result.Project)
	}
	if len(result.Questions) != 1 {
		t.Errorf("Questions = %#v, want one question", result.Questions)
	}
}

func TestCreateProjectSkill_Parse_InvalidOutput(t *testing.T) {
	_, err := (CreateProjectSkill{}).Parse([]byte("not json"))
	if err == nil {
		t.Fatal("Parse() error = nil, want an unmarshal error")
	}
}
