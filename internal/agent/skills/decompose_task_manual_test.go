package skills

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/esvarez/lucas-assist/internal/agent"
)

// These tests hit the real OpenAI API. They're skipped unless
// OPENAI_API_KEY is set, so `go test ./...` and CI never call OpenAI —
// run explicitly with:
//
//	OPENAI_API_KEY=sk-... go test ./internal/agent/skills -run TestDecomposeManual -v

func skipUnlessOpenAIKey(t *testing.T) {
	t.Helper()
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("OPENAI_API_KEY not set; skipping real OpenAI call")
	}
}

func logDecomposeResult(t *testing.T, result DecomposeResult) {
	t.Helper()
	t.Logf("status: %s", result.Status)
	for _, st := range result.Subtasks {
		t.Logf("subtask: %+v", st)
	}
	for _, a := range result.Assumptions {
		t.Logf("assumption: %s", a)
	}
	for _, q := range result.Questions {
		t.Logf("question: %s", q)
	}
}

func runDecompose(t *testing.T, in DecomposeInput) DecomposeResult {
	t.Helper()

	rawInput, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	got, err := agent.Run(context.Background(), DecomposeTaskSkill{}, rawInput)
	if err != nil {
		t.Fatalf("agent.Run() error = %v", err)
	}
	result, ok := got.(DecomposeResult)
	if !ok {
		t.Fatalf("agent.Run() returned %T, want DecomposeResult", got)
	}
	return result
}

func TestDecomposeManual(t *testing.T) {
	skipUnlessOpenAIKey(t)

	result := runDecompose(t, DecomposeInput{
		TaskTitle:       "Clean my room",
		TaskDescription: "It's been a couple weeks, there's laundry and clutter everywhere",
		Domain:          DomainGeneral,
	})
	logDecomposeResult(t, result)

	if result.Status != "ok" && result.Status != "needs_clarification" {
		t.Errorf("Status = %q, want %q or %q", result.Status, "ok", "needs_clarification")
	}
}

// TestDecomposeManual_TitleOnly checks how the skill behaves with just a
// title and no description — the minimal input a caller might send. This
// is exactly the kind of vague input the "needs_clarification" branch
// exists for.
func TestDecomposeManual_TitleOnly(t *testing.T) {
	skipUnlessOpenAIKey(t)

	result := runDecompose(t, DecomposeInput{
		TaskTitle: "Clean my room",
		Domain:    DomainGeneral,
	})
	logDecomposeResult(t, result)

	if result.Status != "ok" && result.Status != "needs_clarification" {
		t.Errorf("Status = %q, want %q or %q", result.Status, "ok", "needs_clarification")
	}
}

// TestDecomposeManual_Regression41 replays the exact round-1 payload from
// #41: a follow-up call with clarification_round 1 and the reported
// answers ("no preference", "out of scope", etc.) as structured
// Clarifications. Before the fix this looped forever, re-asking the same
// questions — clarification_round > 0 must now always return "ok".
func TestDecomposeManual_Regression41(t *testing.T) {
	skipUnlessOpenAIKey(t)

	result := runDecompose(t, DecomposeInput{
		TaskTitle:          "IndieDev Task Tracker",
		TaskDescription:    "Create a CLI tool for indie developers to manage and track tasks",
		Domain:             DomainSoftware,
		ClarificationRound: 1,
		Clarifications: []Clarification{
			{Question: "What specific features do you want (e.g., task creation, editing, deletion)?", Answer: "Create and editing"},
			{Question: "Preferred technology stack?", Answer: "No preference"},
			{Question: "Authentication?", Answer: "Out of scope"},
			{Question: "Target audience?", Answer: "No preference"},
			{Question: "Existing trackers for inspiration?", Answer: "No"},
		},
	})
	logDecomposeResult(t, result)

	if result.Status != "ok" {
		t.Fatalf("Status = %q, want %q (clarification_round > 0 must not re-ask)", result.Status, "ok")
	}
	if len(result.Subtasks) < 3 || len(result.Subtasks) > 7 {
		t.Errorf("Subtasks = %d, want 3-7", len(result.Subtasks))
	}
	if len(result.Assumptions) == 0 {
		t.Error("Assumptions is empty, want the unspecified choices (stack, storage, etc.) recorded")
	}
	if result.Questions != nil {
		t.Errorf("Questions = %#v, want nil on a round-1 ok result", result.Questions)
	}
}
