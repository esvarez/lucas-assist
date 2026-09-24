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

	"github.com/esvarez/lucas-assist/internal/auth"
	"github.com/esvarez/lucas-assist/internal/domain"
	"github.com/esvarez/lucas-assist/internal/store"
)

// withUserID simulates what auth.Middleware does in production: attaches
// an already-resolved user ID to the request's context before it reaches
// the router. Tests in this package exercise handlers directly against
// NewRouter (no middleware in front), so each one that needs an
// authenticated caller sets it up this way instead of a real Cognito
// token — the 401/unauthenticated path itself is internal/auth's own
// responsibility and is tested there (TestMiddleware_ResolverError), not
// re-tested per handler here.
func withUserID(req *http.Request, userID string) *http.Request {
	return req.WithContext(auth.WithUserID(req.Context(), userID))
}

func TestCreateProject_Success(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	body := `{"name": "Nudge", "goal": "Ship the POC", "constraints": ["no VPC"], "status": "active"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, "/projects", bytes.NewBufferString(body)), "user_1")
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
		t.Errorf("UserID = %q, want %q (the authenticated caller)", got.UserID, "user_1")
	}
}

// TestCreateProject_MissingName documents deliberate behavior: name is
// required, not silently left empty. architecture.md's domain model
// treats Name/Goal as a Project's defining content, and create_project
// never returns status: "ok" without having committed to both (#85).
// TestCreateProject_DefaultsDomainToGeneral documents issue #150's
// backward-compatibility rule: a create request that doesn't specify a
// domain gets General, not an empty string.
func TestCreateProject_DefaultsDomainToGeneral(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	body := `{"name": "Nudge", "goal": "Ship the POC"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, "/projects", bytes.NewBufferString(body)), "user_1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got domain.Project
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got.Domain != domain.ProjectDomainGeneral {
		t.Errorf("Domain = %q, want %q (the default)", got.Domain, domain.ProjectDomainGeneral)
	}
}

// TestCreateProject_SoftwareDomain documents that an explicit "software"
// domain is preserved rather than overridden by the default (issue #150).
func TestCreateProject_SoftwareDomain(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	body := `{"name": "Nudge", "goal": "Ship the POC", "domain": "software"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, "/projects", bytes.NewBufferString(body)), "user_1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got domain.Project
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got.Domain != domain.ProjectDomainSoftware {
		t.Errorf("Domain = %q, want %q", got.Domain, domain.ProjectDomainSoftware)
	}
}

// TestCreateProject_InvalidDomain documents that an unrecognized domain
// value is rejected up front rather than silently stored (issue #150).
func TestCreateProject_InvalidDomain(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	body := `{"name": "Nudge", "goal": "Ship the POC", "domain": "hardware"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, "/projects", bytes.NewBufferString(body)), "user_1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var got validationErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if msg, ok := got.Fields["domain"]; !ok {
		t.Errorf("Fields = %#v, want a \"domain\" entry naming which field failed", got.Fields)
	} else if msg == "" {
		t.Error(`Fields["domain"] is empty, want a message explaining why`)
	}
}

func TestCreateProject_MissingName(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	body := `{"goal": "Ship the POC"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, "/projects", bytes.NewBufferString(body)), "user_1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var got validationErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if msg, ok := got.Fields["name"]; !ok {
		t.Errorf("Fields = %#v, want a \"name\" entry naming which field failed", got.Fields)
	} else if msg == "" {
		t.Error(`Fields["name"] is empty, want a message explaining why`)
	}
}

// TestCreateProject_MissingGoal mirrors TestCreateProject_MissingName for
// the Goal field (#85).
func TestCreateProject_MissingGoal(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	body := `{"name": "Nudge"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, "/projects", bytes.NewBufferString(body)), "user_1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var got validationErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if msg, ok := got.Fields["goal"]; !ok {
		t.Errorf("Fields = %#v, want a \"goal\" entry naming which field failed", got.Fields)
	} else if msg == "" {
		t.Error(`Fields["goal"] is empty, want a message explaining why`)
	}
}

