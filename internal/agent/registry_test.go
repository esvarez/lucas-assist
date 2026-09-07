package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/openai/openai-go"
)

type fakeSkill struct {
	name           string
	buildContextFn func(ctx context.Context, raw json.RawMessage) ([]openai.ChatCompletionMessageParamUnion, error)
	responseFormat openai.ResponseFormatJSONSchemaParam
	tools          []openai.ChatCompletionToolParam
	parseFn        func(raw json.RawMessage) (any, error)
}

func (f fakeSkill) Name() string { return f.name }

func (f fakeSkill) BuildContext(ctx context.Context, raw json.RawMessage) ([]openai.ChatCompletionMessageParamUnion, error) {
	return f.buildContextFn(ctx, raw)
}

func (f fakeSkill) ResponseFormat() openai.ResponseFormatJSONSchemaParam { return f.responseFormat }

func (f fakeSkill) Tools() []openai.ChatCompletionToolParam { return f.tools }

func (f fakeSkill) Parse(raw json.RawMessage) (any, error) { return f.parseFn(raw) }

func TestRegistry_Get(t *testing.T) {
	skill := fakeSkill{name: "widget"}
	reg := NewRegistry(skill)

	got, err := reg.Get("widget")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Name() != "widget" {
		t.Errorf("Get() = %+v, want the registered widget skill", got)
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	reg := NewRegistry(fakeSkill{name: "widget"})

	_, err := reg.Get("does-not-exist")
	if err == nil {
		t.Fatal("Get() error = nil, want an error for an unregistered skill")
	}
}
