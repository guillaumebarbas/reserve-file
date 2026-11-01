package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

func TestFormatBytes(t *testing.T) {
	testCases := []struct {
		name      string
		byteCount int64
		want      string
	}{
		{"zero", 0, "0 B"},
		{"largest_count_in_bytes", 1023, "1023 B"},
		{"one_kibibyte", 1024, "1.00 KiB"},
		{"one_and_a_half_kibibytes", 1536, "1.50 KiB"},
		{"one_mebibyte", 1048576, "1.00 MiB"},
		{"thirty_mebibytes", 31457280, "30.00 MiB"},
		{"one_gibibyte", 1073741824, "1.00 GiB"},
		{"one_point_two_gibibytes", 1288490189, "1.20 GiB"},
		{"five_gibibytes", 5368709120, "5.00 GiB"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := formatBytes(testCase.byteCount)

			if got != testCase.want {
				t.Errorf("formatBytes(%d): got %q, want %q", testCase.byteCount, got, testCase.want)
			}
		})
	}
}

func TestWriteError_Catalogue(t *testing.T) {
	testCases := []struct {
		name           string
		err            error
		wantStatus     int
		wantCode       string
		wantTitle      string
		wantDetail     string
		wantFields     map[string]any
		wantRetryAfter string
	}{
		{
			name:       "invalid_request",
			err:        invalidRequest("The request body is not valid JSON."),
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_request",
			wantTitle:  "Invalid request",
			wantDetail: "The request body is not valid JSON.",
		},
		{
			name:       "invalid_idempotency_key",
			err:        invalidIdempotencyKey("The Idempotency-Key header is required. Generate one key per logical request and reuse it on every retry."),
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_idempotency_key",
			wantTitle:  "Invalid idempotency key",
			wantDetail: "The Idempotency-Key header is required. Generate one key per logical request and reuse it on every retry.",
		},
		{
			name:       "request_too_large",
			err:        requestTooLarge(),
			wantStatus: http.StatusRequestEntityTooLarge,
			wantCode:   "request_too_large",
			wantTitle:  "Request too large",
			wantDetail: "The request body must not exceed 4096 bytes.",
		},
		{
			name:       "account_not_found",
			err:        fmt.Errorf("read usage: %w", domain.ErrAccountNotFound),
			wantStatus: http.StatusNotFound,
			wantCode:   "account_not_found",
			wantTitle:  "Account not found",
			wantDetail: "Account 'unknown_account' does not exist.",
		},
		{
			name:       "reservation_not_found",
			err:        fmt.Errorf("release: %w", domain.ErrReservationNotFound),
			wantStatus: http.StatusNotFound,
			wantCode:   "reservation_not_found",
			wantTitle:  "Reservation not found",
			wantDetail: "Reservation 'unknown_reservation' does not exist for account 'unknown_account'.",
		},
		{
			name:           "request_in_progress",
			err:            fmt.Errorf("reserve: %w", domain.ErrRequestInProgress),
			wantStatus:     http.StatusConflict,
			wantCode:       "request_in_progress",
			wantTitle:      "Request in progress",
			wantDetail:     "A request with this Idempotency-Key is still being processed. Retry in a moment to get its result.",
			wantRetryAfter: "1",
		},
		{
			name:       "too_many_active_reservations",
			err:        fmt.Errorf("reserve: %w", domain.TooManyActiveReservationsError{ActiveReservations: 7, MaxActiveReservations: 5}),
			wantStatus: http.StatusConflict,
			wantCode:   "too_many_active_reservations",
			wantTitle:  "Too many active reservations",
			wantDetail: "You have 7 active reservations and the maximum is 5. Release a reservation before creating a new one.",
			wantFields: map[string]any{"active_reservations": 7.0, "max_active_reservations": 5.0},
		},
		{
			name:       "insufficient_storage",
			err:        fmt.Errorf("reserve: %w", domain.InsufficientStorageError{RequestedBytes: 5368709120, AvailableBytes: 1288490189, LimitBytes: 10737418240}),
			wantStatus: http.StatusConflict,
			wantCode:   "insufficient_storage",
			wantTitle:  "Insufficient storage",
			wantDetail: "Requested 5.00 GiB but only 1.20 GiB is available (limit 10.00 GiB). Release a reservation or request less space.",
			wantFields: map[string]any{"requested_bytes": 5368709120.0, "available_bytes": 1288490189.0, "limit_bytes": 10737418240.0},
		},
		{
			name:       "insufficient_storage_when_full",
			err:        fmt.Errorf("reserve: %w", domain.InsufficientStorageError{RequestedBytes: 1024, AvailableBytes: 0, LimitBytes: 104857600}),
			wantStatus: http.StatusConflict,
			wantCode:   "insufficient_storage",
			wantTitle:  "Insufficient storage",
			wantDetail: "Requested 1.00 KiB but only 0 B is available (limit 100.00 MiB). Release a reservation or request less space.",
			wantFields: map[string]any{"requested_bytes": 1024.0, "available_bytes": 0.0, "limit_bytes": 104857600.0},
		},
		{
			name:       "idempotency_key_reused",
			err:        fmt.Errorf("reserve: %w", domain.IdempotencyKeyReusedError{OriginalBytes: 4194304}),
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "idempotency_key_reused",
			wantTitle:  "Idempotency key reused",
			wantDetail: "This Idempotency-Key was already used to reserve 4194304 bytes. Use a new key for a new request.",
			wantFields: map[string]any{"original_bytes": 4194304.0},
		},
		{
			name:           "limit_unavailable",
			err:            fmt.Errorf("reserve: %w", domain.ErrLimitUnavailable),
			wantStatus:     http.StatusServiceUnavailable,
			wantCode:       "limit_unavailable",
			wantTitle:      "Limit unavailable",
			wantDetail:     "Reservation limits are temporarily unavailable. Retry later; releases are not affected.",
			wantRetryAfter: "5",
		},
		{
			name:           "service_unavailable",
			err:            fmt.Errorf("%w: begin reserve: database is locked", domain.ErrServiceUnavailable),
			wantStatus:     http.StatusServiceUnavailable,
			wantCode:       "service_unavailable",
			wantTitle:      "Service unavailable",
			wantDetail:     "The service is temporarily unavailable. Retry with the same Idempotency-Key.",
			wantRetryAfter: "1",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var logOutput bytes.Buffer
			handlers := &apiHandlers{logger: slog.New(slog.NewJSONHandler(&logOutput, nil))}
			recorder := httptest.NewRecorder()

			handlers.writeError(recorder, newRequestWithPathValues("c0a8f3e2b71d4e5f"), testCase.err)

			wantLogged := testCase.wantStatus >= http.StatusInternalServerError
			if logged := logOutput.Len() > 0; logged != wantLogged {
				t.Errorf("error logged: got %t, want %t (log %q)", logged, wantLogged, logOutput.String())
			}
			if recorder.Code != testCase.wantStatus {
				t.Errorf("status: got %d, want %d", recorder.Code, testCase.wantStatus)
			}
			if contentType := recorder.Header().Get("Content-Type"); contentType != problemContentType {
				t.Errorf("Content-Type: got %q, want %q", contentType, problemContentType)
			}
			if retryAfter := recorder.Header().Get("Retry-After"); retryAfter != testCase.wantRetryAfter {
				t.Errorf("Retry-After: got %q, want %q", retryAfter, testCase.wantRetryAfter)
			}
			var body map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body %q: %v", recorder.Body.String(), err)
			}
			wantBody := map[string]any{
				"type":       "https://github.com/guillaumebarbas/reserve-file/blob/main/docs/api.md#" + testCase.wantCode,
				"title":      testCase.wantTitle,
				"status":     float64(testCase.wantStatus),
				"code":       testCase.wantCode,
				"detail":     testCase.wantDetail,
				"request_id": "c0a8f3e2b71d4e5f",
			}
			maps.Copy(wantBody, testCase.wantFields)
			if !maps.Equal(body, wantBody) {
				t.Errorf("body:\n got  %v\n want %v", body, wantBody)
			}
		})
	}
}

