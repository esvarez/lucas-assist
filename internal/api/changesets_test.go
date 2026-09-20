package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/esvarez/lucas-assist/internal/domain"
	"github.com/esvarez/lucas-assist/internal/store"
)

// setupChangesetToAccept creates a project and a proposed changeset with n
// ProposedTasks directly through repo, bypassing HTTP — the accept
// endpoint doesn't create changesets, only commits them.
func setupChangesetToAccept(t *testing.T, repo *store.MemoryRepository, userID string, n int) (domain.Project, domain.Changeset) {
	t.Helper()
	ctx := context.Background()

	project, err := repo.CreateProject(ctx, domain.Project{UserID: userID, Name: "Nudge", Goal: "Ship the POC"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	proposed := make([]domain.ProposedTask, n)
	for i := range proposed {
		proposed[i] = domain.ProposedTask{Title: fmt.Sprintf("Task %d", i)}
	}

	changeset, err := repo.CreateChangeset(ctx, domain.Changeset{
		UserID:        userID,
		ProjectID:     project.ID,
		Skill:         "decompose_task",
		BaseVersion:   project.Version,
		Status:        domain.ChangesetProposed,
		ProposedTasks: proposed,
	})
	if err != nil {
		t.Fatalf("CreateChangeset() error = %v", err)
	}

	return project, changeset
}

func acceptPath(projectID, changesetID string) string {
	return fmt.Sprintf("/projects/%s/changesets/%s/accept", projectID, changesetID)
}

func TestAcceptChangeset_Success(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)
	project, changeset := setupChangesetToAccept(t, repo, "user_1", 2)

	body := `{"user_id": "user_1", "idempotency_key": "key_1"}`
	req := httptest.NewRequest(http.MethodPost, acceptPath(project.ID, changeset.ID), bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got store.AcceptChangesetResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(got.Tasks) != 2 {
		t.Fatalf("Tasks = %+v, want 2 tasks", got.Tasks)
	}
	if got.Project.Version != project.Version+1 {
		t.Errorf("Project.Version = %d, want %d (incremented)", got.Project.Version, project.Version+1)
	}
}

func TestAcceptChangeset_IdempotentReplay(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)
	project, changeset := setupChangesetToAccept(t, repo, "user_1", 1)

	body := `{"user_id": "user_1", "idempotency_key": "key_1"}`

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodPost, acceptPath(project.ID, changeset.ID), bytes.NewBufferString(body)))
	if first.Code != http.StatusOK {
		t.Fatalf("first accept status = %d, want %d (body: %s)", first.Code, http.StatusOK, first.Body.String())
	}

	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodPost, acceptPath(project.ID, changeset.ID), bytes.NewBufferString(body)))
	if second.Code != http.StatusOK {
		t.Fatalf("replayed accept status = %d, want %d (body: %s)", second.Code, http.StatusOK, second.Body.String())
	}
	if first.Body.String() != second.Body.String() {
		t.Errorf("replayed accept body = %s, want identical to first %s", second.Body.String(), first.Body.String())
	}

	tasks, err := repo.ListTasks(context.Background(), "user_1", project.ID)
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("ListTasks() returned %d tasks, want 1 (replay must not create duplicates)", len(tasks))
	}
}

func TestAcceptChangeset_VersionConflict(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)
	project, changeset := setupChangesetToAccept(t, repo, "user_1", 1)

	// Bump the project's version out from under the changeset's BaseVersion.
	if _, err := repo.UpdateProject(context.Background(), "user_1", domain.Project{ID: project.ID, Version: project.Version}); err != nil {
		t.Fatalf("UpdateProject() error = %v", err)
	}

	body := `{"user_id": "user_1", "idempotency_key": "key_1"}`
	req := httptest.NewRequest(http.MethodPost, acceptPath(project.ID, changeset.ID), bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestAcceptChangeset_TooLarge(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)
	// 51 exceeds store's unexported maxChangesetMutations (50, ADR 009).
	project, changeset := setupChangesetToAccept(t, repo, "user_1", 51)

	body := `{"user_id": "user_1", "idempotency_key": "key_1"}`
	req := httptest.NewRequest(http.MethodPost, acceptPath(project.ID, changeset.ID), bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestAcceptChangeset_NotFound(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	body := `{"user_id": "user_1", "idempotency_key": "key_1"}`
	req := httptest.NewRequest(http.MethodPost, acceptPath("does-not-exist", "does-not-exist"), bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestAcceptChangeset_MissingUserID(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)
	project, changeset := setupChangesetToAccept(t, repo, "user_1", 1)

	body := `{"idempotency_key": "key_1"}`
	req := httptest.NewRequest(http.MethodPost, acceptPath(project.ID, changeset.ID), bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var got validationErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if _, ok := got.Fields["user_id"]; !ok {
		t.Errorf("Fields = %#v, want a \"user_id\" entry", got.Fields)
	}
}

func TestAcceptChangeset_MissingIdempotencyKey(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)
	project, changeset := setupChangesetToAccept(t, repo, "user_1", 1)

	body := `{"user_id": "user_1"}`
	req := httptest.NewRequest(http.MethodPost, acceptPath(project.ID, changeset.ID), bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var got validationErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if _, ok := got.Fields["idempotency_key"]; !ok {
		t.Errorf("Fields = %#v, want an \"idempotency_key\" entry", got.Fields)
	}
}

func TestAcceptChangeset_InvalidBody(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	req := httptest.NewRequest(http.MethodPost, acceptPath("proj_1", "cs_1"), bytes.NewBufferString("not json"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
