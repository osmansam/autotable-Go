package utils

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestNormalizeErrorResponse(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		message     string
		wantStatus  int
		wantQuiet   bool
		wantMessage string
	}{
		{
			name:        "context canceled is quiet",
			err:         context.Canceled,
			wantQuiet:   true,
			wantStatus:  0,
			wantMessage: "",
		},
		{
			name:        "deadline exceeded is gateway timeout",
			err:         context.DeadlineExceeded,
			message:     "database failed",
			wantStatus:  http.StatusGatewayTimeout,
			wantMessage: "Request timed out",
		},
		{
			name:        "validation-like errors are bad requests",
			err:         errors.New("invalid pagination parameters"),
			message:     "Invalid pagination parameters",
			wantStatus:  http.StatusBadRequest,
			wantMessage: "Invalid pagination parameters",
		},
		{
			name:        "unexpected errors are internal errors",
			err:         errors.New("mongo connection failed"),
			message:     "Failed to fetch items",
			wantStatus:  http.StatusInternalServerError,
			wantMessage: "Failed to fetch items",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeErrorResponse(tt.err, tt.message)
			if got.Quiet != tt.wantQuiet {
				t.Fatalf("Quiet = %v, want %v", got.Quiet, tt.wantQuiet)
			}
			if got.Status != tt.wantStatus {
				t.Fatalf("Status = %d, want %d", got.Status, tt.wantStatus)
			}
			if got.Message != tt.wantMessage {
				t.Fatalf("Message = %q, want %q", got.Message, tt.wantMessage)
			}
		})
	}
}
