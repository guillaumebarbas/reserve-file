package handler

import (
	"bytes"
	"database/sql"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/repository/sqlite"
	"github.com/guillaumebarbas/reserve-file/internal/service"
)

func TestAPI_DatabaseClosedReturns503(t *testing.T) {
	var logOutput bytes.Buffer
	routes, store, _ := newRoutesOnDatabase(t, &logOutput)
	created := decodeObject(t, reserve(routes, "small_account", "key_1", 31457280))
	reservationID, _ := created["id"].(string)
	_ = store.Close()
	testCases := []struct {
		name           string
		method         string
		path           string
		idempotencyKey string
		body           string
	}{
		{"reserve", http.MethodPost, "/accounts/small_account/reservations", "key_2", `{"bytes": 1048576}`},
		{"release", http.MethodPost, "/accounts/small_account/reservations/" + reservationID + "/release", "", ""},
		{"read_reservation", http.MethodGet, "/accounts/small_account/reservations/" + reservationID, "", ""},
		{"list_active_reservations", http.MethodGet, "/accounts/small_account/reservations", "", ""},
		{"usage", http.MethodGet, "/accounts/small_account/usage", "", ""},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			logOutput.Reset()
			request := httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body))
			if testCase.idempotencyKey != "" {
				request.Header.Set("Idempotency-Key", testCase.idempotencyKey)
			}
			recorder := httptest.NewRecorder()

			routes.ServeHTTP(recorder, request)

			assertServiceUnavailable(t, recorder, "database is closed", logOutput.String())
		})
	}
}

func TestAPI_RetryAfterDatabaseRecovers(t *testing.T) {
	var logOutput bytes.Buffer
	routes, _, databasePath := newRoutesOnDatabase(t, &logOutput)
	otherConnection := openSecondConnection(t, databasePath)
	executeOn(t, otherConnection, "CREATE TRIGGER refuse_reservations BEFORE INSERT ON reservations BEGIN SELECT RAISE(ABORT, 'injected failure'); END")

	failed := reserve(routes, "small_account", "key_1", 31457280)

	assertServiceUnavailable(t, failed, "injected failure", logOutput.String())
	assertUsage(t, routes, "small_account", 0, 0)

	executeOn(t, otherConnection, "DROP TRIGGER refuse_reservations")
	created := reserve(routes, "small_account", "key_1", 31457280)
	replayed := reserve(routes, "small_account", "key_1", 31457280)

	if created.Code != http.StatusCreated || replayed.Code != http.StatusOK || replayed.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("retries once the database recovers: got %d then %d (Idempotent-Replayed %q), want 201 then 200 replayed", created.Code, replayed.Code, replayed.Header().Get("Idempotent-Replayed"))
	}
	if createdID, replayedID := decodeObject(t, created)["id"], decodeObject(t, replayed)["id"]; createdID != replayedID {
		t.Errorf("replayed reservation: got id %v, want %v", replayedID, createdID)
	}
	assertUsage(t, routes, "small_account", 31457280, 1)
}

func newRoutesOnDatabase(t *testing.T, logOutput *bytes.Buffer) (http.Handler, *sqlite.Store, string) {
	t.Helper()
	databasePath := filepath.Join(t.TempDir(), "reserve.db")
	store, err := sqlite.Open(t.Context(), databasePath, map[string]int64{"small_account": 104857600})
	if err != nil {
		t.Fatalf("open the database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	limitProvider := fakeLimitProvider{maxActive: 5, known: true}
	routes := New(slog.New(slog.NewJSONHandler(logOutput, nil)), store, limitProvider, service.New(store, limitProvider, time.Now))
	return routes, store, databasePath
}

func openSecondConnection(t *testing.T, databasePath string) *sql.DB {
	t.Helper()
	database, _ := sql.Open("sqlite", databasePath)
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func executeOn(t *testing.T, database *sql.DB, statement string) {
	t.Helper()
	if _, err := database.ExecContext(t.Context(), statement); err != nil {
		t.Fatalf("%s: %v", statement, err)
	}
}

func assertServiceUnavailable(t *testing.T, recorder *httptest.ResponseRecorder, cause string, logLines string) {
	t.Helper()
	if recorder.Code != http.StatusServiceUnavailable || recorder.Header().Get("Retry-After") != "1" {
		t.Errorf("got %d with Retry-After %q, want 503 with Retry-After 1", recorder.Code, recorder.Header().Get("Retry-After"))
	}
	if strings.Contains(recorder.Body.String(), cause) {
		t.Errorf("body %s shows the internal cause %q", recorder.Body.String(), cause)
	}
	body := decodeObject(t, recorder)
	requestID := recorder.Header().Get("X-Request-Id")
	if body["code"] != "service_unavailable" || body["request_id"] != requestID {
		t.Errorf("body: got code %v and request_id %v, want service_unavailable and %s", body["code"], body["request_id"], requestID)
	}
	if !strings.Contains(logLines, cause) || !strings.Contains(logLines, requestID) {
		t.Errorf("log %q: want the cause %q with the request ID %s", logLines, cause, requestID)
	}
}
