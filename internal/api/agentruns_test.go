package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/esvarez/lucas-assist/internal/domain"
	"github.com/esvarez/lucas-assist/internal/store"
)

func agentRunPath(runID string) string {
	return "/agent-runs/" + runID
}

func TestGetAgentRun_Queued(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	run, err := repo.CreateAgentRun(context.Background(), domain.AgentRun{UserID: "user_1", ProjectID: "proj_1", Skill: "decompose_task"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	req := withUserID(httptest.NewRequest(http.MethodGet, fmt.Sprintf("%s?project_id=proj_1", agentRunPath(run.ID)), nil), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got agentRunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got.Status != domain.AgentRunQueued {
		t.Errorf("Status = %q, want %q", got.Status, domain.AgentRunQueued)
	}
	if got.Changeset != nil {
		t.Errorf("Changeset = %+v, want nil for a queued run", got.Changeset)
	}
}

func TestGetAgentRun_CompletedWithChangeset(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)
	ctx := context.Background()

	project, err := repo.CreateProject(ctx, domain.Project{UserID: "user_1", Name: "Nudge"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	changeset, err := repo.CreateChangeset(ctx, domain.Changeset{
		UserID:    "user_1",
		ProjectID: project.ID,
		Skill:     "decompose_task",
		Status:    domain.ChangesetProposed,
	})
	if err != nil {
		t.Fatalf("CreateChangeset() error = %v", err)
	}

	run, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: "user_1", ProjectID: project.ID, Skill: "decompose_task"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}
	if _, err := repo.LeaseAgentRun(ctx, "user_1", project.ID, run.ID, "worker_1", time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("LeaseAgentRun() error = %v", err)
	}
	if _, err := repo.CompleteAgentRun(ctx, "user_1", project.ID, run.ID, changeset.ID); err != nil {
		t.Fatalf("CompleteAgentRun() error = %v", err)
	}

	req := withUserID(httptest.NewRequest(http.MethodGet, fmt.Sprintf("%s?project_id=%s", agentRunPath(run.ID), project.ID), nil), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got agentRunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got.Status != domain.AgentRunCompleted {
		t.Errorf("Status = %q, want %q", got.Status, domain.AgentRunCompleted)
	}
	if got.Changeset == nil {
		t.Fatal("Changeset = nil, want the associated changeset for a completed run")
	}
	if got.Changeset.ID != changeset.ID {
		t.Errorf("Changeset.ID = %q, want %q", got.Changeset.ID, changeset.ID)
	}
}

func TestGetAgentRun_Failed(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)
	ctx := context.Background()

	run, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: "user_1", ProjectID: "proj_1"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}
	if _, err := repo.LeaseAgentRun(ctx, "user_1", "proj_1", run.ID, "worker_1", time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("LeaseAgentRun() error = %v", err)
	}
	if _, err := repo.FailAgentRun(ctx, "user_1", "proj_1", run.ID, "provider timeout"); err != nil {
		t.Fatalf("FailAgentRun() error = %v", err)
	}

	req := withUserID(httptest.NewRequest(http.MethodGet, fmt.Sprintf("%s?project_id=proj_1", agentRunPath(run.ID)), nil), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got agentRunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got.Status != domain.AgentRunFailed {
		t.Errorf("Status = %q, want %q", got.Status, domain.AgentRunFailed)
	}
	if got.Error != "provider timeout" {
		t.Errorf("Error = %q, want %q", got.Error, "provider timeout")
	}
}

func TestGetAgentRun_NeedsInput(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)
	ctx := context.Background()

	run, err := repo.CreateAgentRun(ctx, domain.AgentRun{UserID: "user_1", ProjectID: "proj_1", Skill: "decompose_task"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}
	if _, err := repo.LeaseAgentRun(ctx, "user_1", "proj_1", run.ID, "worker_1", time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("LeaseAgentRun() error = %v", err)
	}
	questions := []string{"What are you building?"}
	if _, err := repo.NeedsInputAgentRun(ctx, "user_1", "proj_1", run.ID, questions); err != nil {
		t.Fatalf("NeedsInputAgentRun() error = %v", err)
	}

	req := withUserID(httptest.NewRequest(http.MethodGet, fmt.Sprintf("%s?project_id=proj_1", agentRunPath(run.ID)), nil), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got agentRunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got.Status != domain.AgentRunNeedsInput {
		t.Errorf("Status = %q, want %q", got.Status, domain.AgentRunNeedsInput)
	}
	if len(got.Questions) != 1 || got.Questions[0] != questions[0] {
		t.Errorf("Questions = %#v, want %#v", got.Questions, questions)
	}
	if got.Changeset != nil {
		t.Errorf("Changeset = %+v, want nil for a needs_input run", got.Changeset)
	}
}

func TestGetAgentRun_NoProject(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	// create_project runs have no ProjectID at creation (#96); omitting
	// project_id from the query must still find it.
	run, err := repo.CreateAgentRun(context.Background(), domain.AgentRun{UserID: "user_1", Skill: "create_project"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	req := withUserID(httptest.NewRequest(http.MethodGet, agentRunPath(run.ID), nil), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestGetAgentRun_NotFound(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	req := withUserID(httptest.NewRequest(http.MethodGet, agentRunPath("does-not-exist"), nil), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestGetAgentRun_WrongUser documents that a run belonging to a different
// user 404s exactly like a genuinely missing one — same structural
// authorization as every other route (architecture.md §8).
func TestGetAgentRun_WrongUser(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	run, err := repo.CreateAgentRun(context.Background(), domain.AgentRun{UserID: "user_1", ProjectID: "proj_1"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}

	req := withUserID(httptest.NewRequest(http.MethodGet, fmt.Sprintf("%s?project_id=proj_1", agentRunPath(run.ID)), nil), "user_2")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestListAgentRuns documents #169's pending-run reload: a project's
// agent runs come back, optionally filtered to one status.
func TestListAgentRuns(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	project, err := repo.CreateProject(context.Background(), domain.Project{UserID: "user_1", Name: "Nudge"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	queued, err := repo.CreateAgentRun(context.Background(), domain.AgentRun{UserID: "user_1", ProjectID: project.ID, Skill: "decompose_task"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}
	completed, err := repo.CreateAgentRun(context.Background(), domain.AgentRun{UserID: "user_1", ProjectID: project.ID, Skill: "decompose_task"})
	if err != nil {
		t.Fatalf("CreateAgentRun() error = %v", err)
	}
	if _, err := repo.CompleteAgentRun(context.Background(), "user_1", project.ID, completed.ID, "cs_1"); err != nil {
		t.Fatalf("CompleteAgentRun() error = %v", err)
	}

	req := withUserID(httptest.NewRequest(http.MethodGet, "/projects/"+project.ID+"/agent-runs?status=queued", nil), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got listAgentRunsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(got.AgentRuns) != 1 || got.AgentRuns[0].ID != queued.ID {
		t.Fatalf("AgentRuns = %+v, want just the queued run %+v", got.AgentRuns, queued)
	}
}

func TestListAgentRuns_ProjectNotFound(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	req := withUserID(httptest.NewRequest(http.MethodGet, "/projects/does-not-exist/agent-runs", nil), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}
