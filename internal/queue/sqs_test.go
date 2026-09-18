package queue

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// stubSQSClient lets a test capture the SendMessage call or force whatever
// error it returns, without a real AWS call.
type stubSQSClient struct {
	sendErr error

	lastInput *sqs.SendMessageInput
}

func (s *stubSQSClient) SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	s.lastInput = params
	if s.sendErr != nil {
		return nil, s.sendErr
	}
	return &sqs.SendMessageOutput{}, nil
}

func TestEnqueuer_EnqueueRun(t *testing.T) {
	client := &stubSQSClient{}
	enqueuer := NewEnqueuer(client, "https://sqs.example/queue")

	if err := enqueuer.EnqueueRun(context.Background(), "run_1"); err != nil {
		t.Fatalf("EnqueueRun() error = %v", err)
	}

	if client.lastInput == nil {
		t.Fatal("SendMessage() was not called")
	}
	if got := *client.lastInput.QueueUrl; got != "https://sqs.example/queue" {
		t.Errorf("QueueUrl = %q, want %q", got, "https://sqs.example/queue")
	}

	var body Message
	if err := json.Unmarshal([]byte(*client.lastInput.MessageBody), &body); err != nil {
		t.Fatalf("unmarshal MessageBody: %v", err)
	}
	if body.RunID != "run_1" {
		t.Errorf("Message.RunID = %q, want %q", body.RunID, "run_1")
	}
}

func TestEnqueuer_EnqueueRun_SendError(t *testing.T) {
	sendErr := errors.New("sqs unavailable")
	client := &stubSQSClient{sendErr: sendErr}
	enqueuer := NewEnqueuer(client, "https://sqs.example/queue")

	err := enqueuer.EnqueueRun(context.Background(), "run_1")
	if !errors.Is(err, sendErr) {
		t.Fatalf("EnqueueRun() error = %v, want wrapped %v", err, sendErr)
	}
}
