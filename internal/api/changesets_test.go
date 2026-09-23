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

// createProjectAndChangeset is shared setup for the accept-changeset tests
// below: a project plus a proposed changeset against it, ready to accept.
func createProjectAndChangeset(t *testing.T, repo *store.MemoryRepository, userID string, proposedTasks []domain.ProposedTask) (domain.Project, domain.Changeset) {
	t.Helper()
	ctx := context.Background()

	project, err := repo.CreateProject(ctx, domain.Project{UserID: userID, Name: "Nudge", Goal: "Ship the POC"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	changeset, err := repo.CreateChangeset(ctx, domain.Changeset{
		ProjectID:     project.ID,
		UserID:        userID,
		Skill:         "decompose_task",
		BaseVersion:   project.Version,
		Status:        domain.ChangesetProposed,
		ProposedTasks: proposedTasks,
	})
	if err != nil {
		t.Fatalf("CreateChangeset() error = %v", err)
	}

	return project, changeset
}

func acceptChangesetPath(project domain.Project, changeset domain.Changeset) string {
	return fmt.Sprintf("/projects/%s/changesets/%s/accept", project.ID, changeset.ID)
}

func TestAcceptChangeset_Success(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	project, changeset := createProjectAndChangeset(t, repo, "user_1", []domain.ProposedTask{
		{Title: "First", Description: "Do the first thing"},
		{Title: "Second", Description: "Do the second thing"},
	})

	body := `{"idempotency_key": "idem-key-1"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, acceptChangesetPath(project, changeset), bytes.NewBufferString(body)), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got acceptChangesetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(got.Tasks) != 2 {
		t.Fatalf("len(Tasks) = %d, want 2", len(got.Tasks))
	}
	if got.Project.Version != project.Version+1 {
		t.Errorf("Project.Version = %d, want %d", got.Project.Version, project.Version+1)
	}
	if got.Event.Type != domain.EventChangesetAccepted {
		t.Errorf("Event.Type = %q, want %q", got.Event.Type, domain.EventChangesetAccepted)
	}

	gotChangeset, err := repo.GetChangeset(context.Background(), "user_1", project.ID, changeset.ID)
	if err != nil {
		t.Fatalf("GetChangeset() error = %v", err)
	}
	if gotChangeset.Status != domain.ChangesetApplied {
		t.Errorf("Changeset.Status = %q, want %q", gotChangeset.Status, domain.ChangesetApplied)
	}
}

func TestAcceptChangeset_MissingIdempotencyKey(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	project, changeset := createProjectAndChangeset(t, repo, "user_1", []domain.ProposedTask{{Title: "First"}})

	req := withUserID(httptest.NewRequest(http.MethodPost, acceptChangesetPath(project, changeset), bytes.NewBufferString("{}")), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestAcceptChangeset_InvalidBody(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	project, changeset := createProjectAndChangeset(t, repo, "user_1", []domain.ProposedTask{{Title: "First"}})

	req := withUserID(httptest.NewRequest(http.MethodPost, acceptChangesetPath(project, changeset), bytes.NewBufferString("not json")), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAcceptChangeset_ProjectNotFound(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	body := `{"idempotency_key": "idem-key-1"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, "/projects/does-not-exist/changesets/cs_1/accept", bytes.NewBufferString(body)), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestAcceptChangeset_ChangesetNotFound(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	project, err := repo.CreateProject(context.Background(), domain.Project{UserID: "user_1", Name: "Nudge"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	body := `{"idempotency_key": "idem-key-1"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, "/projects/"+project.ID+"/changesets/does-not-exist/accept", bytes.NewBufferString(body)), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestAcceptChangeset_WrongUser documents the same structural-authorization
// convention as every other route: a project belonging to a different user
// 404s rather than 403ing.
func TestAcceptChangeset_WrongUser(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	project, changeset := createProjectAndChangeset(t, repo, "user_1", []domain.ProposedTask{{Title: "First"}})

	body := `{"idempotency_key": "idem-key-1"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, acceptChangesetPath(project, changeset), bytes.NewBufferString(body)), "user_2")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestAcceptChangeset_NotProposed documents that only a "proposed"
// changeset can be accepted — accepting an already-applied one 409s rather
// than re-applying it.
func TestAcceptChangeset_NotProposed(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	project, changeset := createProjectAndChangeset(t, repo, "user_1", []domain.ProposedTask{{Title: "First"}})

	body := `{"idempotency_key": "idem-key-1"}`
	first := withUserID(httptest.NewRequest(http.MethodPost, acceptChangesetPath(project, changeset), bytes.NewBufferString(body)), "user_1")
	firstRec := httptest.NewRecorder()
	router.ServeHTTP(firstRec, first)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("first accept status = %d, want %d (body: %s)", firstRec.Code, http.StatusOK, firstRec.Body.String())
	}

	// A second, distinct accept attempt (different idempotency key) against
	// the now-applied changeset must not re-apply it.
	secondBody := `{"idempotency_key": "idem-key-2"}`
	second := withUserID(httptest.NewRequest(http.MethodPost, acceptChangesetPath(project, changeset), bytes.NewBufferString(secondBody)), "user_1")
	secondRec := httptest.NewRecorder()
	router.ServeHTTP(secondRec, second)

	if secondRec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body: %s)", secondRec.Code, http.StatusConflict, secondRec.Body.String())
	}

	tasks, err := repo.ListTasks(context.Background(), "user_1", project.ID)
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("ListTasks() returned %d tasks, want 1 (must not double-apply)", len(tasks))
	}
}

// TestAcceptChangeset_VersionConflict documents architecture.md §8: a
// stale baseVersion 409s and moves the changeset to conflict, rather than
// applying against the wrong project state.
func TestAcceptChangeset_VersionConflict(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	project, changeset := createProjectAndChangeset(t, repo, "user_1", []domain.ProposedTask{{Title: "First"}})

	if _, err := repo.UpdateProject(context.Background(), "user_1", domain.Project{ID: project.ID, Goal: "changed", Version: project.Version}); err != nil {
		t.Fatalf("UpdateProject() error = %v", err)
	}

	body := `{"idempotency_key": "idem-key-1"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, acceptChangesetPath(project, changeset), bytes.NewBufferString(body)), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusConflict, rec.Body.String())
	}

	gotChangeset, err := repo.GetChangeset(context.Background(), "user_1", project.ID, changeset.ID)
	if err != nil {
		t.Fatalf("GetChangeset() error = %v", err)
	}
	if gotChangeset.Status != domain.ChangesetConflict {
		t.Errorf("Changeset.Status = %q, want %q", gotChangeset.Status, domain.ChangesetConflict)
	}
}

// TestAcceptChangeset_IdempotentReplay documents architecture.md §15:
// replaying an accept with the same idempotency key returns the original
// result instead of re-applying it.
func TestAcceptChangeset_IdempotentReplay(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	project, changeset := createProjectAndChangeset(t, repo, "user_1", []domain.ProposedTask{{Title: "First"}})

	body := `{"idempotency_key": "idem-key-1"}`

	first := withUserID(httptest.NewRequest(http.MethodPost, acceptChangesetPath(project, changeset), bytes.NewBufferString(body)), "user_1")
	firstRec := httptest.NewRecorder()
	router.ServeHTTP(firstRec, first)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("first accept status = %d, want %d (body: %s)", firstRec.Code, http.StatusOK, firstRec.Body.String())
	}

	second := withUserID(httptest.NewRequest(http.MethodPost, acceptChangesetPath(project, changeset), bytes.NewBufferString(body)), "user_1")
	secondRec := httptest.NewRecorder()
	router.ServeHTTP(secondRec, second)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("replayed accept status = %d, want %d (body: %s)", secondRec.Code, http.StatusOK, secondRec.Body.String())
	}

	if firstRec.Body.String() != secondRec.Body.String() {
		t.Errorf("replayed accept body = %s, want identical to the first response %s", secondRec.Body.String(), firstRec.Body.String())
	}

	tasks, err := repo.ListTasks(context.Background(), "user_1", project.ID)
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("ListTasks() returned %d tasks, want 1 (replay must not re-apply)", len(tasks))
	}
}

// TestAcceptChangeset_OversizedChangeset documents ADR 009: a changeset
// over the 50-mutation cap is rejected with 400 before any write, not
// partially applied.
func TestAcceptChangeset_OversizedChangeset(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	tasks := make([]domain.ProposedTask, 51)
	for i := range tasks {
		tasks[i] = domain.ProposedTask{Title: fmt.Sprintf("Task %d", i)}
	}
	project, changeset := createProjectAndChangeset(t, repo, "user_1", tasks)

	body := `{"idempotency_key": "idem-key-1"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, acceptChangesetPath(project, changeset), bytes.NewBufferString(body)), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	got, err := repo.ListTasks(context.Background(), "user_1", project.ID)
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListTasks() returned %d tasks, want 0 (rejected before any write)", len(got))
	}

	gotChangeset, err := repo.GetChangeset(context.Background(), "user_1", project.ID, changeset.ID)
	if err != nil {
		t.Fatalf("GetChangeset() error = %v", err)
	}
	if gotChangeset.Status != domain.ChangesetProposed {
		t.Errorf("Changeset.Status = %q, want unchanged %q", gotChangeset.Status, domain.ChangesetProposed)
	}
}

