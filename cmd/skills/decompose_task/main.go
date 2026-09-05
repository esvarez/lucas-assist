// Command decompose_task is a Lambda exposed via a Function URL. POC scope
// per architecture.md: one skill, no API Gateway, no datastore. It exists
// to find out whether the Task struct has the right fields.
package main

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/esvarez/lucas-assist/internal/agent/skills"
)

type requestBody struct {
	TaskTitle       string        `json:"task_title"`
	TaskDescription string        `json:"task_description"`
	Domain          skills.Domain `json:"domain"`
}

func handler(ctx context.Context, req events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	var body requestBody
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()}), nil
	}

	result, err := skills.Decompose(ctx, skills.DecomposeInput{
		TaskTitle:       body.TaskTitle,
		TaskDescription: body.TaskDescription,
		Domain:          body.Domain,
	})
	if err != nil {
		return jsonResponse(http.StatusBadGateway, map[string]string{"error": err.Error()}), nil
	}

	return jsonResponse(http.StatusOK, result), nil
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
