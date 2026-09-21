package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubResolver lets a test force whatever ResolveUserID returns.
type stubResolver struct {
	userID string
	err    error
}

func (s stubResolver) ResolveUserID(r *http.Request) (string, error) {
	return s.userID, s.err
}

func TestMiddleware_Success(t *testing.T) {
	var gotUserID string
	var gotOK bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID, gotOK = UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := Middleware(stubResolver{userID: "user-123"})(next)

	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !gotOK || gotUserID != "user-123" {
		t.Errorf("UserIDFromContext() = (%q, %v), want (\"user-123\", true)", gotUserID, gotOK)
	}
}

func TestMiddleware_ResolverError(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	handler := Middleware(stubResolver{err: errors.New("boom")})(next)

	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if called {
		t.Error("next handler was called despite a resolver error — 401 must short-circuit")
	}
}