// createCreateProjectChangeset is shared setup for the project-less
// accept-changeset tests below: a proposed create_project changeset, ready
// to accept — createCreateProjectAndChangeset's counterpart for a
// changeset with no project yet.
func createCreateProjectChangeset(t *testing.T, repo *store.MemoryRepository, userID string) domain.Changeset {
	t.Helper()

	changeset, err := repo.CreateChangeset(context.Background(), domain.Changeset{
		UserID: userID,
		Skill:  "create_project",
		Status: domain.ChangesetProposed,
		ProposedProject: &domain.ProposedProject{
			Name: "Tidepool Sync",
			Goal: "Ship the sync engine",
		},
	})
	if err != nil {
		t.Fatalf("CreateChangeset() error = %v", err)
	}
	return changeset
}

func acceptCreateProjectChangesetPath(changeset domain.Changeset) string {
	return "/changesets/" + changeset.ID + "/accept"
}

func TestAcceptCreateProjectChangeset_Success(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	changeset := createCreateProjectChangeset(t, repo, "user_1")

	body := `{"idempotency_key": "idem-key-1"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, acceptCreateProjectChangesetPath(changeset), bytes.NewBufferString(body)), "user_1")
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
		t.Error("Project.ID = \"\", want a generated ID")
	}
	if got.Name != "Tidepool Sync" || got.Goal != "Ship the sync engine" {
		t.Errorf("Project = %+v, want the proposed name/goal", got)
	}

	gotChangeset, err := repo.GetChangeset(context.Background(), "user_1", "", changeset.ID)
	if err != nil {
		t.Fatalf("GetChangeset() error = %v", err)
	}
	if gotChangeset.Status != domain.ChangesetApplied {
		t.Errorf("Changeset.Status = %q, want %q", gotChangeset.Status, domain.ChangesetApplied)
	}
}

func TestAcceptCreateProjectChangeset_MissingIdempotencyKey(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	changeset := createCreateProjectChangeset(t, repo, "user_1")

	req := withUserID(httptest.NewRequest(http.MethodPost, acceptCreateProjectChangesetPath(changeset), bytes.NewBufferString("{}")), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestAcceptCreateProjectChangeset_InvalidBody(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	changeset := createCreateProjectChangeset(t, repo, "user_1")

	req := withUserID(httptest.NewRequest(http.MethodPost, acceptCreateProjectChangesetPath(changeset), bytes.NewBufferString("not json")), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAcceptCreateProjectChangeset_ChangesetNotFound(t *testing.T) {
	router := NewRouter(store.NewMemoryRepository())

	body := `{"idempotency_key": "idem-key-1"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, "/changesets/does-not-exist/accept", bytes.NewBufferString(body)), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestAcceptCreateProjectChangeset_WrongUser documents the same
// structural 404-not-403 convention as TestAcceptChangeset_WrongUser: a
// changeset
// belonging to a different user isn't found, rather than being rejected
// with a permission error that would confirm it exists.
func TestAcceptCreateProjectChangeset_WrongUser(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	changeset := createCreateProjectChangeset(t, repo, "user_1")

	body := `{"idempotency_key": "idem-key-1"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, acceptCreateProjectChangesetPath(changeset), bytes.NewBufferString(body)), "user_2")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestAcceptCreateProjectChangeset_RealTaskChangesetNotFound documents
// that a normal task changeset (the shape acceptChangesetHandler handles)
// is structurally unreachable through this route: it lives under its
// project's real SK, never the NOPROJECT placeholder this route always
// looks up, so it 404s rather than ever reaching the create_project shape
// check below.
func TestAcceptCreateProjectChangeset_RealTaskChangesetNotFound(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	_, changeset := createProjectAndChangeset(t, repo, "user_1", []domain.ProposedTask{{Title: "First"}})

	body := `{"idempotency_key": "idem-key-1"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, "/changesets/"+changeset.ID+"/accept", bytes.NewBufferString(body)), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestAcceptCreateProjectChangeset_NotCreateProject documents the
// defensive shape check itself: a project-less changeset whose skill
// isn't create_project (a data bug, or a future project-less skill)
// 400s rather than being treated as a create_project proposal.
func TestAcceptCreateProjectChangeset_NotCreateProject(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	changeset, err := repo.CreateChangeset(context.Background(), domain.Changeset{
		UserID: "user_1",
		Skill:  "some_other_skill",
		Status: domain.ChangesetProposed,
	})
	if err != nil {
		t.Fatalf("CreateChangeset() error = %v", err)
	}

	body := `{"idempotency_key": "idem-key-1"}`
	req := withUserID(httptest.NewRequest(http.MethodPost, acceptCreateProjectChangesetPath(changeset), bytes.NewBufferString(body)), "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// TestAcceptCreateProjectChangeset_NotProposed documents that only a
// "proposed" changeset can be accepted — a second, distinct accept attempt
// against an already-applied one 409s rather than creating a second
// project.
func TestAcceptCreateProjectChangeset_NotProposed(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	changeset := createCreateProjectChangeset(t, repo, "user_1")

	body := `{"idempotency_key": "idem-key-1"}`
	first := withUserID(httptest.NewRequest(http.MethodPost, acceptCreateProjectChangesetPath(changeset), bytes.NewBufferString(body)), "user_1")
	firstRec := httptest.NewRecorder()
	router.ServeHTTP(firstRec, first)
	if firstRec.Code != http.StatusCreated {
		t.Fatalf("first accept status = %d, want %d (body: %s)", firstRec.Code, http.StatusCreated, firstRec.Body.String())
	}

	secondBody := `{"idempotency_key": "idem-key-2"}`
	second := withUserID(httptest.NewRequest(http.MethodPost, acceptCreateProjectChangesetPath(changeset), bytes.NewBufferString(secondBody)), "user_1")
	secondRec := httptest.NewRecorder()
	router.ServeHTTP(secondRec, second)

	if secondRec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body: %s)", secondRec.Code, http.StatusConflict, secondRec.Body.String())
	}

	projects, err := repo.ListProjects(context.Background(), "user_1")
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(projects) != 1 {
		t.Errorf("ListProjects() returned %d projects, want 1 (must not double-apply)", len(projects))
	}
}

// TestAcceptCreateProjectChangeset_IdempotentReplay documents
// architecture.md §15 for this path: replaying an accept with the same
// idempotency key returns the original result instead of creating a
// second project.
func TestAcceptCreateProjectChangeset_IdempotentReplay(t *testing.T) {
	repo := store.NewMemoryRepository()
	router := NewRouter(repo)

	changeset := createCreateProjectChangeset(t, repo, "user_1")

	body := `{"idempotency_key": "idem-key-1"}`

	first := withUserID(httptest.NewRequest(http.MethodPost, acceptCreateProjectChangesetPath(changeset), bytes.NewBufferString(body)), "user_1")
	firstRec := httptest.NewRecorder()
	router.ServeHTTP(firstRec, first)
	if firstRec.Code != http.StatusCreated {
		t.Fatalf("first accept status = %d, want %d (body: %s)", firstRec.Code, http.StatusCreated, firstRec.Body.String())
	}

	second := withUserID(httptest.NewRequest(http.MethodPost, acceptCreateProjectChangesetPath(changeset), bytes.NewBufferString(body)), "user_1")
	secondRec := httptest.NewRecorder()
	router.ServeHTTP(secondRec, second)
	if secondRec.Code != http.StatusCreated {
		t.Fatalf("replayed accept status = %d, want %d (body: %s)", secondRec.Code, http.StatusCreated, secondRec.Body.String())
	}

	if firstRec.Body.String() != secondRec.Body.String() {
		t.Errorf("replayed accept body = %s, want identical to the first response %s", secondRec.Body.String(), firstRec.Body.String())
	}

	projects, err := repo.ListProjects(context.Background(), "user_1")
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(projects) != 1 {
		t.Errorf("ListProjects() returned %d projects, want 1 (replay must not create a second)", len(projects))
	}
}
