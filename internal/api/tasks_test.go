package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

func postTask(t *testing.T, router http.Handler, userID, projectID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := withUserID(httptest.NewRequest(http.MethodPost, "/projects/"+projectID+"/tasks", strings.NewReader(body)), userID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestCreateTask_Success(t *testing.T) {
	repo := store.NewMemoryRepository()
	project := createTestProject(t, repo, "user_1", "Nudge")
	router := NewRouter(repo)

	rec := postTask(t, router, "user_1", project.ID, `{"title":"  Write the README  ","description":"Setup steps","acceptance_criteria":["Covers install"," ","Covers run"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got domain.Task
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got.ID == "" {
		t.Error("ID is empty, want a generated ID")
	}
	if got.ProjectID != project.ID || got.Title != "Write the README" || got.Description != "Setup steps" {
		t.Errorf("got = %+v, want project %q, trimmed title, description", got, project.ID)
	}
	if got.Status != "todo" || got.Order != 0 || got.ParentID != "" {
		t.Errorf("status/order/parent = %q/%d/%q, want todo/0/\"\"", got.Status, got.Order, got.ParentID)
	}
	if len(got.AcceptanceCriteria) != 2 || got.AcceptanceCriteria[0] != "Covers install" || got.AcceptanceCriteria[1] != "Covers run" {
		t.Errorf("acceptance_criteria = %q, want blanks dropped", got.AcceptanceCriteria)
	}

	stored, err := repo.ListTasks(context.Background(), "user_1", project.ID)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(stored) != 1 || stored[0].ID != got.ID {
		t.Errorf("stored tasks = %+v, want just the created one", stored)
	}
}

func TestCreateTask_AppendsAfterSiblings(t *testing.T) {
	repo := store.NewMemoryRepository()
	project := createTestProject(t, repo, "user_1", "Nudge")
	parent, err := repo.CreateTask(context.Background(), "user_1", domain.Task{ProjectID: project.ID, Title: "Parent", Status: "todo", Order: 3})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	router := NewRouter(repo)

	rec := postTask(t, router, "user_1", project.ID, `{"title":"Root sibling"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	var root domain.Task
	_ = json.Unmarshal(rec.Body.Bytes(), &root)
	if root.Order != 4 {
		t.Errorf("root order = %d, want 4 (after the existing root at 3)", root.Order)
	}

	rec = postTask(t, router, "user_1", project.ID, `{"title":"First subtask","parent_id":"`+parent.ID+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	var sub domain.Task
	_ = json.Unmarshal(rec.Body.Bytes(), &sub)
	if sub.ParentID != parent.ID || sub.Order != 0 {
		t.Errorf("subtask parent/order = %q/%d, want %q/0", sub.ParentID, sub.Order, parent.ID)
	}
}

func TestCreateTask_MissingTitle(t *testing.T) {
	repo := store.NewMemoryRepository()
	project := createTestProject(t, repo, "user_1", "Nudge")

	rec := postTask(t, NewRouter(repo), "user_1", project.ID, `{"title":"   "}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	var got validationErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if _, ok := got.Fields["title"]; !ok {
		t.Errorf("fields = %v, want a title error", got.Fields)
	}
}

func TestCreateTask_InvalidBody(t *testing.T) {
	repo := store.NewMemoryRepository()
	project := createTestProject(t, repo, "user_1", "Nudge")

	rec := postTask(t, NewRouter(repo), "user_1", project.ID, `{`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateTask_ProjectNotFound(t *testing.T) {
	repo := store.NewMemoryRepository()
	project := createTestProject(t, repo, "user_1", "Nudge")

	// Another user's project is indistinguishable from a nonexistent one.
	rec := postTask(t, NewRouter(repo), "user_2", project.ID, `{"title":"Sneaky"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	tasks, _ := repo.ListTasks(context.Background(), "user_1", project.ID)
	if len(tasks) != 0 {
		t.Errorf("tasks = %+v, want none created", tasks)
	}
}

func TestCreateTask_ParentInOtherProject(t *testing.T) {
	repo := store.NewMemoryRepository()
	project := createTestProject(t, repo, "user_1", "Nudge")
	other := createTestProject(t, repo, "user_1", "Other")
	foreign, err := repo.CreateTask(context.Background(), "user_1", domain.Task{ProjectID: other.ID, Title: "Elsewhere", Status: "todo"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	rec := postTask(t, NewRouter(repo), "user_1", project.ID, `{"title":"Child","parent_id":"`+foreign.ID+`"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	var got validationErrorResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if _, ok := got.Fields["parent_id"]; !ok {
		t.Errorf("fields = %v, want a parent_id error", got.Fields)
	}
}

func TestCreateTask_StoreError(t *testing.T) {
	router := NewRouter(stubRepository{createTaskErr: errors.New("boom")})

	rec := postTask(t, router, "user_1", "p1", `{"title":"Anything"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
