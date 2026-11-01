package handler

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestAPI_ConcurrentSameKey(t *testing.T) {
	routes := newRoutesWithAccounts(t, map[string]int64{"small_account": 104857600}, fakeLimitProvider{maxActive: 5, known: true})

	recorders := reserveSameKeyConcurrentlyOverHTTP(routes, "small_account", "key_shared", 50, 1048576)

	statuses := map[int]int{}
	reservationIDs := map[string]bool{}
	for index, recorder := range recorders {
		statuses[recorder.Code]++
		switch recorder.Code {
		case http.StatusCreated, http.StatusOK:
			reservationIDs[decodeObject(t, recorder)["id"].(string)] = true
		case http.StatusConflict:
			body := decodeObject(t, recorder)
			if body["code"] != "request_in_progress" || recorder.Header().Get("Retry-After") != "1" {
				t.Errorf("request %d: got 409 %v with Retry-After %q, want request_in_progress with Retry-After 1", index, body["code"], recorder.Header().Get("Retry-After"))
			}
		default:
			t.Errorf("request %d: got %d %s, want 201, 200 or 409", index, recorder.Code, recorder.Body.String())
		}
	}
	if statuses[http.StatusCreated] != 1 || statuses[http.StatusOK]+statuses[http.StatusConflict] != 49 {
		t.Errorf("50 concurrent requests with one key: got statuses %v, want one 201 and 49 answers that are 200 or 409", statuses)
	}
	if len(reservationIDs) != 1 {
		t.Errorf("reservation IDs in the 201 and 200 answers: got %d different ones, want 1", len(reservationIDs))
	}
	assertUsage(t, routes, "small_account", 1048576, 1)
}

func reserveSameKeyConcurrentlyOverHTTP(routes http.Handler, accountID, idempotencyKey string, requestCount int, bytes int64) []*httptest.ResponseRecorder {
	start := make(chan struct{})
	var requests sync.WaitGroup
	recorders := make([]*httptest.ResponseRecorder, requestCount)
	for index := range recorders {
		requests.Go(func() {
			<-start
			recorders[index] = reserve(routes, accountID, idempotencyKey, bytes)
		})
	}
	close(start)
	requests.Wait()
	return recorders
}
