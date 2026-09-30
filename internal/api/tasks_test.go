package api

import (
	"bytes"
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

	req := withUserID(httptest.NewRequest(http.MethodGet, "/projects/"+project.ID+"/tasks", nil), "user_1")
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
	req := withUserID(httptest.NewRequest(http.MethodGet, "/projects/"+project.ID+"/tasks", nil), "user_1")
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

func TestListTasks_UnknownProject(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	req := withUserID(httptest.NewRequest(http.MethodGet, "/projects/does-not-exist/tasks", nil), "user_1")
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

	req := withUserID(httptest.NewRequest(http.MethodGet, "/projects/"+project.ID+"/tasks", nil), "user_2")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestUpdateTaskStatus documents #181's durable half: checking a task
// done persists it, instead of only updating the browser's own React
// state until the next page load quietly forgets it.
func TestUpdateTaskStatus(t *testing.T) {
	repo := store.NewMemoryRepository()
	project := createTestProject(t, repo, "user_1", "Nudge")
	task, err := repo.CreateTask(context.Background(), "user_1", domain.Task{ProjectID: project.ID, Title: "Add login command", Status: "todo"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	router := NewRouter(repo)

	body := `{"status": "done"}`
	req := withUserID(httptest.NewRequest(http.MethodPatch, "/projects/"+project.ID+"/tasks/"+task.ID, bytes.NewBufferString(body)), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got updateTaskStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got.Task.Status != "done" {
		t.Errorf("Task.Status = %q, want %q", got.Task.Status, "done")
	}

	stored, err := repo.GetTask(context.Background(), "user_1", project.ID, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.Status != "done" {
		t.Errorf("stored Task.Status = %q, want %q — update didn't persist", stored.Status, "done")
	}
}

func TestUpdateTaskStatus_InvalidStatus(t *testing.T) {
	repo := store.NewMemoryRepository()
	project := createTestProject(t, repo, "user_1", "Nudge")
	task, err := repo.CreateTask(context.Background(), "user_1", domain.Task{ProjectID: project.ID, Title: "Add login command"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	router := NewRouter(repo)

	body := `{"status": "not-a-real-status"}`
	req := withUserID(httptest.NewRequest(http.MethodPatch, "/projects/"+project.ID+"/tasks/"+task.ID, bytes.NewBufferString(body)), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestUpdateTaskStatus_UnknownTask(t *testing.T) {
	repo := store.NewMemoryRepository()
	project := createTestProject(t, repo, "user_1", "Nudge")
	router := NewRouter(repo)

	body := `{"status": "done"}`
	req := withUserID(httptest.NewRequest(http.MethodPatch, "/projects/"+project.ID+"/tasks/does-not-exist", bytes.NewBufferString(body)), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestUpdateTaskStatus_WrongOwner documents that a task ID belonging to a
// different user 404s, same as TestListTasks_WrongOwner — it must not let
// one user mutate another's task.
func TestUpdateTaskStatus_WrongOwner(t *testing.T) {
	repo := store.NewMemoryRepository()
	project := createTestProject(t, repo, "user_1", "Nudge")
	task, err := repo.CreateTask(context.Background(), "user_1", domain.Task{ProjectID: project.ID, Title: "Add login command", Status: "todo"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	router := NewRouter(repo)

	body := `{"status": "done"}`
	req := withUserID(httptest.NewRequest(http.MethodPatch, "/projects/"+project.ID+"/tasks/"+task.ID, bytes.NewBufferString(body)), "user_2")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}

	stored, err := repo.GetTask(context.Background(), "user_1", project.ID, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.Status == "done" {
		t.Error("a different user's request changed the task's status")
	}
}
