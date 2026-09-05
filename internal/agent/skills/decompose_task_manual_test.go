package skills

import (
	"context"
	"os"
	"testing"
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

func TestDecomposeManual(t *testing.T) {
	skipUnlessOpenAIKey(t)

	result, err := Decompose(context.Background(), DecomposeInput{
		TaskTitle:       "Clean my room",
		TaskDescription: "It's been a couple weeks, there's laundry and clutter everywhere",
		Domain:          DomainGeneral,
	})
	if err != nil {
		t.Fatalf("Decompose() error = %v", err)
	}
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

	result, err := Decompose(context.Background(), DecomposeInput{
		TaskTitle: "Clean my room",
		Domain:    DomainGeneral,
	})
	if err != nil {
		t.Fatalf("Decompose() error = %v", err)
	}
	logDecomposeResult(t, result)

	if result.Status != "ok" && result.Status != "needs_clarification" {
		t.Errorf("Status = %q, want %q or %q", result.Status, "ok", "needs_clarification")
	}
}
