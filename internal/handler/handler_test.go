package handler

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guillaumebarbas/reserve-file/internal/repository/sqlite"
)

func TestHealthz(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()

	New(slog.New(slog.DiscardHandler), nil).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusOK)
	}
	contentType := recorder.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", contentType, "application/json")
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", recorder.Body.String(), err)
	}
	if body.Status != "ok" {
		t.Errorf("status field: got %q, want %q", body.Status, "ok")
	}
}

func TestAPI_Readyz(t *testing.T) {
	store, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "reserve.db"), nil)
	if err != nil {
		t.Fatalf("open the database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	var logOutput bytes.Buffer
	routes := New(slog.New(slog.NewJSONHandler(&logOutput, nil)), store)

	openStatus, openBody := getReadiness(routes)
	_ = store.Close()
	closedStatus, closedBody := getReadiness(routes)

	if openStatus != http.StatusOK || openBody != `{"database":"ok"}` {
		t.Errorf("GET /readyz with the database open: got %d %s, want 200 {\"database\":\"ok\"}", openStatus, openBody)
	}
	if closedStatus != http.StatusServiceUnavailable || closedBody != `{"database":"unavailable"}` {
		t.Errorf("GET /readyz with the database closed: got %d %s, want 503 {\"database\":\"unavailable\"}", closedStatus, closedBody)
	}
	if !strings.Contains(logOutput.String(), `"msg":"database not ready"`) {
		t.Errorf("log %q: want the readiness failure", logOutput.String())
	}
}

func getReadiness(routes http.Handler) (int, string) {
	recorder := httptest.NewRecorder()
	routes.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	return recorder.Code, strings.TrimSpace(recorder.Body.String())
}
