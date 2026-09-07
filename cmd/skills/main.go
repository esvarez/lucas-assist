// Command skills is the consolidated Skill Lambda entrypoint, exposed via
// a Function URL (architecture.md's POC scope; API Gateway routing to
// this Lambda is a separate infra concern, tracked apart from this).
// Dispatches to the agent.Skill named in the request body — adding a
// skill means adding it to the registry below, not a new binary.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/esvarez/lucas-assist/internal/agent"
	"github.com/esvarez/lucas-assist/internal/agent/skills"
)

// registry is constructed once at package init and reused across
// invocations — never inside the handler.
var registry = agent.NewRegistry(
	skills.DecomposeTaskSkill{},
	skills.CreateProjectSkill{},
)

type requestEnvelope struct {
	Skill string          `json:"skill"`
	Input json.RawMessage `json:"input"`
}

func handler(ctx context.Context, req events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	var envelope requestEnvelope
	if err := json.Unmarshal([]byte(req.Body), &envelope); err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()}), nil
	}

	skill, err := registry.Get(envelope.Skill)
	if err != nil {
		return jsonResponse(http.StatusNotFound, map[string]string{"error": err.Error()}), nil
	}

	result, err := agent.Run(ctx, skill, envelope.Input)
	if err != nil {
		return jsonResponse(statusForRunError(err), map[string]string{"error": err.Error()}), nil
	}

	return jsonResponse(http.StatusOK, result), nil
}

// statusForRunError maps an agent.Run error to an HTTP status: malformed
// or missing skill input is the client's mistake (400), anything else
// (the chat completion call, the model's output) is treated as upstream
// (502) — Run's errors are wrapped with %w all the way down, so this
// checks all the way down too.
func statusForRunError(err error) int {
	if errors.Is(err, agent.ErrInvalidInput) {
		return http.StatusBadRequest
	}
	return http.StatusBadGateway
}

func jsonResponse(status int, body any) events.LambdaFunctionURLResponse {
	b, err := json.Marshal(body)
	if err != nil {
		return events.LambdaFunctionURLResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       `{"error":"failed to marshal response"}`,
			Headers:    map[string]string{"Content-Type": "application/json"},
		}
	}
	return events.LambdaFunctionURLResponse{
		StatusCode: status,
		Body:       string(b),
		Headers:    map[string]string{"Content-Type": "application/json"},
	}
}

func main() {
	lambda.Start(handler)
}
