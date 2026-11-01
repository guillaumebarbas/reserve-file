package handler

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAPI_ReleaseReservation(t *testing.T) {
	routes := newRoutesWithAccounts(t, map[string]int64{"small_account": 104857600}, fakeLimitProvider{maxActive: 5, known: true})
	created := decodeObject(t, reserve(routes, "small_account", "key_1", 31457280))
	reservationID, _ := created["id"].(string)

	recorder := requestRelease(routes, "small_account", reservationID)

	if recorder.Code != http.StatusOK {
		t.Fatalf("POST /accounts/small_account/reservations/%s/release: got %d %s, want %d", reservationID, recorder.Code, recorder.Body.String(), http.StatusOK)
	}
	released := decodeObject(t, recorder)
	releasedAt, _ := released["released_at"].(string)
	if parsed, err := time.Parse(time.RFC3339, releasedAt); err != nil || !strings.HasSuffix(releasedAt, "Z") || parsed.Before(time.Now().Add(-time.Minute)) {
		t.Errorf("released_at %q: want the release time, in RFC 3339 and UTC", releasedAt)
	}
	wantReleased := maps.Clone(created)
	wantReleased["status"] = "released"
	wantReleased["released_at"] = releasedAt
	if !maps.Equal(released, wantReleased) {
		t.Errorf("released body:\n got  %v\n want %v", released, wantReleased)
	}
	assertUsage(t, routes, "small_account", 0, 0)
}

func TestAPI_GetReservation(t *testing.T) {
	routes := newRoutesWithAccounts(t, map[string]int64{"small_account": 104857600}, fakeLimitProvider{maxActive: 5, known: true})
	created := decodeObject(t, reserve(routes, "small_account", "key_1", 31457280))
	reservationID, _ := created["id"].(string)

	recorder := requestReservation(routes, "small_account", reservationID)

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /accounts/small_account/reservations/%s: got %d %s, want %d", reservationID, recorder.Code, recorder.Body.String(), http.StatusOK)
	}
	if got := decodeObject(t, recorder); !maps.Equal(got, created) {
		t.Errorf("read body:\n got  %v\n want the body of its creation %v", got, created)
	}
	assertUsage(t, routes, "small_account", 31457280, 1)
}

func TestAPI_ReleaseIdempotent(t *testing.T) {
	routes := newRoutesWithAccounts(t, map[string]int64{"small_account": 104857600}, fakeLimitProvider{maxActive: 5, known: true})
	created := decodeObject(t, reserve(routes, "small_account", "key_1", 31457280))
	reserve(routes, "small_account", "key_2", 31457280)
	reservationID, _ := created["id"].(string)

	firstRelease := requestRelease(routes, "small_account", reservationID)
	secondRelease := requestRelease(routes, "small_account", reservationID)

	if firstRelease.Code != http.StatusOK || secondRelease.Code != http.StatusOK {
		t.Fatalf("two releases of %s: got %d then %d %s, want 200 twice", reservationID, firstRelease.Code, secondRelease.Code, secondRelease.Body.String())
	}
	if firstBody, secondBody := decodeObject(t, firstRelease), decodeObject(t, secondRelease); !maps.Equal(secondBody, firstBody) {
		t.Errorf("second release body:\n got  %v\n want the first one %v", secondBody, firstBody)
	}
	assertUsage(t, routes, "small_account", 31457280, 1)
}

func TestAPI_ReservationNotFound(t *testing.T) {
	testCases := []struct {
		name       string
		method     string
		path       string
		wantCode   string
		wantDetail string
	}{
		{
			name:       "read_unknown_reservation",
			method:     http.MethodGet,
			path:       "/accounts/small_account/reservations/unknown_reservation",
			wantCode:   "reservation_not_found",
			wantDetail: "Reservation 'unknown_reservation' does not exist for account 'small_account'.",
		},
		{
			name:       "release_unknown_reservation",
			method:     http.MethodPost,
			path:       "/accounts/small_account/reservations/unknown_reservation/release",
			wantCode:   "reservation_not_found",
			wantDetail: "Reservation 'unknown_reservation' does not exist for account 'small_account'.",
		},
		{
			name:       "release_on_unknown_account",
			method:     http.MethodPost,
			path:       "/accounts/unknown_account/reservations/unknown_reservation/release",
			wantCode:   "account_not_found",
			wantDetail: "Account 'unknown_account' does not exist.",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			routes := newRoutesWithAccounts(t, map[string]int64{"small_account": 104857600}, fakeLimitProvider{maxActive: 5, known: true})
			recorder := httptest.NewRecorder()

			routes.ServeHTTP(recorder, httptest.NewRequest(testCase.method, testCase.path, nil))

			body := decodeObject(t, recorder)
			if recorder.Code != http.StatusNotFound || body["code"] != testCase.wantCode || body["detail"] != testCase.wantDetail {
				t.Errorf("%s %s: got %d %v %q, want 404 %s %q", testCase.method, testCase.path, recorder.Code, body["code"], body["detail"], testCase.wantCode, testCase.wantDetail)
			}
			assertUsage(t, routes, "small_account", 0, 0)
		})
	}
}

func requestRelease(routes http.Handler, accountID, reservationID string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	routes.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/accounts/"+accountID+"/reservations/"+reservationID+"/release", nil))
	return recorder
}

func requestReservation(routes http.Handler, accountID, reservationID string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	routes.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/accounts/"+accountID+"/reservations/"+reservationID, nil))
	return recorder
}