func TestCreateProject_InvalidBody(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	req := withUserID(httptest.NewRequest(http.MethodPost, "/projects", bytes.NewBufferString("not json")), "user_1")
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
	createErr    error
	deleteErr    error
	updateErr    error
	getErr       error
	listErr      error
	getRunErr    error
	getChangeErr error
	acceptErr    error
	listTasksErr error
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

func (s stubRepository) GetAgentRun(ctx context.Context, userID, projectID, runID string) (domain.AgentRun, error) {
	return domain.AgentRun{}, s.getRunErr
}

func (s stubRepository) ListAgentRuns(ctx context.Context, userID, projectID string) ([]domain.AgentRun, error) {
	return nil, s.getRunErr
}

func (s stubRepository) GetChangeset(ctx context.Context, userID, projectID, changesetID string) (domain.Changeset, error) {
	return domain.Changeset{}, s.getChangeErr
}

func (s stubRepository) ListChangesets(ctx context.Context, userID, projectID string) ([]domain.Changeset, error) {
	return nil, s.getChangeErr
}

func (s stubRepository) UpdateChangesetProposedTasks(ctx context.Context, userID, projectID, changesetID string, tasks []domain.ProposedTask) (domain.Changeset, error) {
	return domain.Changeset{}, s.acceptErr
}

func (s stubRepository) AcceptChangeset(ctx context.Context, p domain.Project, c domain.Changeset, remainingProposedTasks []domain.ProposedTask, requestFingerprint, idempotencyKey string) (store.AcceptChangesetResult, error) {
	return store.AcceptChangesetResult{}, s.acceptErr
}

func (s stubRepository) AcceptCreateProjectChangeset(ctx context.Context, c domain.Changeset, idempotencyKey string) (store.AcceptCreateProjectResult, error) {
	return store.AcceptCreateProjectResult{}, s.acceptErr
}

func (s stubRepository) ListTasks(ctx context.Context, userID, projectID string) ([]domain.Task, error) {
	return nil, s.listTasksErr
}

func TestCreateProject_DuplicateID(t *testing.T) {
	router := NewRouter(stubRepository{createErr: store.ErrDuplicateID})

	body := `{"name": "Nudge", "goal": "Ship the POC"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, "/projects", bytes.NewBufferString(body)), "user_1")
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

	req := withUserID(httptest.NewRequest(http.MethodDelete, "/projects/"+created.ID, nil), "user_1")
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

	req := withUserID(httptest.NewRequest(http.MethodDelete, "/projects/does-not-exist", nil), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestUpdateProject_Success(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	createBody := `{"name": "Nudge", "goal": "Ship the POC", "status": "active"}`
	createReq := withUserID(httptest.NewRequest(http.MethodPost, "/projects", bytes.NewBufferString(createBody)), "user_1")
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)

	var created domain.Project
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}

	updateBody := `{"version": 1, "goal": "Ship v2", "constraints": ["no VPC"], "status": "done"}`
	updateReq := withUserID(httptest.NewRequest(http.MethodPut, "/projects/"+created.ID, bytes.NewBufferString(updateBody)), "user_1")
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
	if got.Version != 2 {
		t.Errorf("Version = %d, want 2 (incremented from the created project's 1)", got.Version)
	}
}

// TestUpdateProject_VersionConflict documents the optimistic-concurrency
// path (architecture.md §8): retrying with a stale version 409s instead of
// silently applying.
func TestUpdateProject_VersionConflict(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	createBody := `{"name": "Nudge", "goal": "v1"}`
	createReq := withUserID(httptest.NewRequest(http.MethodPost, "/projects", bytes.NewBufferString(createBody)), "user_1")
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)

	var created domain.Project
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}

	firstBody := `{"version": 1, "goal": "v2"}`
	firstReq := withUserID(httptest.NewRequest(http.MethodPut, "/projects/"+created.ID, bytes.NewBufferString(firstBody)), "user_1")
	firstRec := httptest.NewRecorder()
	router.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("first update status = %d, want %d (body: %s)", firstRec.Code, http.StatusOK, firstRec.Body.String())
	}

	staleBody := `{"version": 1, "goal": "v3 (stale)"}`
	staleReq := withUserID(httptest.NewRequest(http.MethodPut, "/projects/"+created.ID, bytes.NewBufferString(staleBody)), "user_1")
	staleRec := httptest.NewRecorder()
	router.ServeHTTP(staleRec, staleReq)

	if staleRec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body: %s)", staleRec.Code, http.StatusConflict, staleRec.Body.String())
	}

	got, err := repo.GetProject(context.Background(), "user_1", created.ID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if got.Goal != "v2" {
		t.Errorf("Goal = %q, want %q (the conflicting write must not have applied)", got.Goal, "v2")
	}
}

func TestUpdateProject_NotFound(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	body := `{"version": 1, "goal": "Ship v2", "status": "done"}`
	req := withUserID(httptest.NewRequest(http.MethodPut, "/projects/does-not-exist", bytes.NewBufferString(body)), "user_1")
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

	createBody := `{"name": "Nudge", "goal": "Ship the POC"}`
	createReq := withUserID(httptest.NewRequest(http.MethodPost, "/projects", bytes.NewBufferString(createBody)), "user_1")
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)

	var created domain.Project
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}

	updateBody := `{"version": 1, "goal": "Ship v2", "status": "done"}`
	req := withUserID(httptest.NewRequest(http.MethodPut, "/projects/"+created.ID, bytes.NewBufferString(updateBody)), "user_2")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestUpdateProject_MissingVersion documents deliberate behavior: version
// is required, not silently defaulted to 0. Zero is never a real project's
// version (CreateProject always starts at 1), so an omitted version must
// be rejected up front rather than reaching the repository and always
// conflicting (architecture.md §8).
func TestUpdateProject_MissingVersion(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	body := `{"goal": "Ship v2", "status": "done"}`
	req := withUserID(httptest.NewRequest(http.MethodPut, "/projects/some-id", bytes.NewBufferString(body)), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var got validationErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if msg, ok := got.Fields["version"]; !ok {
		t.Errorf("Fields = %#v, want a \"version\" entry naming which field failed", got.Fields)
	} else if msg == "" {
		t.Error(`Fields["version"] is empty, want a message explaining why`)
	}
}

func TestUpdateProject_InvalidBody(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	req := withUserID(httptest.NewRequest(http.MethodPut, "/projects/some-id", bytes.NewBufferString("not json")), "user_1")
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

	req := withUserID(httptest.NewRequest(http.MethodGet, "/projects/"+created.ID, nil), "user_1")
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

	req := withUserID(httptest.NewRequest(http.MethodGet, "/projects/does-not-exist", nil), "user_1")
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

	req := withUserID(httptest.NewRequest(http.MethodGet, "/projects/"+created.ID, nil), "user_2")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
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

	req := withUserID(httptest.NewRequest(http.MethodGet, "/projects", nil), "user_1")
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
			t.Errorf("GET /projects as user_1 leaked project owned by %q", p.UserID)
		}
	}
}

func TestListProjects_Empty(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	req := withUserID(httptest.NewRequest(http.MethodGet, "/projects", nil), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	if body := rec.Body.String(); strings.TrimSpace(body) != "[]" {
		t.Errorf("body = %q, want an empty JSON array, not null", body)
	}
}
