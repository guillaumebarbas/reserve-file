package domain

import (
	"testing"
	"time"
)

func TestAccount_Release(t *testing.T) {
	account := RestoreAccount("small_account", 104857600, 52428800, 2)
	reservation := Reservation{ID: "reservation_1", AccountID: "small_account", IdempotencyKey: "key_1", Bytes: 31457280, Status: StatusActive}
	releasedAt := time.Date(2026, 11, 1, 12, 3, 10, 0, time.UTC)

	released := account.Release(&reservation, releasedAt)

	if !released {
		t.Error("Release of an active reservation: got false, want true")
	}
	if reservation.Status != StatusReleased || !reservation.ReleasedAt.Equal(releasedAt) {
		t.Errorf("reservation after Release: got status %q released at %v, want %q released at %v", reservation.Status, reservation.ReleasedAt, StatusReleased, releasedAt)
	}
	if account.ReservedBytes() != 20971520 || account.ActiveReservations() != 1 {
		t.Errorf("account after Release: got %d bytes reserved and %d active, want 20971520 and 1", account.ReservedBytes(), account.ActiveReservations())
	}
}

func TestAccount_ReleaseTwice(t *testing.T) {
	account := RestoreAccount("small_account", 104857600, 52428800, 2)
	reservation := Reservation{ID: "reservation_1", AccountID: "small_account", IdempotencyKey: "key_1", Bytes: 31457280, Status: StatusActive}
	firstReleasedAt := time.Date(2026, 11, 1, 12, 3, 10, 0, time.UTC)
	account.Release(&reservation, firstReleasedAt)

	releasedAgain := account.Release(&reservation, firstReleasedAt.Add(time.Minute))

	if releasedAgain {
		t.Error("second Release: got true, want false")
	}
	if reservation.Status != StatusReleased || !reservation.ReleasedAt.Equal(firstReleasedAt) {
		t.Errorf("reservation after the second Release: got status %q released at %v, want %q released at %v", reservation.Status, reservation.ReleasedAt, StatusReleased, firstReleasedAt)
	}
	if account.ReservedBytes() != 20971520 || account.ActiveReservations() != 1 {
		t.Errorf("account after the second Release: got %d bytes reserved and %d active, want 20971520 and 1", account.ReservedBytes(), account.ActiveReservations())
	}
}
