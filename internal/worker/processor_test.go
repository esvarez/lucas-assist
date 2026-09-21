package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/openai/openai-go"

	"github.com/esvarez/lucas-assist/internal/agent"
	"github.com/esvarez/lucas-assist/internal/agent/skills"
	"github.com/esvarez/lucas-assist/internal/domain"
	"github.com/esvarez/lucas-assist/internal/store"
)

// fakeSkill implements agent.Skill without any real OpenAI interaction —
// BuildContext/ResponseFormat/Tools are never exercised by ProcessRun
// (agent.Run itself is stubbed via runSkill), only Name matters for
// registry lookup.
type fakeSkill struct{ name string }

func (f fakeSkill) Name() string { return f.name }
func (f fakeSkill) BuildContext(context.Context, json.RawMessage) ([]openai.ChatCompletionMessageParamUnion, error) {
	return nil, nil
}
func (f fakeSkill) ResponseFormat() openai.ResponseFormatJSONSchemaParam {
	return openai.ResponseFormatJSONSchemaParam{}
}
func (f fakeSkill) Tools() []openai.ChatCompletionToolParam { return nil }
func (f fakeSkill) Parse(json.RawMessage) (any, error)      { return nil, nil }

func newTestProcessor(repo RunRepository, run runSkillFunc, reg *agent.Registry) *Processor {
	return &Processor{
		Repo:     repo,
		Registry: reg,
		WorkerID: "worker_1",
		LeaseFor: time.Minute,
		runSkill: run,
	}
}

// stubRunRepository wraps a real RunRepository (a *store.MemoryRepository
// in every test here) and overrides just GetProject, so a test can inject
// a transient infrastructure failure without needing MemoryRepository
// itself to support one.
type stubRunRepository struct {
	RunRepository
	getProjectErr error
}

func (s stubRunRepository) GetProject(ctx context.Context, userID, id string) (domain.Project, error) {
	if s.getProjectErr != nil {
		return domain.Project{}, s.getProjectErr
	}
	return s.RunRepository.GetProject(ctx, userID, id)
}

