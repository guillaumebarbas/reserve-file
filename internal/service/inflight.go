package service

import "sync"

type inFlightRequest struct {
	accountID      string
	idempotencyKey string
}

type inFlightGuard struct {
	mutex    sync.Mutex
	requests map[inFlightRequest]bool
}

func newInFlightGuard() *inFlightGuard {
	return &inFlightGuard{requests: map[inFlightRequest]bool{}}
}

func (guard *inFlightGuard) enter(request inFlightRequest) bool {
	guard.mutex.Lock()
	defer guard.mutex.Unlock()
	if guard.requests[request] {
		return false
	}
	guard.requests[request] = true
	return true
}

func (guard *inFlightGuard) leave(request inFlightRequest) {
	guard.mutex.Lock()
	defer guard.mutex.Unlock()
	delete(guard.requests, request)
}
