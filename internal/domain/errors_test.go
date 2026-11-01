package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrors_Messages(t *testing.T) {
	testCases := []struct {
		name        string
		err         error
		wantMessage string
	}{
		{"account_not_found", ErrAccountNotFound, "account not found"},
		{"reservation_not_found", ErrReservationNotFound, "reservation not found"},
		{"request_in_progress", ErrRequestInProgress, "request in progress"},
		{"limit_unavailable", ErrLimitUnavailable, "limit unavailable"},
		{"service_unavailable", ErrServiceUnavailable, "service unavailable"},
		{
			"insufficient_storage",
			InsufficientStorageError{RequestedBytes: 300, AvailableBytes: 200, LimitBytes: 1000},
			"insufficient storage: 300 bytes requested, 200 available out of 1000",
		},
		{
			"too_many_active_reservations",
			TooManyActiveReservationsError{ActiveReservations: 5, MaxActiveReservations: 5},
			"too many active reservations: 5 active, 5 allowed",
		},
		{
			"idempotency_key_reused",
			IdempotencyKeyReusedError{OriginalBytes: 4194304},
			"idempotency key reused: it first reserved 4194304 bytes",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			wrapped := fmt.Errorf("reserve: %w", testCase.err)

			wantMessage := "reserve: " + testCase.wantMessage
			if got := wrapped.Error(); got != wantMessage {
				t.Errorf("message: got %q, want %q", got, wantMessage)
			}
			if !errors.Is(wrapped, testCase.err) {
				t.Errorf("errors.Is(%q, %q): got false, want true", wrapped, testCase.err)
			}
		})
	}
}
