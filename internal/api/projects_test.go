package api

import (
	"bytes"
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

func TestCreateProject_Success(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	body := `{"user_id": "user_1", "name": "Nudge", "goal": "Ship the POC", "constraints": ["no VPC"], "status": "active"}`
	req := httptest.NewRequest(http.MethodPost, "/projects", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got domain.Project
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got.ID == "" {
		t.Error("ID = \"\", want a generated ID")
	}
	if got.Name != "Nudge" || got.Goal != "Ship the POC" {
		t.Errorf("response = %+v, want Name/Goal preserved from request", got)
	}
	if got.UserID != "user_1" {
		t.Errorf("UserID = %q, want %q (the supplied user_id)", got.UserID, "user_1")
	}
}

// TestCreateProject_MissingUserID documents deliberate behavior: user_id
// is required, not silently defaulted to empty. The DynamoDB key schema
// partitions by user (PK=USER#<uid>) — an empty UserID would collide
// every such project into one shared "USER#" partition, defeating that
// isolation (#47).
func TestCreateProject_MissingUserID(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	body := `{"name": "Nudge", "goal": "Ship the POC"}`
	req := httptest.NewRequest(http.MethodPost, "/projects", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var got validationErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if msg, ok := got.Fields["user_id"]; !ok {
		t.Errorf("Fields = %#v, want a \"user_id\" entry naming which field failed", got.Fields)
	} else if msg == "" {
		t.Error(`Fields["user_id"] is empty, want a message explaining why`)
	}
}

func TestCreateProject_InvalidBody(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	req := httptest.NewRequest(http.MethodPost, "/projects", bytes.NewBufferString("not json"))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// stubRepository lets a test force whatever error CreateProject/
// DeleteProject/UpdateProject/GetProject/ListProjects returns. Project IDs
// are always server-generated (see domain.NewID, #18), so a client can't
// trigger store.ErrDuplicateID through the HTTP API itself — this is the
// only way to exercise the handlers' error-status mapping for errors the
// memory repo won't naturally produce via the API.
type stubRepository struct {
	createErr error
	deleteErr error
	updateErr error
	getErr    error
	listErr   error
}

func (s stubRepository) CreateProject(ctx context.Context, p domain.Project) (domain.Project, error) {
	return domain.Project{}, s.createErr
}

func (s stubRepository) DeleteProject(ctx context.Context, userID, id string) error {
	return s.deleteErr
}

func (s stubRepository) UpdateProject(ctx context.Context, userID string, p domain.Project) (domain.Project, error) {
	return domain.Project{}, s.updateErr
}

func (s stubRepository) GetProject(ctx context.Context, userID, id string) (domain.Project, error) {
	return domain.Project{}, s.getErr
}

func (s stubRepository) ListProjects(ctx context.Context, userID string) ([]domain.Project, error) {
	return nil, s.listErr
}

func TestCreateProject_DuplicateID(t *testing.T) {
	router := NewRouter(stubRepository{createErr: store.ErrDuplicateID})

	body := `{"user_id": "user_1", "name": "Nudge"}`
	req := httptest.NewRequest(http.MethodPost, "/projects", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestDeleteProject_Success(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	created, err := repo.CreateProject(context.Background(), domain.Project{UserID: "user_1", Name: "Nudge"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/projects/"+created.ID+"?user_id=user_1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	if _, err := repo.GetProject(context.Background(), "user_1", created.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("GetProject() after delete error = %v, want %v", err, store.ErrNotFound)
	}
}

func TestDeleteProject_NotFound(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	req := httptest.NewRequest(http.MethodDelete, "/projects/does-not-exist?user_id=user_1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestDeleteProject_MissingUserID(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	req := httptest.NewRequest(http.MethodDelete, "/projects/some-id", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestUpdateProject_Success(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	createBody := `{"user_id": "user_1", "name": "Nudge", "goal": "Ship the POC", "status": "active"}`
	createReq := httptest.NewRequest(http.MethodPost, "/projects", bytes.NewBufferString(createBody))
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)

	var created domain.Project
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}

	updateBody := `{"user_id": "user_1", "goal": "Ship v2", "constraints": ["no VPC"], "status": "done"}`
	updateReq := httptest.NewRequest(http.MethodPut, "/projects/"+created.ID, bytes.NewBufferString(updateBody))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, updateReq)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got domain.Project
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %q, want unchanged %q", got.ID, created.ID)
	}
	if got.Goal != "Ship v2" || got.Status != "done" {
		t.Errorf("response = %+v, want Goal/Status updated", got)
	}
	if got.Name != "Nudge" {
		t.Errorf("Name = %q, want unchanged %q (UpdateProject never touches Name)", got.Name, "Nudge")
	}
}

func TestUpdateProject_NotFound(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	body := `{"user_id": "user_1", "goal": "Ship v2", "status": "done"}`
	req := httptest.NewRequest(http.MethodPut, "/projects/does-not-exist", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestUpdateProject_WrongUser documents that "not found" also covers a
// project ID that exists, but under a different user's partition — the
// same structural-authorization property GetProject/ListProjects/
// DeleteProject already have (architecture.md §7).
func TestUpdateProject_WrongUser(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	createBody := `{"user_id": "user_1", "name": "Nudge"}`
	createReq := httptest.NewRequest(http.MethodPost, "/projects", bytes.NewBufferString(createBody))
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)

	var created domain.Project
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}

	updateBody := `{"user_id": "user_2", "goal": "Ship v2", "status": "done"}`
	req := httptest.NewRequest(http.MethodPut, "/projects/"+created.ID, bytes.NewBufferString(updateBody))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestUpdateProject_MissingUserID(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	body := `{"goal": "Ship v2", "status": "done"}`
	req := httptest.NewRequest(http.MethodPut, "/projects/some-id", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestUpdateProject_InvalidBody(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	req := httptest.NewRequest(http.MethodPut, "/projects/some-id", bytes.NewBufferString("not json"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetProject_Success(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	created, err := repo.CreateProject(context.Background(), domain.Project{UserID: "user_1", Name: "Nudge", Goal: "Ship the POC"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/projects/"+created.ID+"?user_id=user_1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got domain.Project
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got.ID != created.ID || got.Name != "Nudge" || got.Goal != "Ship the POC" {
		t.Errorf("response = %+v, want the created project", got)
	}
}

func TestGetProject_NotFound(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	req := httptest.NewRequest(http.MethodGet, "/projects/does-not-exist?user_id=user_1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestGetProject_WrongUser documents that a project ID belonging to a
// different user 404s exactly like a genuinely missing one — the item
// lives under a different USER#<uid> partition entirely, so there's no
// way to distinguish "not yours" from "doesn't exist" (architecture.md
// §7's structural authorization).
func TestGetProject_WrongUser(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	created, err := repo.CreateProject(context.Background(), domain.Project{UserID: "user_1", Name: "Nudge"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/projects/"+created.ID+"?user_id=user_2", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestGetProject_MissingUserID(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	req := httptest.NewRequest(http.MethodGet, "/projects/some-id", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestListProjects_WithItems(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	ctx := context.Background()
	if _, err := repo.CreateProject(ctx, domain.Project{UserID: "user_1", Name: "Nudge"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if _, err := repo.CreateProject(ctx, domain.Project{UserID: "user_1", Name: "Widget"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	// A different user's project must never show up in user_1's list.
	if _, err := repo.CreateProject(ctx, domain.Project{UserID: "user_2", Name: "Someone else's"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/projects?user_id=user_1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []domain.Project
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListProjects() returned %d projects, want 2", len(got))
	}
	for _, p := range got {
		if p.UserID != "user_1" {
			t.Errorf("GET /projects?user_id=user_1 leaked project owned by %q", p.UserID)
		}
	}
}

func TestListProjects_Empty(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	req := httptest.NewRequest(http.MethodGet, "/projects?user_id=user_1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	if body := rec.Body.String(); strings.TrimSpace(body) != "[]" {
		t.Errorf("body = %q, want an empty JSON array, not null", body)
	}
}

func TestListProjects_MissingUserID(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
