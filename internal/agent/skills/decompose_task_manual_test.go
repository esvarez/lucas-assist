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
