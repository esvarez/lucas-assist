package api

import (
	"bytes"
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

func TestCreateProject_DuplicateID(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	body := `{"id": "proj_1", "name": "Nudge"}`
	first := httptest.NewRequest(http.MethodPost, "/projects", bytes.NewBufferString(body))
	router.ServeHTTP(httptest.NewRecorder(), first)

	second := httptest.NewRequest(http.MethodPost, "/projects", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, second)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusConflict, rec.Body.String())
	}
}
