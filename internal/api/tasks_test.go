package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/esvarez/lucas-assist/internal/domain"
	"github.com/esvarez/lucas-assist/internal/store"
)

func createTestProject(t *testing.T, repo *store.MemoryRepository, userID, name string) domain.Project {
	t.Helper()
	p, err := repo.CreateProject(context.Background(), domain.Project{UserID: userID, Name: name, Goal: "goal"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return p
}

func TestListTasks_Empty(t *testing.T) {
	repo := store.NewMemoryRepository()
	project := createTestProject(t, repo, "user_1", "Nudge")
	router := NewRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/projects/"+project.ID+"/tasks?user_id=user_1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []domain.Task
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got == nil {
		t.Error("response = null, want an empty array")
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}

func TestListTasks_FlatAndNested(t *testing.T) {
	repo := store.NewMemoryRepository()
	project := createTestProject(t, repo, "user_1", "Nudge")

	parent, err := repo.CreateTask(context.Background(), "user_1", domain.Task{
		ProjectID: project.ID,
		Title:     "Ship the POC",
		Status:    "todo",
		Order:     0,
	})
	if err != nil {
		t.Fatalf("CreateTask (parent): %v", err)
	}
	child, err := repo.CreateTask(context.Background(), "user_1", domain.Task{
		ProjectID: project.ID,
		ParentID:  parent.ID,
		Title:     "Wire the schema",
		Status:    "in-progress",
		Order:     0,
	})
	if err != nil {
		t.Fatalf("CreateTask (child): %v", err)
	}

	router := NewRouter(repo)
	req := httptest.NewRequest(http.MethodGet, "/projects/"+project.ID+"/tasks?user_id=user_1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []domain.Task
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (flat, not nested)", len(got))
	}

	byID := map[string]domain.Task{}
	for _, task := range got {
		byID[task.ID] = task
	}
	if byID[child.ID].ParentID != parent.ID {
		t.Errorf("child.ParentID = %q, want %q", byID[child.ID].ParentID, parent.ID)
	}
	if byID[parent.ID].ParentID != "" {
		t.Errorf("parent.ParentID = %q, want empty", byID[parent.ID].ParentID)
	}
}

func TestListTasks_MissingUserID(t *testing.T) {
	repo := store.NewMemoryRepository()
	project := createTestProject(t, repo, "user_1", "Nudge")
	router := NewRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/projects/"+project.ID+"/tasks", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestListTasks_UnknownProject(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	req := httptest.NewRequest(http.MethodGet, "/projects/does-not-exist/tasks?user_id=user_1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestListTasks_WrongOwner documents that a project ID belonging to a
// different user 404s, same as getProjectHandler — it must not leak that
// user's task list.
func TestListTasks_WrongOwner(t *testing.T) {
	repo := store.NewMemoryRepository()
	project := createTestProject(t, repo, "user_1", "Nudge")
	router := NewRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/projects/"+project.ID+"/tasks?user_id=user_2", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}
