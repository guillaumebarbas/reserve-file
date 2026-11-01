package domain

import "time"

func (account *Account) Release(reservation *Reservation, releasedAt time.Time) {
	reservation.Status = StatusReleased
	reservation.ReleasedAt = releasedAt
	account.reservedBytes -= reservation.Bytes
	account.activeReservations--
}
