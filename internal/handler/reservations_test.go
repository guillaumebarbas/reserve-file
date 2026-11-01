package handler

import (
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAPI_ReserveCreated(t *testing.T) {
	routes := newRoutesWithAccounts(t, map[string]int64{"small_account": 104857600}, fakeLimitProvider{maxActive: 5, known: true})

	recorder := reserve(routes, "small_account", "key_1", 31457280)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("POST /accounts/small_account/reservations: got %d %s, want %d", recorder.Code, recorder.Body.String(), http.StatusCreated)
	}
	body := decodeObject(t, recorder)
	reservationID, _ := body["id"].(string)
	if identifier, err := uuid.Parse(reservationID); err != nil || identifier.Version() != 7 {
		t.Errorf("id %q: want a UUIDv7", reservationID)
	}
	if location := recorder.Header().Get("Location"); location != "/accounts/small_account/reservations/"+reservationID {
		t.Errorf("Location: got %q, want %q", location, "/accounts/small_account/reservations/"+reservationID)
	}
	createdAt, _ := body["created_at"].(string)
	if _, err := time.Parse(time.RFC3339, createdAt); err != nil || !strings.HasSuffix(createdAt, "Z") {
		t.Errorf("created_at %q: want an RFC 3339 time in UTC", createdAt)
	}
	wantBody := map[string]any{"id": reservationID, "account_id": "small_account", "bytes": 31457280.0, "status": "active", "created_at": createdAt, "released_at": nil}
	if !maps.Equal(body, wantBody) {
		t.Errorf("body:\n got  %v\n want %v", body, wantBody)
	}
	assertUsage(t, routes, "small_account", 31457280, 1)
}

func TestAPI_ReserveRetry(t *testing.T) {
	routes := newRoutesWithAccounts(t, map[string]int64{"small_account": 104857600}, fakeLimitProvider{maxActive: 5, known: true})
	first := reserve(routes, "small_account", "key_1", 31457280)
	if first.Code != http.StatusCreated {
		t.Fatalf("first attempt: got %d %s, want %d", first.Code, first.Body.String(), http.StatusCreated)
	}
	firstBody := decodeObject(t, first)

	for attempt := 1; attempt <= 10; attempt++ {
		retry := reserve(routes, "small_account", "key_1", 31457280)

		if retry.Code != http.StatusOK || retry.Header().Get("Idempotent-Replayed") != "true" {
			t.Errorf("retry %d: got %d with Idempotent-Replayed %q, want %d with \"true\"", attempt, retry.Code, retry.Header().Get("Idempotent-Replayed"), http.StatusOK)
		}
		if body := decodeObject(t, retry); !maps.Equal(body, firstBody) {
			t.Errorf("retry %d body:\n got  %v\n want the first reservation %v", attempt, body, firstBody)
		}
	}

	assertUsage(t, routes, "small_account", 31457280, 1)
}

func TestAPI_KeyReuseWithDifferentBytes(t *testing.T) {
	routes := newRoutesWithAccounts(t, map[string]int64{"small_account": 104857600}, fakeLimitProvider{maxActive: 5, known: true})
	first := reserve(routes, "small_account", "key_1", 31457280)
	if first.Code != http.StatusCreated {
		t.Fatalf("first attempt: got %d %s, want %d", first.Code, first.Body.String(), http.StatusCreated)
	}

	reused := reserve(routes, "small_account", "key_1", 32505856)

	if reused.Code != http.StatusUnprocessableEntity {
		t.Fatalf("same key with 31 MiB after 30 MiB: got %d %s, want %d", reused.Code, reused.Body.String(), http.StatusUnprocessableEntity)
	}
	body := decodeObject(t, reused)
	wantDetail := "This Idempotency-Key was already used to reserve 31457280 bytes. Use a new key for a new request."
	if body["code"] != "idempotency_key_reused" || body["original_bytes"] != 31457280.0 || body["detail"] != wantDetail {
		t.Errorf("same key with 31 MiB after 30 MiB: got code %v, original_bytes %v and detail %q, want idempotency_key_reused, 31457280 and %q",
			body["code"], body["original_bytes"], body["detail"], wantDetail)
	}
	assertUsage(t, routes, "small_account", 31457280, 1)
}

