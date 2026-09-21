package skillsapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openai/openai-go"

	"github.com/esvarez/lucas-assist/internal/agent"
	"github.com/esvarez/lucas-assist/internal/domain"
)

// fakeSkill is only used to populate a registry for the tests below. Its
// BuildContext is what this package's tests exercise for input validation
// — no network call happens in this package's tests.
type fakeSkill struct {
	name         string
	buildContext func(context.Context, json.RawMessage) ([]openai.ChatCompletionMessageParamUnion, error)
}

func (f fakeSkill) Name() string { return f.name }
func (f fakeSkill) BuildContext(ctx context.Context, raw json.RawMessage) ([]openai.ChatCompletionMessageParamUnion, error) {
	if f.buildContext != nil {
		return f.buildContext(ctx, raw)
	}
	return nil, nil
}
func (f fakeSkill) ResponseFormat() openai.ResponseFormatJSONSchemaParam {
	return openai.ResponseFormatJSONSchemaParam{}
}
func (f fakeSkill) Tools() []openai.ChatCompletionToolParam { return nil }
func (f fakeSkill) Parse(json.RawMessage) (any, error)      { return nil, nil }

// fakeAgentRunCreator stubs AgentRunStore so tests don't need a real
// store.Repository. domain.AgentRun embeds a json.RawMessage (a slice), so
// it isn't comparable with == — called tracks invocation instead of
// comparing lastInput against a zero value.
type fakeAgentRunCreator struct {
	createErr error
	created   domain.AgentRun

	called    bool
	lastInput domain.AgentRun

	failErr      error
	failCalled   bool
	failRunID    string
	failErrorMsg string
}

func (f *fakeAgentRunCreator) CreateAgentRun(_ context.Context, r domain.AgentRun) (domain.AgentRun, error) {
	f.called = true
	f.lastInput = r
	if f.createErr != nil {
		return domain.AgentRun{}, f.createErr
	}
	if f.created.ID == "" {
		f.created = r
		f.created.ID = "run_1"
	}
	return f.created, nil
}

func (f *fakeAgentRunCreator) FailAgentRun(_ context.Context, _, _, runID, errMsg string) (domain.AgentRun, error) {
	f.failCalled = true
	f.failRunID = runID
	f.failErrorMsg = errMsg
	if f.failErr != nil {
		return domain.AgentRun{}, f.failErr
	}
	return domain.AgentRun{ID: runID, Status: domain.AgentRunFailed, Error: errMsg}, nil
}

// fakeEnqueuer stubs Enqueuer so tests don't need a real SQS client.
type fakeEnqueuer struct {
	enqueueErr    error
	lastRunID     string
	lastUserID    string
	lastProjectID string
}

func (f *fakeEnqueuer) EnqueueRun(_ context.Context, userID, projectID, runID string) error {
	f.lastUserID = userID
	f.lastProjectID = projectID
	f.lastRunID = runID
	return f.enqueueErr
}

