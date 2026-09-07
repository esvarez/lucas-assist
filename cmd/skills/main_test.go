package main

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/esvarez/lucas-assist/internal/agent"
)

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