func TestAPI_ReserveRefused(t *testing.T) {
	testCases := []struct {
		name           string
		accountID      string
		setupBytes     []int64
		requestedBytes int64
		wantStatus     int
		wantCode       string
		wantDetail     string
		wantFields     map[string]any
	}{
		{
			name:           "insufficient_storage",
			accountID:      "small_account",
			setupBytes:     []int64{94371840},
			requestedBytes: 20971520,
			wantStatus:     http.StatusConflict,
			wantCode:       "insufficient_storage",
			wantDetail:     "Requested 20.00 MiB but only 10.00 MiB is available (limit 100.00 MiB). Release a reservation or request less space.",
			wantFields:     map[string]any{"requested_bytes": 20971520.0, "available_bytes": 10485760.0, "limit_bytes": 104857600.0},
		},
		{
			name:           "too_many_active_reservations",
			accountID:      "small_account",
			setupBytes:     []int64{1048576, 1048576, 1048576, 1048576, 1048576},
			requestedBytes: 1048576,
			wantStatus:     http.StatusConflict,
			wantCode:       "too_many_active_reservations",
			wantDetail:     "You have 5 active reservations and the maximum is 5. Release a reservation before creating a new one.",
			wantFields:     map[string]any{"active_reservations": 5.0, "max_active_reservations": 5.0},
		},
		{
			name:           "unknown_account",
			accountID:      "unknown_account",
			requestedBytes: 1048576,
			wantStatus:     http.StatusNotFound,
			wantCode:       "account_not_found",
			wantDetail:     "Account 'unknown_account' does not exist.",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			routes := newRoutesWithAccounts(t, map[string]int64{"small_account": 104857600}, fakeLimitProvider{maxActive: 5, known: true})
			var setupTotalBytes int64
			for index, bytes := range testCase.setupBytes {
				if recorder := reserve(routes, "small_account", fmt.Sprintf("setup_key_%d", index), bytes); recorder.Code != http.StatusCreated {
					t.Fatalf("setup reservation %d: got %d %s, want %d", index, recorder.Code, recorder.Body.String(), http.StatusCreated)
				}
				setupTotalBytes += bytes
			}

			recorder := reserve(routes, testCase.accountID, "key_1", testCase.requestedBytes)

			if recorder.Code != testCase.wantStatus {
				t.Errorf("status: got %d, want %d", recorder.Code, testCase.wantStatus)
			}
			body := decodeObject(t, recorder)
			if body["code"] != testCase.wantCode || body["detail"] != testCase.wantDetail {
				t.Errorf("got code %v and detail %q, want %s and %q", body["code"], body["detail"], testCase.wantCode, testCase.wantDetail)
			}
			for field, wantValue := range testCase.wantFields {
				if body[field] != wantValue {
					t.Errorf("%s: got %v, want %v", field, body[field], wantValue)
				}
			}
			assertUsage(t, routes, "small_account", setupTotalBytes, len(testCase.setupBytes))
		})
	}
}

func TestAPI_ReserveRequiresIdempotencyKey(t *testing.T) {
	testCases := []struct {
		name   string
		header http.Header
	}{
		{"missing_header", http.Header{}},
		{"empty_header", http.Header{"Idempotency-Key": {""}}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			routes := newRoutesWithAccounts(t, map[string]int64{"small_account": 104857600}, fakeLimitProvider{maxActive: 5, known: true})

			recorder := postReservation(routes, "small_account", testCase.header, `{"bytes": 1048576}`)

			wantDetail := "The Idempotency-Key header is required. Generate one key per logical request and reuse it on every retry."
			body := decodeObject(t, recorder)
			if recorder.Code != http.StatusBadRequest || body["code"] != "invalid_idempotency_key" || body["detail"] != wantDetail {
				t.Errorf("got %d %v %q, want 400 invalid_idempotency_key %q", recorder.Code, body["code"], body["detail"], wantDetail)
			}
			assertUsage(t, routes, "small_account", 0, 0)
		})
	}
}

func TestAPI_ReserveRejectsInvalidBytes(t *testing.T) {
	testCases := []struct {
		name       string
		body       string
		wantDetail string
	}{
		{"missing_bytes", `{}`, "bytes is required: the number of bytes to reserve, as a positive integer."},
		{"zero_bytes", `{"bytes": 0}`, "bytes must be a positive integer, got 0."},
		{"negative_bytes", `{"bytes": -5}`, "bytes must be a positive integer, got -5."},
		{"malformed_body", `{"bytes": `, "The request body must be a single JSON object, without unknown fields."},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			routes := newRoutesWithAccounts(t, map[string]int64{"small_account": 104857600}, fakeLimitProvider{maxActive: 5, known: true})

			recorder := postReservation(routes, "small_account", http.Header{"Idempotency-Key": {"key_1"}}, testCase.body)

			body := decodeObject(t, recorder)
			if recorder.Code != http.StatusBadRequest || body["code"] != "invalid_request" || body["detail"] != testCase.wantDetail {
				t.Errorf("body %s: got %d %v %q, want 400 invalid_request %q", testCase.body, recorder.Code, body["code"], body["detail"], testCase.wantDetail)
			}
			assertUsage(t, routes, "small_account", 0, 0)
		})
	}
}

func reserve(routes http.Handler, accountID, idempotencyKey string, bytes int64) *httptest.ResponseRecorder {
	return postReservation(routes, accountID, http.Header{"Idempotency-Key": {idempotencyKey}}, fmt.Sprintf(`{"bytes": %d}`, bytes))
}

func postReservation(routes http.Handler, accountID string, header http.Header, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/accounts/"+accountID+"/reservations", strings.NewReader(body))
	request.Header = header
	recorder := httptest.NewRecorder()
	routes.ServeHTTP(recorder, request)
	return recorder
}

func assertUsage(t *testing.T, routes http.Handler, accountID string, wantReservedBytes int64, wantActiveReservations int) {
	t.Helper()
	usage := decodeObject(t, requestUsage(routes, accountID))
	if usage["reserved_bytes"] != float64(wantReservedBytes) || usage["active_reservations"] != float64(wantActiveReservations) {
		t.Errorf("usage of %s: got %v bytes reserved and %v active, want %d and %d", accountID, usage["reserved_bytes"], usage["active_reservations"], wantReservedBytes, wantActiveReservations)
	}
}
