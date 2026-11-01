package domain

import "time"

func (account *Account) Release(reservation *Reservation, releasedAt time.Time) bool {
	if reservation.Status == StatusReleased {
		return false
	}
	reservation.Status = StatusReleased
	reservation.ReleasedAt = releasedAt
	account.reservedBytes -= reservation.Bytes
	account.activeReservations--
	return true
}
