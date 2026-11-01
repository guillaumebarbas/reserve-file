package domain

import "time"

type ReservationStatus string

const (
	StatusActive   ReservationStatus = "active"
	StatusReleased ReservationStatus = "released"
)

type Reservation struct {
	ID             string
	AccountID      string
	IdempotencyKey string
	Bytes          int64
	Status         ReservationStatus
	CreatedAt      time.Time
	ReleasedAt     time.Time
}

type Outcome int

const (
	Created Outcome = iota + 1
	Replayed
)

type MaxActive struct {
	Value int
	Known bool
}
