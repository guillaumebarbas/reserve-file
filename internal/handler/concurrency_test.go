package handler

import (
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
)

func TestAPI_ConcurrentReservations(t *testing.T) {
	testCases := []struct {
		name           string
		setupBytes     []int64
		requestCount   int
		requestedBytes int64
		wantOutcomes   map[string]int
		wantReserved   int64
		wantActive     int
	}{
		{
			name:           "room_for_three",
			setupBytes:     []int64{31457280},
			requestCount:   20,
			requestedBytes: 20971520,
			wantOutcomes:   map[string]int{"201": 3, "409 insufficient_storage": 17},
			wantReserved:   31457280 + 3*20971520,
			wantActive:     4,
		},
		{
			name:           "five_slots",
			requestCount:   50,
			requestedBytes: 1048576,
			wantOutcomes:   map[string]int{"201": 5, "409 too_many_active_reservations": 45},
			wantReserved:   5 * 1048576,
			wantActive:     5,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			routes := newRoutesWithAccounts(t, map[string]int64{"small_account": 104857600}, fakeLimitProvider{maxActive: 5, known: true})
			for index, bytes := range testCase.setupBytes {
				if recorder := reserve(routes, "small_account", fmt.Sprintf("setup_key_%d", index), bytes); recorder.Code != http.StatusCreated {
					t.Fatalf("setup reservation %d: got %d %s, want %d", index, recorder.Code, recorder.Body.String(), http.StatusCreated)
				}
			}

			recorders := reserveConcurrentlyOverHTTP(routes, "small_account", testCase.requestCount, testCase.requestedBytes)

			outcomes := map[string]int{}
			for _, recorder := range recorders {
				outcome := strconv.Itoa(recorder.Code)
				if recorder.Code != http.StatusCreated {
					outcome += fmt.Sprintf(" %v", decodeObject(t, recorder)["code"])
				}
				outcomes[outcome]++
			}
			if !maps.Equal(outcomes, testCase.wantOutcomes) {
				t.Errorf("%d concurrent requests of %d bytes: got %v, want %v", testCase.requestCount, testCase.requestedBytes, outcomes, testCase.wantOutcomes)
			}
			assertUsage(t, routes, "small_account", testCase.wantReserved, testCase.wantActive)
		})
	}
}

func reserveConcurrentlyOverHTTP(routes http.Handler, accountID string, requestCount int, bytes int64) []*httptest.ResponseRecorder {
	start := make(chan struct{})
	var requests sync.WaitGroup
	recorders := make([]*httptest.ResponseRecorder, requestCount)
	for index := range recorders {
		requests.Go(func() {
			<-start
			recorders[index] = reserve(routes, accountID, fmt.Sprintf("concurrent_key_%d", index), bytes)
		})
	}
	close(start)
	requests.Wait()
	return recorders
}