func newRequestWithPathValues(requestID string) *http.Request {
	request := httptest.NewRequestWithContext(contextWithRequestID(requestID), http.MethodGet, "/accounts/unknown_account/reservations/unknown_reservation", nil)
	request.SetPathValue("accountID", "unknown_account")
	request.SetPathValue("reservationID", "unknown_reservation")
	return request
}

func TestWriteError_UnknownErrorHidesDetails(t *testing.T) {
	var logOutput bytes.Buffer
	handlers := &apiHandlers{logger: slog.New(slog.NewJSONHandler(&logOutput, nil))}
	recorder := httptest.NewRecorder()
	internalError := errors.New("scan reservation: sql: expected 6 destination arguments, not 5")

	handlers.writeError(recorder, newRequestWithPathValues("c0a8f3e2b71d4e5f"), internalError)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if strings.Contains(recorder.Body.String(), "destination arguments") {
		t.Errorf("body %q contains the internal error", recorder.Body.String())
	}
	var body struct {
		Code      string `json:"code"`
		Detail    string `json:"detail"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", recorder.Body.String(), err)
	}
	wantDetail := "An unexpected error occurred. Reference: c0a8f3e2b71d4e5f."
	if body.Code != "internal_error" || body.Detail != wantDetail || body.RequestID != "c0a8f3e2b71d4e5f" {
		t.Errorf("body: got %+v, want internal_error, %q and the request ID", body, wantDetail)
	}
	var logRecord errorLogRecord
	if err := json.Unmarshal(logOutput.Bytes(), &logRecord); err != nil {
		t.Fatalf("log %q is not one JSON line: %v", logOutput.String(), err)
	}
	wantLogRecord := errorLogRecord{Level: "ERROR", Message: "request failed", RequestID: "c0a8f3e2b71d4e5f", Error: internalError.Error()}
	if logRecord != wantLogRecord {
		t.Errorf("log: got %+v, want %+v", logRecord, wantLogRecord)
	}
}

type errorLogRecord struct {
	Level     string `json:"level"`
	Message   string `json:"msg"`
	RequestID string `json:"request_id"`
	Error     string `json:"error"`
}
