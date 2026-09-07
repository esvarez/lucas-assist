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

	body := `{"name": "Nudge", "goal": "Ship the POC", "constraints": ["no VPC"], "status": "active"}`
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

func TestCreateProject_DuplicateID(t *testing.T) {
	router := NewRouter(stubRepository{err: store.ErrDuplicateID})

	body := `{"name": "Nudge"}`
	req := httptest.NewRequest(http.MethodPost, "/projects", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusConflict, rec.Body.String())
	}
}
