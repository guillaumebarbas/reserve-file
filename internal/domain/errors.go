package domain

import (
	"errors"
	"fmt"
)

var (
	ErrAccountNotFound     = errors.New("account not found")
	ErrReservationNotFound = errors.New("reservation not found")
	ErrRequestInProgress   = errors.New("request in progress")
	ErrLimitUnavailable    = errors.New("limit unavailable")
	ErrServiceUnavailable  = errors.New("service unavailable")
	ErrBytesNotPositive    = errors.New("a reservation must reserve at least one byte")
)

type InsufficientStorageError struct {
	RequestedBytes int64
	AvailableBytes int64
	LimitBytes     int64
}

func (refusal InsufficientStorageError) Error() string {
	return fmt.Sprintf("insufficient storage: %d bytes requested, %d available out of %d",
		refusal.RequestedBytes, refusal.AvailableBytes, refusal.LimitBytes)
}

type TooManyActiveReservationsError struct {
	ActiveReservations    int
	MaxActiveReservations int
}

func (refusal TooManyActiveReservationsError) Error() string {
	return fmt.Sprintf("too many active reservations: %d active, %d allowed",
		refusal.ActiveReservations, refusal.MaxActiveReservations)
}

type IdempotencyKeyReusedError struct {
	OriginalBytes int64
}

func (refusal IdempotencyKeyReusedError) Error() string {
	return fmt.Sprintf("idempotency key reused: it first reserved %d bytes", refusal.OriginalBytes)
}
