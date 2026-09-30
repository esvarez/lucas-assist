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

// TestCreateTask documents #175: a user can add a single task by hand,
// without going through decompose_task's proposal/review/accept flow.
func TestCreateTask(t *testing.T) {
	repo := store.NewMemoryRepository()
	project := createTestProject(t, repo, "user_1", "Nudge")
	router := NewRouter(repo)

	body := `{"title": "Add login command", "description": "Device-flow login for the CLI", "acceptance_criteria": ["running nudge login prints a device code"]}`
	req := withUserID(httptest.NewRequest(http.MethodPost, "/projects/"+project.ID+"/tasks", bytes.NewBufferString(body)), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got createTaskResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got.Task.ID == "" {
		t.Error("Task.ID = \"\", want a generated ID")
	}
	if got.Task.Title != "Add login command" {
		t.Errorf("Task.Title = %q, want %q", got.Task.Title, "Add login command")
	}
	if got.Task.Status != "todo" {
		t.Errorf("Task.Status = %q, want %q", got.Task.Status, "todo")
	}

	stored, err := repo.GetTask(context.Background(), "user_1", project.ID, got.Task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.Title != "Add login command" {
		t.Errorf("stored Task.Title = %q, want %q — creation didn't persist", stored.Title, "Add login command")
	}
}

// TestCreateTask_Subtask documents that a task created with a parent_id
// attaches under that task, ordered after any existing siblings.
func TestCreateTask_Subtask(t *testing.T) {
	repo := store.NewMemoryRepository()
	project := createTestProject(t, repo, "user_1", "Nudge")
	parent, err := repo.CreateTask(context.Background(), "user_1", domain.Task{ProjectID: project.ID, Title: "Ship the POC", Status: "todo"})
	if err != nil {
		t.Fatalf("CreateTask (parent): %v", err)
	}
	if _, err := repo.CreateTask(context.Background(), "user_1", domain.Task{ProjectID: project.ID, ParentID: parent.ID, Title: "First subtask", Status: "todo", Order: 0}); err != nil {
		t.Fatalf("CreateTask (existing sibling): %v", err)
	}
	router := NewRouter(repo)

	body := `{"title": "Second subtask", "parent_id": "` + parent.ID + `"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, "/projects/"+project.ID+"/tasks", bytes.NewBufferString(body)), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got createTaskResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got.Task.ParentID != parent.ID {
		t.Errorf("Task.ParentID = %q, want %q", got.Task.ParentID, parent.ID)
	}
	if got.Task.Order != 1 {
		t.Errorf("Task.Order = %d, want 1 (after the existing sibling)", got.Task.Order)
	}
}

func TestCreateTask_MissingTitle(t *testing.T) {
	repo := store.NewMemoryRepository()
	project := createTestProject(t, repo, "user_1", "Nudge")
	router := NewRouter(repo)

	body := `{"description": "no title"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, "/projects/"+project.ID+"/tasks", bytes.NewBufferString(body)), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestCreateTask_UnknownParent(t *testing.T) {
	repo := store.NewMemoryRepository()
	project := createTestProject(t, repo, "user_1", "Nudge")
	router := NewRouter(repo)

	body := `{"title": "Orphan", "parent_id": "does-not-exist"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, "/projects/"+project.ID+"/tasks", bytes.NewBufferString(body)), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestCreateTask_UnknownProject(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	body := `{"title": "Add login command"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, "/projects/does-not-exist/tasks", bytes.NewBufferString(body)), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestCreateTask_WrongOwner documents that a project ID belonging to a
// different user 404s, same as TestListTasks_WrongOwner — it must not let
// one user create a task inside another user's project.
func TestCreateTask_WrongOwner(t *testing.T) {
	repo := store.NewMemoryRepository()
	project := createTestProject(t, repo, "user_1", "Nudge")
	router := NewRouter(repo)

	body := `{"title": "Add login command"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, "/projects/"+project.ID+"/tasks", bytes.NewBufferString(body)), "user_2")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}

	tasks, err := repo.ListTasks(context.Background(), "user_1", project.ID)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Error("a different user's request created a task in this project")
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
