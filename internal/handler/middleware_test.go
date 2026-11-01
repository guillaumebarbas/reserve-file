package handler

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestID_SetOnEveryResponse(t *testing.T) {
	routes := New(slog.New(slog.DiscardHandler), nil, nil, nil)
	paths := []string{"/healthz", "/healthz", "/unknown"}
	seenRequestIDs := map[string]bool{}

	for _, path := range paths {
		recorder := httptest.NewRecorder()
		routes.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))

		requestID := recorder.Header().Get("X-Request-Id")
		if _, err := hex.DecodeString(requestID); err != nil || len(requestID) != 16 {
			t.Errorf("GET %s: X-Request-Id %q, want 16 hexadecimal characters", path, requestID)
		}
		if seenRequestIDs[requestID] {
			t.Errorf("GET %s: X-Request-Id %q was already given to another request", path, requestID)
		}
		seenRequestIDs[requestID] = true
	}
}

type accessLogRecord struct {
	Message   string `json:"msg"`
	RequestID string `json:"request_id"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Status    int    `json:"status"`
	Duration  string `json:"duration"`
}

func TestAccessLog_LogsEveryRequest(t *testing.T) {
	var logOutput bytes.Buffer
	routes := New(slog.New(slog.NewJSONHandler(&logOutput, nil)), nil, nil, nil)
	recorder := httptest.NewRecorder()

	routes.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/unknown", nil))

	var logRecord accessLogRecord
	if err := json.Unmarshal(logOutput.Bytes(), &logRecord); err != nil {
		t.Fatalf("access log %q is not one JSON line: %v", logOutput.String(), err)
	}
	wantRecord := accessLogRecord{
		Message:   "request handled",
		RequestID: recorder.Header().Get("X-Request-Id"),
		Method:    http.MethodGet,
		Path:      "/unknown",
		Status:    http.StatusNotFound,
		Duration:  logRecord.Duration,
	}
	if logRecord != wantRecord {
		t.Errorf("access log: got %+v, want %+v", logRecord, wantRecord)
	}
	if logRecord.Duration == "" {
		t.Error("access log: no duration")
	}
}

func TestRecovery_Returns500AndKeepsServing(t *testing.T) {
	var logOutput bytes.Buffer
	handlers := &apiHandlers{logger: slog.New(slog.NewJSONHandler(&logOutput, nil))}
	routes := http.NewServeMux()
	routes.HandleFunc("GET /panic", func(writer http.ResponseWriter, request *http.Request) {
		panic("bug in a handler")
	})
	routes.HandleFunc("GET /healthz", handlers.reportLiveness)
	server := httptest.NewServer(withRequestID(withPanicRecovery(handlers, routes)))
	defer server.Close()
	client := server.Client()

	panicResponse, panicErr := client.Get(server.URL + "/panic")
	nextResponse, nextErr := client.Get(server.URL + "/healthz")

	if panicErr != nil {
		t.Fatalf("GET /panic: %v", panicErr)
	}
	defer panicResponse.Body.Close()
	if nextErr != nil {
		t.Fatalf("GET /healthz after the panic: %v", nextErr)
	}
	defer nextResponse.Body.Close()
	var body struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(panicResponse.Body).Decode(&body); err != nil {
		t.Fatalf("decode the /panic body: %v", err)
	}
	wantDetail := "An unexpected error occurred. Reference: " + panicResponse.Header.Get("X-Request-Id") + "."
	if panicResponse.StatusCode != http.StatusInternalServerError || body.Code != "internal_error" || body.Detail != wantDetail {
		t.Errorf("GET /panic: got %d %s %q, want 500 internal_error %q", panicResponse.StatusCode, body.Code, body.Detail, wantDetail)
	}
	if nextResponse.StatusCode != http.StatusOK {
		t.Errorf("GET /healthz after the panic: got %d, want %d", nextResponse.StatusCode, http.StatusOK)
	}
	if !strings.Contains(logOutput.String(), "panic: bug in a handler") || !strings.Contains(logOutput.String(), "goroutine ") {
		t.Errorf("log %q: want the panic value and its stack trace", logOutput.String())
	}
}