func TestProcessor_ProcessRun_DecomposeTask_Success(t *testing.T) {
	repo := store.NewMemoryRepository()
	ctx := context.Background()

	project, err := repo.CreateProject(ctx, domain.Project{UserID: "user_1", Name: "Nudge"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	run, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: "user_1", ProjectID: project.ID, Skill: "decompose_task", Input: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	stubResult := skills.DecomposeResult{
		Status:   "ok",
		Subtasks: []domain.ProposedTask{{Title: "Add login command"}, {Title: "Add logout command"}},
	}
	runSkill := func(ctx context.Context, s agent.Skill, raw json.RawMessage) (any, error) {
		return stubResult, nil
	}

	p := newTestProcessor(repo, runSkill, agent.NewRegistry(fakeSkill{name: "decompose_task"}))

	if err := p.ProcessRun(ctx, "user_1", project.ID, run.ID); err != nil {
		t.Fatalf("ProcessRun() error = %v", err)
	}

	got, err := repo.GetAgentRun(ctx, "user_1", project.ID, run.ID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if got.Status != domain.AgentRunCompleted {
		t.Fatalf("Status = %q, want %q", got.Status, domain.AgentRunCompleted)
	}
	if got.ChangesetID == "" {
		t.Fatal("ChangesetID = \"\", want the saved changeset's ID")
	}

	changeset, err := repo.GetChangeset(ctx, "user_1", project.ID, got.ChangesetID)
	if err != nil {
		t.Fatalf("GetChangeset() error = %v", err)
	}
	if changeset.Status != domain.ChangesetProposed {
		t.Errorf("Changeset.Status = %q, want %q", changeset.Status, domain.ChangesetProposed)
	}
	if len(changeset.ProposedTasks) != 2 {
		t.Errorf("Changeset.ProposedTasks = %+v, want 2 tasks", changeset.ProposedTasks)
	}
	if changeset.BaseVersion != project.Version {
		t.Errorf("Changeset.BaseVersion = %d, want the project's version %d", changeset.BaseVersion, project.Version)
	}
}

func TestProcessor_ProcessRun_CreateProject_Success(t *testing.T) {
	repo := store.NewMemoryRepository()
	ctx := context.Background()

	// create_project runs have no ProjectID (#96) - that's the point.
	run, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: "user_1", Skill: "create_project", Input: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	stubResult := skills.CreateProjectResult{
		Status:  "ok",
		Project: &domain.ProposedProject{Name: "Nudge", Goal: "Ship the POC"},
	}
	runSkill := func(ctx context.Context, s agent.Skill, raw json.RawMessage) (any, error) {
		return stubResult, nil
	}

	p := newTestProcessor(repo, runSkill, agent.NewRegistry(fakeSkill{name: "create_project"}))

	if err := p.ProcessRun(ctx, "user_1", "", run.ID); err != nil {
		t.Fatalf("ProcessRun() error = %v", err)
	}

	got, err := repo.GetAgentRun(ctx, "user_1", "", run.ID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if got.Status != domain.AgentRunCompleted {
		t.Fatalf("Status = %q, want %q", got.Status, domain.AgentRunCompleted)
	}

	changeset, err := repo.GetChangeset(ctx, "user_1", "", got.ChangesetID)
	if err != nil {
		t.Fatalf("GetChangeset() error = %v", err)
	}
	if changeset.ProposedProject == nil || changeset.ProposedProject.Name != "Nudge" {
		t.Errorf("Changeset.ProposedProject = %+v, want the proposed project", changeset.ProposedProject)
	}
}

// TestProcessor_ProcessRun_DuplicateDelivery_NoOp documents the #101
// acceptance criterion: a duplicate delivery of an already-completed run
// is a no-op - no second model call, no second Changeset.
func TestProcessor_ProcessRun_DuplicateDelivery_NoOp(t *testing.T) {
	repo := store.NewMemoryRepository()
	ctx := context.Background()

	project, err := repo.CreateProject(ctx, domain.Project{UserID: "user_1", Name: "Nudge"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	run, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: "user_1", ProjectID: project.ID, Skill: "decompose_task", Input: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	calls := 0
	runSkill := func(ctx context.Context, s agent.Skill, raw json.RawMessage) (any, error) {
		calls++
		return skills.DecomposeResult{Status: "ok", Subtasks: []domain.ProposedTask{{Title: "Task"}}}, nil
	}
	p := newTestProcessor(repo, runSkill, agent.NewRegistry(fakeSkill{name: "decompose_task"}))

	if err := p.ProcessRun(ctx, "user_1", project.ID, run.ID); err != nil {
		t.Fatalf("first ProcessRun() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("runSkill called %d times after first ProcessRun(), want 1", calls)
	}

	completed, err := repo.GetAgentRun(ctx, "user_1", project.ID, run.ID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}

	// Duplicate delivery of the same (now-completed) run.
	if err := p.ProcessRun(ctx, "user_1", project.ID, run.ID); err != nil {
		t.Fatalf("replayed ProcessRun() error = %v", err)
	}
	if calls != 1 {
		t.Errorf("runSkill called %d times after replayed ProcessRun(), want still 1 (no second model call)", calls)
	}

	after, err := repo.GetAgentRun(ctx, "user_1", project.ID, run.ID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if after.ChangesetID != completed.ChangesetID {
		t.Errorf("ChangesetID changed after replay: %q -> %q, want unchanged (no second Changeset)", completed.ChangesetID, after.ChangesetID)
	}
}

// TestProcessor_ProcessRun_LeaseHeld_NoOp documents that a run currently
// leased by another attempt is left alone - SQS's visibility timeout
// handles redelivery, not a race against the worker that has it.
func TestProcessor_ProcessRun_LeaseHeld_NoOp(t *testing.T) {
	repo := store.NewMemoryRepository()
	ctx := context.Background()

	run, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: "user_1", ProjectID: "proj_1", Skill: "decompose_task"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}
	if _, err := repo.LeaseAgentRun(ctx, "user_1", "proj_1", run.ID, "other_worker", time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("LeaseAgentRun() error = %v", err)
	}

	calls := 0
	runSkill := func(ctx context.Context, s agent.Skill, raw json.RawMessage) (any, error) {
		calls++
		return nil, nil
	}
	p := newTestProcessor(repo, runSkill, agent.NewRegistry(fakeSkill{name: "decompose_task"}))

	if err := p.ProcessRun(ctx, "user_1", "proj_1", run.ID); err != nil {
		t.Fatalf("ProcessRun() error = %v", err)
	}
	if calls != 0 {
		t.Errorf("runSkill called %d times, want 0 (another attempt holds the lease)", calls)
	}

	got, err := repo.GetAgentRun(ctx, "user_1", "proj_1", run.ID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if got.WorkerID != "other_worker" {
		t.Errorf("WorkerID = %q, want unchanged %q", got.WorkerID, "other_worker")
	}
}

// TestProcessor_ProcessRun_ModelFailure_ReturnsErrorForRetry documents the
// fix for a PR #120 review finding: a transient failure (the model call
// itself — rate limit, 5xx, timeout) must not permanently fail the run.
// It's returned as a real error instead, leaving the run "running" so
// AgentJobsQueue's RedrivePolicy (architecture.md §15) gets a chance to
// retry before the message ever reaches the DLQ. Marking it failed
// immediately would burn the job's one shot on a blip.
func TestProcessor_ProcessRun_ModelFailure_ReturnsErrorForRetry(t *testing.T) {
	repo := store.NewMemoryRepository()
	ctx := context.Background()

	run, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: "user_1", ProjectID: "proj_1", Skill: "decompose_task"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	modelErr := errors.New("openai: rate limited")
	runSkill := func(ctx context.Context, s agent.Skill, raw json.RawMessage) (any, error) {
		return nil, modelErr
	}
	p := newTestProcessor(repo, runSkill, agent.NewRegistry(fakeSkill{name: "decompose_task"}))

	err = p.ProcessRun(ctx, "user_1", "proj_1", run.ID)
	if !errors.Is(err, modelErr) {
		t.Fatalf("ProcessRun() error = %v, want wrapped %v (a transient failure must be returned so SQS retries it)", err, modelErr)
	}

	got, err := repo.GetAgentRun(ctx, "user_1", "proj_1", run.ID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if got.Status != domain.AgentRunRunning {
		t.Fatalf("Status = %q, want %q (still running, not permanently failed)", got.Status, domain.AgentRunRunning)
	}
}

// TestProcessor_ProcessRun_LoadProjectFailure_ReturnsErrorForRetry covers
// the same PR #120 finding for the DynamoDB read between a successful
// model call and saving the changeset: also transient, also must not
// discard a model result the run already paid for.
func TestProcessor_ProcessRun_LoadProjectFailure_ReturnsErrorForRetry(t *testing.T) {
	memRepo := store.NewMemoryRepository()
	ctx := context.Background()

	project, err := memRepo.CreateProject(ctx, domain.Project{UserID: "user_1", Name: "Nudge"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	run, err := memRepo.CreateAgentRun(ctx, domain.AgentRun{UserID: "user_1", ProjectID: project.ID, Skill: "decompose_task"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	dbErr := errors.New("dynamo unavailable")
	repo := stubRunRepository{RunRepository: memRepo, getProjectErr: dbErr}
	runSkill := func(ctx context.Context, s agent.Skill, raw json.RawMessage) (any, error) {
		return skills.DecomposeResult{Status: "ok", Subtasks: []domain.ProposedTask{{Title: "Task"}}}, nil
	}
	p := newTestProcessor(repo, runSkill, agent.NewRegistry(fakeSkill{name: "decompose_task"}))

	err = p.ProcessRun(ctx, "user_1", project.ID, run.ID)
	if !errors.Is(err, dbErr) {
		t.Fatalf("ProcessRun() error = %v, want wrapped %v", err, dbErr)
	}

	got, err := memRepo.GetAgentRun(ctx, "user_1", project.ID, run.ID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if got.Status != domain.AgentRunRunning {
		t.Errorf("Status = %q, want %q (transient failure, not permanently failed)", got.Status, domain.AgentRunRunning)
	}
}

// TestProcessor_ProcessRun_ChangesetAlreadyCreated_Reused documents the
// fix for a PR #120 review finding: if a prior attempt saved a Changeset
// but crashed (or itself hit a transient error) before calling
// CompleteAgentRun, a retry must reuse that changeset — via its
// deterministic, run-ID-derived ID — rather than saving a second, duplicate
// proposal.
func TestProcessor_ProcessRun_ChangesetAlreadyCreated_Reused(t *testing.T) {
	repo := store.NewMemoryRepository()
	ctx := context.Background()

	project, err := repo.CreateProject(ctx, domain.Project{UserID: "user_1", Name: "Nudge"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	run, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: "user_1", ProjectID: project.ID, Skill: "decompose_task"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	// Simulate a prior attempt that saved the changeset but never reached
	// CompleteAgentRun.
	preExisting, err := repo.CreateChangeset(ctx, domain.Changeset{
		ID:            run.ID,
		UserID:        "user_1",
		ProjectID:     project.ID,
		Skill:         "decompose_task",
		BaseVersion:   project.Version,
		Status:        domain.ChangesetProposed,
		ProposedTasks: []domain.ProposedTask{{Title: "Original task"}},
	})
	if err != nil {
		t.Fatalf("CreateChangeset() error = %v", err)
	}

	calls := 0
	runSkill := func(ctx context.Context, s agent.Skill, raw json.RawMessage) (any, error) {
		calls++
		return skills.DecomposeResult{Status: "ok", Subtasks: []domain.ProposedTask{{Title: "A different task from the retry"}}}, nil
	}
	p := newTestProcessor(repo, runSkill, agent.NewRegistry(fakeSkill{name: "decompose_task"}))

	if err := p.ProcessRun(ctx, "user_1", project.ID, run.ID); err != nil {
		t.Fatalf("ProcessRun() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("runSkill called %d times, want 1", calls)
	}

	got, err := repo.GetAgentRun(ctx, "user_1", project.ID, run.ID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if got.Status != domain.AgentRunCompleted {
		t.Fatalf("Status = %q, want %q", got.Status, domain.AgentRunCompleted)
	}
	if got.ChangesetID != preExisting.ID {
		t.Fatalf("ChangesetID = %q, want the pre-existing changeset %q reused", got.ChangesetID, preExisting.ID)
	}

	changeset, err := repo.GetChangeset(ctx, "user_1", project.ID, got.ChangesetID)
	if err != nil {
		t.Fatalf("GetChangeset() error = %v", err)
	}
	if len(changeset.ProposedTasks) != 1 || changeset.ProposedTasks[0].Title != "Original task" {
		t.Errorf("Changeset.ProposedTasks = %+v, want the original pre-existing tasks preserved, not overwritten by the retry's model call", changeset.ProposedTasks)
	}
}

func TestProcessor_ProcessRun_UnknownSkill_MarksFailed(t *testing.T) {
	repo := store.NewMemoryRepository()
	ctx := context.Background()

	run, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: "user_1", ProjectID: "proj_1", Skill: "does-not-exist"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	p := newTestProcessor(repo, nil, agent.NewRegistry(fakeSkill{name: "decompose_task"}))

	if err := p.ProcessRun(ctx, "user_1", "proj_1", run.ID); err != nil {
		t.Fatalf("ProcessRun() error = %v", err)
	}

	got, err := repo.GetAgentRun(ctx, "user_1", "proj_1", run.ID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if got.Status != domain.AgentRunFailed {
		t.Fatalf("Status = %q, want %q", got.Status, domain.AgentRunFailed)
	}
}

func TestProcessor_ProcessRun_NeedsClarification_MarksFailed(t *testing.T) {
	repo := store.NewMemoryRepository()
	ctx := context.Background()

	run, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: "user_1", ProjectID: "proj_1", Skill: "decompose_task"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	runSkill := func(ctx context.Context, s agent.Skill, raw json.RawMessage) (any, error) {
		return skills.DecomposeResult{Status: "needs_clarification", Questions: []string{"what are you building?"}}, nil
	}
	p := newTestProcessor(repo, runSkill, agent.NewRegistry(fakeSkill{name: "decompose_task"}))

	if err := p.ProcessRun(ctx, "user_1", "proj_1", run.ID); err != nil {
		t.Fatalf("ProcessRun() error = %v", err)
	}

	got, err := repo.GetAgentRun(ctx, "user_1", "proj_1", run.ID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if got.Status != domain.AgentRunFailed {
		t.Fatalf("Status = %q, want %q (see #42)", got.Status, domain.AgentRunFailed)
	}
}

// TestProcessor_ProcessRun_CreateProject_NilProject_MarksFailed documents
// the fix for a PR #120 review finding: strict mode's schema can't express
// "Project is non-null when Status is ok", so a malformed model response
// could otherwise complete a create_project run with no proposal for the
// client to ever accept.
func TestProcessor_ProcessRun_CreateProject_NilProject_MarksFailed(t *testing.T) {
	repo := store.NewMemoryRepository()
	ctx := context.Background()

	run, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: "user_1", Skill: "create_project"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	runSkill := func(ctx context.Context, s agent.Skill, raw json.RawMessage) (any, error) {
		return skills.CreateProjectResult{Status: "ok", Project: nil}, nil
	}
	p := newTestProcessor(repo, runSkill, agent.NewRegistry(fakeSkill{name: "create_project"}))

	if err := p.ProcessRun(ctx, "user_1", "", run.ID); err != nil {
		t.Fatalf("ProcessRun() error = %v", err)
	}

	got, err := repo.GetAgentRun(ctx, "user_1", "", run.ID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if got.Status != domain.AgentRunFailed {
		t.Fatalf("Status = %q, want %q", got.Status, domain.AgentRunFailed)
	}
}

// TestProcessor_ProcessRun_DecomposeTask_EmptySubtasks_MarksFailed is the
// same defensive check for decompose_task's equivalent malformed-response
// shape.
func TestProcessor_ProcessRun_DecomposeTask_EmptySubtasks_MarksFailed(t *testing.T) {
	repo := store.NewMemoryRepository()
	ctx := context.Background()

	run, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: "user_1", ProjectID: "proj_1", Skill: "decompose_task"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	runSkill := func(ctx context.Context, s agent.Skill, raw json.RawMessage) (any, error) {
		return skills.DecomposeResult{Status: "ok", Subtasks: nil}, nil
	}
	p := newTestProcessor(repo, runSkill, agent.NewRegistry(fakeSkill{name: "decompose_task"}))

	if err := p.ProcessRun(ctx, "user_1", "proj_1", run.ID); err != nil {
		t.Fatalf("ProcessRun() error = %v", err)
	}

	got, err := repo.GetAgentRun(ctx, "user_1", "proj_1", run.ID)
	if err != nil {
		t.Fatalf("GetAgentRun() error = %v", err)
	}
	if got.Status != domain.AgentRunFailed {
		t.Fatalf("Status = %q, want %q", got.Status, domain.AgentRunFailed)
	}
}

func TestProcessor_ProcessRun_NotFound(t *testing.T) {
	repo := store.NewMemoryRepository()
	p := newTestProcessor(repo, nil, agent.NewRegistry())

	err := p.ProcessRun(context.Background(), "user_1", "proj_1", "does-not-exist")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("ProcessRun() error = %v, want %v", err, store.ErrNotFound)
	}
}
