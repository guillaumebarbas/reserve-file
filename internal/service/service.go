package service

import "time"

type Service struct {
	accounts AccountRepository
	limits   LimitProvider
	now      func() time.Time
	inFlight *inFlightGuard
}

func New(accounts AccountRepository, limits LimitProvider, now func() time.Time) *Service {
	return &Service{accounts: accounts, limits: limits, now: now, inFlight: newInFlightGuard()}
}
