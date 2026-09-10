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

// stubRepository lets a test force whatever error CreateProject returns.
// Project IDs are always server-generated (see domain.NewID, #18), so a
// client can't trigger store.ErrDuplicateID through the HTTP API itself —
// this is the only way to exercise the handler's 409 mapping for it.
type stubRepository struct {
	err error
}

func (s stubRepository) CreateProject(ctx context.Context, p domain.Project) (domain.Project, error) {
	return domain.Project{}, s.err
}

func (s stubRepository) UpdateProject(ctx context.Context, userID string, p domain.Project) (domain.Project, error) {
	return domain.Project{}, s.err
}

func TestCreateProject_DuplicateID(t *testing.T) {
	router := NewRouter(stubRepository{err: store.ErrDuplicateID})

	body := `{"user_id": "user_1", "name": "Nudge"}`
	req := httptest.NewRequest(http.MethodPost, "/projects", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusConflict, rec.Body.String())
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
