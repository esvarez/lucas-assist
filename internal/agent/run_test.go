package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/openai/openai-go"
)

func fakeCompletion(content string) *openai.ChatCompletion {
	return &openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{
			{Message: openai.ChatCompletionMessage{Content: content}},
		},
	}
}

func withStubbedChat(t *testing.T, stub func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error)) {
	t.Helper()
	original := newChatCompletion
	newChatCompletion = stub
	t.Cleanup(func() { newChatCompletion = original })
}

func TestRun_Success(t *testing.T) {
	wantMessages := []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")}
	wantTools := []openai.ChatCompletionToolParam{{Function: openai.FunctionDefinitionParam{Name: "get_thing"}}}
	wantFormat := openai.ResponseFormatJSONSchemaParam{
		JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{Name: "widget_result"},
	}

	var gotParams openai.ChatCompletionNewParams
	withStubbedChat(t, func(_ context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
		gotParams = params
		return fakeCompletion(`{"ok": true}`), nil
	})

	skill := fakeSkill{
		name: "widget",
		buildContextFn: func(_ context.Context, raw json.RawMessage) ([]openai.ChatCompletionMessageParamUnion, error) {
			return wantMessages, nil
		},
		responseFormat: wantFormat,
		tools:          wantTools,
		parseFn: func(raw json.RawMessage) (any, error) {
			return "parsed:" + string(raw), nil
		},
	}

	got, err := Run(context.Background(), skill, json.RawMessage(`{"input":1}`))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got != `parsed:{"ok": true}` {
		t.Errorf("Run() = %v, want the skill's Parse() output", got)
	}

	if gotParams.Model != Model {
		t.Errorf("Model = %v, want %v", gotParams.Model, Model)
	}
	if len(gotParams.Messages) != 1 || gotParams.Messages[0].OfUser.Content.OfString.Value != "hello" {
		t.Errorf("Messages = %+v, want the skill's BuildContext() output", gotParams.Messages)
	}
	if len(gotParams.Tools) != 1 || gotParams.Tools[0].Function.Name != "get_thing" {
		t.Errorf("Tools = %+v, want the skill's Tools() output", gotParams.Tools)
	}
	if gotParams.ResponseFormat.OfJSONSchema == nil || gotParams.ResponseFormat.OfJSONSchema.JSONSchema.Name != "widget_result" {
		t.Errorf("ResponseFormat = %+v, want the skill's ResponseFormat() output", gotParams.ResponseFormat)
	}
}

func TestRun_BuildContextError(t *testing.T) {
	wantErr := errors.New("bad input")
	skill := fakeSkill{
		name: "widget",
		buildContextFn: func(_ context.Context, raw json.RawMessage) ([]openai.ChatCompletionMessageParamUnion, error) {
			return nil, wantErr
		},
	}

	_, err := Run(context.Background(), skill, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want it to wrap %v", err, wantErr)
	}
}

func TestRun_ChatCompletionError(t *testing.T) {
	wantErr := errors.New("network is down")
	withStubbedChat(t, func(_ context.Context, _ openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
		return nil, wantErr
	})

	skill := fakeSkill{
		name: "widget",
		buildContextFn: func(_ context.Context, raw json.RawMessage) ([]openai.ChatCompletionMessageParamUnion, error) {
			return nil, nil
		},
	}

	_, err := Run(context.Background(), skill, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want it to wrap %v", err, wantErr)
	}
}

func TestRun_ParseError(t *testing.T) {
	wantErr := errors.New("bad output")
	withStubbedChat(t, func(_ context.Context, _ openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
		return fakeCompletion(`not json`), nil
	})

	skill := fakeSkill{
		name: "widget",
		buildContextFn: func(_ context.Context, raw json.RawMessage) ([]openai.ChatCompletionMessageParamUnion, error) {
			return nil, nil
		},
		parseFn: func(raw json.RawMessage) (any, error) {
			return nil, wantErr
		},
	}

	_, err := Run(context.Background(), skill, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want it to wrap %v", err, wantErr)
	}
}
