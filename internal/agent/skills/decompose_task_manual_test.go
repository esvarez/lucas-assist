package skills

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/esvarez/lucas-assist/internal/agent"
	"github.com/esvarez/lucas-assist/internal/domain"
	"github.com/esvarez/lucas-assist/internal/llm"
	"github.com/esvarez/lucas-assist/internal/store"
)

// These tests hit the real OpenAI API. They're skipped unless
// OPENAI_API_KEY is set, so `go test ./...` and CI never call OpenAI —
// run explicitly with:
//
//	OPENAI_API_KEY=sk-... go test ./internal/agent/skills -run TestDecomposeManual -v

func skipUnlessOpenAIKey(t *testing.T) {
	t.Helper()
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set; skipping real OpenAI call")
	}
	// llm.Client is a zero value until Init runs — cmd/local and
	// cmd/skills call it at startup, but these tests exercise
	// agent.Run() directly, so nothing else does.
	llm.Init(apiKey)
}

func logDecomposeResult(t *testing.T, result DecomposeResult) {
	t.Helper()
	for _, st := range result.Subtasks {
		t.Logf("subtask: %+v", st)
	}
	for _, a := range result.Assumptions {
		t.Logf("assumption: %s", a)
	}
}

func runDecompose(t *testing.T, in DecomposeInput) DecomposeResult {
	t.Helper()
	return runDecomposeWithSkill(t, context.Background(), DecomposeTaskSkill{}, in)
}

// runDecomposeWithSkill is runDecompose generalized over the skill and ctx,
// so a test can supply a NewDecomposeTaskSkill backed by a seeded repo and
// a ctx carrying agent.WithUserID — needed to exercise the project_id path
// (#78), which the plain zero-value DecomposeTaskSkill{} used by
// runDecompose above can't reach.
func runDecomposeWithSkill(t *testing.T, ctx context.Context, skill DecomposeTaskSkill, in DecomposeInput) DecomposeResult {
	t.Helper()

	rawInput, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	got, err := agent.Run(ctx, skill, rawInput)
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

	if len(result.Subtasks) < 3 || len(result.Subtasks) > 7 {
		t.Errorf("Subtasks = %d, want 3-7", len(result.Subtasks))
	}
}

// TestDecomposeManual_TitleOnly checks how the skill behaves with just a
// title and no description — the minimal input a caller might send. This
// is exactly the kind of vague input that used to trigger
// needs_clarification; #42 removed that branch, so the model must instead
// decompose its smallest defensible interpretation and disclose the
// guesswork in assumptions rather than asking.
func TestDecomposeManual_TitleOnly(t *testing.T) {
	skipUnlessOpenAIKey(t)

	result := runDecompose(t, DecomposeInput{
		TaskTitle: "Clean my room",
		Domain:    DomainGeneral,
	})
	logDecomposeResult(t, result)

	if len(result.Subtasks) < 3 || len(result.Subtasks) > 7 {
		t.Errorf("Subtasks = %d, want 3-7", len(result.Subtasks))
	}
	if len(result.Assumptions) == 0 {
		t.Error("Assumptions is empty, want the guesswork behind this minimal input disclosed")
	}
}

// TestDecomposeManual_ProjectContext_AvoidsDuplicate is #78's eval scenario
// (architecture.md §16) against the real model: given a project_id whose
// project already has a task that covers part of the requested work, the
// decomposition must not propose a subtask that duplicates it.
func TestDecomposeManual_ProjectContext_AvoidsDuplicate(t *testing.T) {
	skipUnlessOpenAIKey(t)

	repo := store.NewMemoryRepository()
	proj, err := repo.CreateProject(context.Background(), domain.Project{
		UserID: "user_1",
		Name:   "IndieDev Task Tracker",
		Goal:   "Ship a CLI tool for indie developers to manage and track tasks",
	})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	existingTitle := "Set up SQLite storage for tasks"
	if _, err := repo.CreateTask(context.Background(), "user_1", domain.Task{
		ProjectID: proj.ID,
		Title:     existingTitle,
	}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	ctx := agent.WithUserID(context.Background(), "user_1")
	result := runDecomposeWithSkill(t, ctx, NewDecomposeTaskSkill(repo), DecomposeInput{
		TaskTitle:       "Build the task tracker CLI",
		TaskDescription: "A CLI tool for indie developers to create, edit, and track tasks, including local storage",
		Domain:          DomainSoftware,
		ProjectID:       proj.ID,
	})
	logDecomposeResult(t, result)

	if len(result.Subtasks) < 3 || len(result.Subtasks) > 7 {
		t.Errorf("Subtasks = %d, want 3-7", len(result.Subtasks))
	}
	for _, st := range result.Subtasks {
		if strings.EqualFold(st.Title, existingTitle) {
			t.Errorf("Subtasks contains %q, which exactly duplicates the existing task %q", st.Title, existingTitle)
		}
	}
}
