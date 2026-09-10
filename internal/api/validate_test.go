package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

type sampleValidateRequest struct {
	UserID string `json:"user_id" validate:"required"`
}

func TestValidateStruct_Passes(t *testing.T) {
	rec := httptest.NewRecorder()

	if ok := validateStruct(rec, sampleValidateRequest{UserID: "user_1"}); !ok {
		t.Fatalf("validateStruct() = false, want true for a valid request (body: %s)", rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Errorf("validateStruct() wrote a response on success: %s", rec.Body.String())
	}
}

func TestValidateStruct_ReportsFailingFieldAndReason(t *testing.T) {
	rec := httptest.NewRecorder()

	if ok := validateStruct(rec, sampleValidateRequest{}); ok {
		t.Fatal("validateStruct() = true, want false for a missing required field")
	}

	var got validationErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	// Uses the json tag ("user_id"), not the Go field name ("UserID") —
	// that's what the caller actually sent.
	msg, ok := got.Fields["user_id"]
	if !ok {
		t.Fatalf("Fields = %#v, want a \"user_id\" entry", got.Fields)
	}
	if msg == "" {
		t.Error(`Fields["user_id"] is empty, want a message explaining the failing rule`)
	}
}
