package domain

import (
	"errors"
	"math"
	"testing"
)

func TestAccount_AvailableBytesNeverNegative(t *testing.T) {
	testCases := []struct {
		name          string
		limitBytes    int64
		reservedBytes int64
		want          int64
	}{
		{"nothing_reserved", 1000, 0, 1000},
		{"partly_reserved", 1000, 300, 700},
		{"fully_reserved", 1000, 1000, 0},
		{"reserved_above_a_lowered_limit", 500, 800, 0},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			account := RestoreAccount("large_account", testCase.limitBytes, testCase.reservedBytes, 1)

			got := account.AvailableBytes()

			if got != testCase.want {
				t.Errorf("AvailableBytes with a limit of %d and %d reserved: got %d, want %d", testCase.limitBytes, testCase.reservedBytes, got, testCase.want)
			}
		})
	}
}

func TestRestoreAccount_KeepsCounters(t *testing.T) {
	account := RestoreAccount("small_account", 104857600, 30, 1)

	if account.ID != "small_account" || account.LimitBytes != 104857600 || account.ReservedBytes() != 30 || account.ActiveReservations() != 1 {
		t.Errorf("RestoreAccount: got %s with a limit of %d, %d reserved and %d active, want small_account with a limit of 104857600, 30 reserved and 1 active",
			account.ID, account.LimitBytes, account.ReservedBytes(), account.ActiveReservations())
	}
}

func TestAccount_Reserve(t *testing.T) {
	knownLimit := MaxActive{Value: 5, Known: true}
	testCases := []struct {
		name                   string
		limitBytes             int64
		reservedBytes          int64
		activeReservations     int
		maxActive              MaxActive
		requestedBytes         int64
		wantErr                error
		wantReservedBytes      int64
		wantActiveReservations int
	}{
		{name: "fits", limitBytes: 100, reservedBytes: 20, activeReservations: 1, maxActive: knownLimit, requestedBytes: 30, wantReservedBytes: 50, wantActiveReservations: 2},
		{name: "exactly_fills_the_space", limitBytes: 100, reservedBytes: 70, activeReservations: 1, maxActive: knownLimit, requestedBytes: 30, wantReservedBytes: 100, wantActiveReservations: 2},
		{name: "one_byte_too_many", limitBytes: 100, reservedBytes: 70, activeReservations: 1, maxActive: knownLimit, requestedBytes: 31, wantErr: InsufficientStorageError{RequestedBytes: 31, AvailableBytes: 30, LimitBytes: 100}, wantReservedBytes: 70, wantActiveReservations: 1},
		{name: "larger_than_an_empty_account", limitBytes: 100, maxActive: knownLimit, requestedBytes: 101, wantErr: InsufficientStorageError{RequestedBytes: 101, AvailableBytes: 100, LimitBytes: 100}},
		{name: "all_slots_used_checked_before_space", limitBytes: 100, reservedBytes: 99, activeReservations: 5, maxActive: knownLimit, requestedBytes: 50, wantErr: TooManyActiveReservationsError{ActiveReservations: 5, MaxActiveReservations: 5}, wantReservedBytes: 99, wantActiveReservations: 5},
		{name: "limit_unknown_checked_first", limitBytes: 100, activeReservations: 5, maxActive: MaxActive{}, requestedBytes: 500, wantErr: ErrLimitUnavailable, wantActiveReservations: 5},
		{name: "usage_above_a_lowered_limit", limitBytes: 50, reservedBytes: 80, activeReservations: 1, maxActive: knownLimit, requestedBytes: 1, wantErr: InsufficientStorageError{RequestedBytes: 1, AvailableBytes: 0, LimitBytes: 50}, wantReservedBytes: 80, wantActiveReservations: 1},
		{name: "near_max_int64_fits", limitBytes: math.MaxInt64, reservedBytes: math.MaxInt64 - 10, activeReservations: 1, maxActive: knownLimit, requestedBytes: 10, wantReservedBytes: math.MaxInt64, wantActiveReservations: 2},
		{name: "near_max_int64_does_not_overflow", limitBytes: math.MaxInt64, reservedBytes: math.MaxInt64 - 10, activeReservations: 1, maxActive: knownLimit, requestedBytes: 11, wantErr: InsufficientStorageError{RequestedBytes: 11, AvailableBytes: 10, LimitBytes: math.MaxInt64}, wantReservedBytes: math.MaxInt64 - 10, wantActiveReservations: 1},
		{name: "zero_bytes", limitBytes: 100, maxActive: knownLimit, requestedBytes: 0, wantErr: ErrBytesNotPositive},
		{name: "negative_bytes", limitBytes: 100, maxActive: knownLimit, requestedBytes: -5, wantErr: ErrBytesNotPositive},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			account := RestoreAccount("small_account", testCase.limitBytes, testCase.reservedBytes, testCase.activeReservations)
			candidate := Reservation{ID: "reservation_1", AccountID: "small_account", IdempotencyKey: "key_1", Bytes: testCase.requestedBytes, Status: StatusActive}

			err := account.Reserve(candidate, testCase.maxActive)

			if !errors.Is(err, testCase.wantErr) {
				t.Errorf("Reserve(%d bytes): got error %v, want %v", testCase.requestedBytes, err, testCase.wantErr)
			}
			if account.ReservedBytes() != testCase.wantReservedBytes || account.ActiveReservations() != testCase.wantActiveReservations {
				t.Errorf("after Reserve(%d bytes): %d bytes reserved and %d active, want %d and %d",
					testCase.requestedBytes, account.ReservedBytes(), account.ActiveReservations(), testCase.wantReservedBytes, testCase.wantActiveReservations)
			}
		})
	}
}
