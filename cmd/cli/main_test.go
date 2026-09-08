package main

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/esvarez/lucas-assist/internal/domain"
)

func TestReadDescription_FromArgs(t *testing.T) {
	got, err := readDescription([]string{"Build", "a", "thing"}, os.Stdin)
	if err != nil {
		t.Fatalf("readDescription() error = %v", err)
	}
	if got != "Build a thing" {
		t.Errorf("readDescription() = %q, want %q", got, "Build a thing")
	}
}

func TestReadDescription_NoArgsNoPipe(t *testing.T) {
	// os.Stdin in a `go test` run is not a char device and not a pipe with
	// data either; simulate "nothing usable" by pointing at an already
	// fully-read pipe instead of relying on the test runner's actual stdin.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	w.Close()

	_, err = readDescription(nil, r)
	if err == nil {
		t.Fatal("readDescription() error = nil, want an error for empty input")
	}
}

func TestReadDescription_FromStdinPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	go func() {
		w.WriteString("Piped description\n")
		w.Close()
	}()

	got, err := readDescription(nil, r)
	if err != nil {
		t.Fatalf("readDescription() error = %v", err)
	}
	if got != "Piped description" {
		t.Errorf("readDescription() = %q, want %q", got, "Piped description")
	}
}

func TestParseConfirmAnswer(t *testing.T) {
	cases := map[string]bool{
		"y":     true,
		"Y":     true,
		"yes":   true,
		"YES":   true,
		" y \n": true,
		"n":     false,
		"no":    false,
		"":      false,
		"sure":  false,
	}
	for input, want := range cases {
		if got := parseConfirmAnswer(input); got != want {
			t.Errorf("parseConfirmAnswer(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestBuildCreateProjectRequest(t *testing.T) {
	deadline := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	proposed := domain.ProposedProject{
		Name:        "Nudge",
		Goal:        "Ship the POC",
		Deadline:    &deadline,
		Constraints: []string{"no VPC"},
	}

	got := buildCreateProjectRequest(proposed)

	if got.Name != proposed.Name || got.Goal != proposed.Goal {
		t.Errorf("buildCreateProjectRequest() = %+v, want Name/Goal preserved", got)
	}
	if got.Deadline != proposed.Deadline {
		t.Errorf("Deadline = %v, want %v", got.Deadline, proposed.Deadline)
	}
	if len(got.Constraints) != 1 || got.Constraints[0] != "no VPC" {
		t.Errorf("Constraints = %v, want [no VPC]", got.Constraints)
	}
	if got.Status != defaultProjectStatus {
		t.Errorf("Status = %q, want the default %q (ProposedProject carries none)", got.Status, defaultProjectStatus)
	}
}

func TestDecodeSkillResponse_OK(t *testing.T) {
	body := []byte(`{"status": "ok", "project": {"name": "Nudge", "goal": "Ship", "deadline": null, "constraints": []}, "questions": null}`)

	result, err := decodeSkillResponse(http.StatusOK, body)
	if err != nil {
		t.Fatalf("decodeSkillResponse() error = %v", err)
	}
	if result.Status != "ok" || result.Project == nil || result.Project.Name != "Nudge" {
		t.Errorf("decodeSkillResponse() = %+v, want a parsed ok result", result)
	}
}

func TestDecodeSkillResponse_NonOKStatus(t *testing.T) {
	_, err := decodeSkillResponse(http.StatusForbidden, []byte(`{"message":"Forbidden"}`))
	if err == nil {
		t.Fatal("decodeSkillResponse() error = nil, want an error for a non-200 status")
	}
}

func TestDecodeSkillResponse_InvalidBody(t *testing.T) {
	_, err := decodeSkillResponse(http.StatusOK, []byte("not json"))
	if err == nil {
		t.Fatal("decodeSkillResponse() error = nil, want an unmarshal error")
	}
}

func TestDecodeCommitResponse_Created(t *testing.T) {
	body := []byte(`{"id": "abc123", "name": "Nudge", "goal": "Ship", "status": "active"}`)

	created, err := decodeCommitResponse(http.StatusCreated, body)
	if err != nil {
		t.Fatalf("decodeCommitResponse() error = %v", err)
	}
	if created.ID != "abc123" || created.Name != "Nudge" {
		t.Errorf("decodeCommitResponse() = %+v, want the parsed created project", created)
	}
}

func TestDecodeCommitResponse_NonCreatedStatus(t *testing.T) {
	_, err := decodeCommitResponse(http.StatusConflict, []byte(`{"error":"duplicate"}`))
	if err == nil {
		t.Fatal("decodeCommitResponse() error = nil, want an error for a non-201 status")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("decodeCommitResponse() error = %v, want it to include the response body", err)
	}
}
