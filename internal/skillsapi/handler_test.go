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
)

// fakeSkill is only used to populate a registry for the tests below, all
// of which return before agent.Run would ever call OpenAI — no network
// call happens in this package's tests.
type fakeSkill struct{ name string }

func (f fakeSkill) Name() string { return f.name }
func (f fakeSkill) BuildContext(context.Context, json.RawMessage) ([]openai.ChatCompletionMessageParamUnion, error) {
	return nil, nil
}
func (f fakeSkill) ResponseFormat() openai.ResponseFormatJSONSchemaParam { return openai.ResponseFormatJSONSchemaParam{} }
func (f fakeSkill) Tools() []openai.ChatCompletionToolParam              { return nil }
func (f fakeSkill) Parse(json.RawMessage) (any, error)                  { return nil, nil }

func TestHandler_InvalidRequestBody(t *testing.T) {
	handler := NewHandler(agent.NewRegistry(fakeSkill{name: "widget"}))

	req := httptest.NewRequest(http.MethodPost, "/skills", bytes.NewBufferString("not json"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandler_UnknownSkill(t *testing.T) {
	handler := NewHandler(agent.NewRegistry(fakeSkill{name: "widget"}))

	body := `{"skill": "does-not-exist", "input": {}}`
	req := httptest.NewRequest(http.MethodPost, "/skills", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestStatusForRunError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "invalid input",
			err:  fmt.Errorf("decompose_task: build context: %w", agent.ErrInvalidInput),
			want: http.StatusBadRequest,
		},
		{
			name: "upstream failure",
			err:  errors.New("decompose_task: chat completion: network is down"),
			want: http.StatusBadGateway,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusForRunError(tc.err); got != tc.want {
				t.Errorf("statusForRunError(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}