func TestHandler_InvalidRequestBody(t *testing.T) {
	handler := NewHandler(agent.NewRegistry(fakeSkill{name: "widget"}), &fakeAgentRunCreator{}, &fakeEnqueuer{})

	req := httptest.NewRequest(http.MethodPost, "/skills", bytes.NewBufferString("not json"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandler_MissingUserID(t *testing.T) {
	handler := NewHandler(agent.NewRegistry(fakeSkill{name: "widget"}), &fakeAgentRunCreator{}, &fakeEnqueuer{})

	body := `{"skill": "widget", "input": {}}`
	req := httptest.NewRequest(http.MethodPost, "/skills", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandler_UnknownSkill(t *testing.T) {
	handler := NewHandler(agent.NewRegistry(fakeSkill{name: "widget"}), &fakeAgentRunCreator{}, &fakeEnqueuer{})

	body := `{"skill": "does-not-exist", "input": {}, "user_id": "user_1"}`
	req := httptest.NewRequest(http.MethodPost, "/skills", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestHandler_InvalidInput_DoesNotCreateOrEnqueue documents the issue #99
// acceptance criterion: malformed skill input still 400s without creating
// an AgentRun or enqueueing anything.
func TestHandler_InvalidInput_DoesNotCreateOrEnqueue(t *testing.T) {
	skill := fakeSkill{
		name: "widget",
		buildContext: func(context.Context, json.RawMessage) ([]openai.ChatCompletionMessageParamUnion, error) {
			return nil, fmt.Errorf("widget: unmarshal input: %w: bad shape", agent.ErrInvalidInput)
		},
	}
	runs := &fakeAgentRunCreator{}
	enqueuer := &fakeEnqueuer{}
	handler := NewHandler(agent.NewRegistry(skill), runs, enqueuer)

	body := `{"skill": "widget", "input": {}, "user_id": "user_1"}`
	req := httptest.NewRequest(http.MethodPost, "/skills", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if runs.called {
		t.Errorf("CreateAgentRun() was called with %+v, want not called", runs.lastInput)
	}
	if enqueuer.lastRunID != "" {
		t.Errorf("EnqueueRun() was called with %q, want not called", enqueuer.lastRunID)
	}
}

func TestHandler_Accepted(t *testing.T) {
	runs := &fakeAgentRunCreator{}
	enqueuer := &fakeEnqueuer{}
	handler := NewHandler(agent.NewRegistry(fakeSkill{name: "widget"}), runs, enqueuer)

	body := `{"skill": "widget", "input": {"foo":"bar"}, "user_id": "user_1", "project_id": "proj_1"}`
	req := httptest.NewRequest(http.MethodPost, "/skills", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	if runs.lastInput.UserID != "user_1" || runs.lastInput.ProjectID != "proj_1" || runs.lastInput.Skill != "widget" {
		t.Errorf("CreateAgentRun() called with %+v, want UserID/ProjectID/Skill from the request", runs.lastInput)
	}
	if string(runs.lastInput.Input) != `{"foo":"bar"}` {
		t.Errorf("CreateAgentRun() Input = %s, want the request's raw input", runs.lastInput.Input)
	}

	if enqueuer.lastRunID != "run_1" {
		t.Errorf("EnqueueRun() called with %q, want the created run's ID %q", enqueuer.lastRunID, "run_1")
	}
	if enqueuer.lastUserID != "user_1" || enqueuer.lastProjectID != "proj_1" {
		t.Errorf("EnqueueRun() called with userID=%q projectID=%q, want %q/%q (the worker needs these to look the run up)", enqueuer.lastUserID, enqueuer.lastProjectID, "user_1", "proj_1")
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["run_id"] != "run_1" {
		t.Errorf("response run_id = %q, want %q", resp["run_id"], "run_1")
	}
}

func TestHandler_CreateAgentRunError(t *testing.T) {
	runs := &fakeAgentRunCreator{createErr: errors.New("dynamo unavailable")}
	enqueuer := &fakeEnqueuer{}
	handler := NewHandler(agent.NewRegistry(fakeSkill{name: "widget"}), runs, enqueuer)

	body := `{"skill": "widget", "input": {}, "user_id": "user_1"}`
	req := httptest.NewRequest(http.MethodPost, "/skills", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if enqueuer.lastRunID != "" {
		t.Errorf("EnqueueRun() was called with %q, want not called after CreateAgentRun failed", enqueuer.lastRunID)
	}
}

// TestHandler_EnqueueError_MarksRunFailed documents the fix for the review
// finding on PR #115: a run whose enqueue fails must not stay "queued"
// forever with no worker able to ever lease it and no run_id the caller can
// retry against. The handler marks it failed instead of orphaning it.
func TestHandler_EnqueueError_MarksRunFailed(t *testing.T) {
	runs := &fakeAgentRunCreator{}
	enqueueErr := errors.New("sqs unavailable")
	enqueuer := &fakeEnqueuer{enqueueErr: enqueueErr}
	handler := NewHandler(agent.NewRegistry(fakeSkill{name: "widget"}), runs, enqueuer)

	body := `{"skill": "widget", "input": {}, "user_id": "user_1", "project_id": "proj_1"}`
	req := httptest.NewRequest(http.MethodPost, "/skills", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}

	if !runs.failCalled {
		t.Fatal("FailAgentRun() was not called after EnqueueRun failed — the run is left orphaned in queued")
	}
	if runs.failRunID != "run_1" {
		t.Errorf("FailAgentRun() runID = %q, want %q", runs.failRunID, "run_1")
	}
	if runs.failErrorMsg != enqueueErr.Error() {
		t.Errorf("FailAgentRun() errMsg = %q, want %q", runs.failErrorMsg, enqueueErr.Error())
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["run_id"] != "run_1" {
		t.Errorf("response run_id = %q, want %q so the caller can still identify the failed run", resp["run_id"], "run_1")
	}
}

// TestHandler_EnqueueError_FailAgentRunAlsoFails documents that the handler
// still reports the original enqueue error (rather than panicking or
// masking it) when marking the run failed doesn't work either.
func TestHandler_EnqueueError_FailAgentRunAlsoFails(t *testing.T) {
	runs := &fakeAgentRunCreator{failErr: errors.New("dynamo unavailable")}
	enqueuer := &fakeEnqueuer{enqueueErr: errors.New("sqs unavailable")}
	handler := NewHandler(agent.NewRegistry(fakeSkill{name: "widget"}), runs, enqueuer)

	body := `{"skill": "widget", "input": {}, "user_id": "user_1"}`
	req := httptest.NewRequest(http.MethodPost, "/skills", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if !runs.failCalled {
		t.Fatal("FailAgentRun() was not called")
	}
}

func TestStatusForBuildContextError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "invalid input",
			err:  fmt.Errorf("decompose_task: unmarshal input: %w: bad shape", agent.ErrInvalidInput),
			want: http.StatusBadRequest,
		},
		{
			name: "unexpected error",
			err:  errors.New("something else went wrong"),
			want: http.StatusInternalServerError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusForBuildContextError(tc.err); got != tc.want {
				t.Errorf("statusForBuildContextError(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}
