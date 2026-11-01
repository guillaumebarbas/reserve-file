package handler

import (
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestAPI_ReleaseUnknownAndForeignLookTheSame(t *testing.T) {
	routes := newRoutesWithAccounts(t, map[string]int64{"large_account": 10737418240, "small_account": 104857600}, fakeLimitProvider{maxActive: 5, known: true})
	foreignReservation := decodeObject(t, reserve(routes, "large_account", "key_1", 31457280))
	foreignReservationID, _ := foreignReservation["id"].(string)
	testCases := []struct {
		name          string
		reservationID string
	}{
		{"unknown_id", "01a198a1-6cb4-7ad6-ae5f-baca16281555"},
		{"malformed_id", "not-a-uuid"},
		{"other_account_id", foreignReservationID},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			read := requestReservation(routes, "small_account", testCase.reservationID)
			release := requestRelease(routes, "small_account", testCase.reservationID)

			assertReservationNotFound(t, "GET", read, testCase.reservationID)
			assertReservationNotFound(t, "release", release, testCase.reservationID)
		})
	}

	assertUsage(t, routes, "large_account", 31457280, 1)
	assertUsage(t, routes, "small_account", 0, 0)
	if stillActive := decodeObject(t, requestReservation(routes, "large_account", foreignReservationID)); stillActive["status"] != "active" {
		t.Errorf("large_account's reservation after the attempts through small_account: got status %v, want active", stillActive["status"])
	}
}

func assertReservationNotFound(t *testing.T, operation string, recorder *httptest.ResponseRecorder, reservationID string) {
	t.Helper()
	body := decodeObject(t, recorder)
	wantKeys := []string{"code", "detail", "request_id", "status", "title", "type"}
	if keys := slices.Sorted(maps.Keys(body)); !slices.Equal(keys, wantKeys) {
		t.Errorf("%s of %q through small_account: got the fields %v, want %v", operation, reservationID, keys, wantKeys)
	}
	wantDetail := fmt.Sprintf("Reservation '%s' does not exist for account 'small_account'.", reservationID)
	if recorder.Code != http.StatusNotFound || body["code"] != "reservation_not_found" || body["detail"] != wantDetail {
		t.Errorf("%s of %q through small_account: got %d %v %q, want 404 reservation_not_found %q", operation, reservationID, recorder.Code, body["code"], body["detail"], wantDetail)
	}
}
