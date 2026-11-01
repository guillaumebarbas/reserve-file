package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteJSON_ClientGone(t *testing.T) {
	var logOutput bytes.Buffer
	handlers := &apiHandlers{logger: slog.New(slog.NewJSONHandler(&logOutput, nil))}
	writer := &disconnectedClientWriter{header: http.Header{}}

	handlers.writeJSON(contextWithRequestID("a1b2c3d4e5f60718"), writer, http.StatusOK, jsonContentType, livenessBody{Status: "ok"})

	if writer.status != http.StatusOK || writer.contentTypeWhenStatusWritten != jsonContentType {
		t.Errorf("status %d with Content-Type %q, want %d with %q", writer.status, writer.contentTypeWhenStatusWritten, http.StatusOK, jsonContentType)
	}
	var logRecord struct {
		Message   string `json:"msg"`
		RequestID string `json:"request_id"`
		Error     string `json:"error"`
	}
	if err := json.Unmarshal(logOutput.Bytes(), &logRecord); err != nil {
		t.Fatalf("log %q is not one JSON line: %v", logOutput.String(), err)
	}
	if logRecord.Message != "response not written" || logRecord.RequestID != "a1b2c3d4e5f60718" || logRecord.Error != "connection reset by peer" {
		t.Errorf("log: got %+v, want the message %q, the request ID and the write error", logRecord, "response not written")
	}
}

type disconnectedClientWriter struct {
	header                       http.Header
	status                       int
	contentTypeWhenStatusWritten string
}

func (writer *disconnectedClientWriter) Header() http.Header {
	return writer.header
}

func (writer *disconnectedClientWriter) WriteHeader(status int) {
	writer.status = status
	writer.contentTypeWhenStatusWritten = writer.header.Get("Content-Type")
}

func (writer *disconnectedClientWriter) Write(body []byte) (int, error) {
	return 0, errors.New("connection reset by peer")
}

func contextWithRequestID(requestID string) context.Context {
	return context.WithValue(context.Background(), requestIDKey{}, requestID)
}

type bytesOnlyBody struct {
	Bytes *int64 `json:"bytes"`
}

func TestDecodeJSON_ValidBody(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/accounts/large_account/reservations", strings.NewReader(`{"bytes": 4194304}`))
	var destination bytesOnlyBody

	err := decodeJSON(httptest.NewRecorder(), request, &destination)

	if err != nil {
		t.Fatalf("decodeJSON: got error %v, want nil", err)
	}
	if destination.Bytes == nil || *destination.Bytes != 4194304 {
		t.Errorf("bytes: got %v, want 4194304", destination.Bytes)
	}
}

func TestDecodeJSON_RejectsInvalidBodies(t *testing.T) {
	testCases := []struct {
		name       string
		body       string
		wantCode   string
		wantDetail string
	}{
		{"malformed_json", `{"bytes": `, "invalid_request", "The request body must be a single JSON object, without unknown fields."},
		{"unknown_field", `{"bytes": 1, "size": 2}`, "invalid_request", "The request body must be a single JSON object, without unknown fields."},
		{"trailing_data", `{"bytes": 1} {"bytes": 2}`, "invalid_request", "The request body must be a single JSON object, without unknown fields."},
		{"empty_body", ``, "invalid_request", "The request body is empty. Send a JSON object."},
		{"body_over_4_kib", `{"bytes": 1` + strings.Repeat(" ", 4096) + `}`, "request_too_large", "The request body must not exceed 4096 bytes."},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/accounts/large_account/reservations", strings.NewReader(testCase.body))
			var destination bytesOnlyBody

			err := decodeJSON(httptest.NewRecorder(), request, &destination)

			var invalidInput invalidInputError
			if !errors.As(err, &invalidInput) {
				t.Fatalf("decodeJSON(%q): got error %v, want an invalid input error", testCase.body, err)
			}
			if invalidInput.problem.Code != testCase.wantCode || invalidInput.problem.Detail != testCase.wantDetail {
				t.Errorf("decodeJSON(%q): got %s %q, want %s %q", testCase.body, invalidInput.problem.Code, invalidInput.problem.Detail, testCase.wantCode, testCase.wantDetail)
			}
			if !strings.Contains(err.Error(), testCase.wantCode) {
				t.Errorf("decodeJSON(%q): error text %q does not name %s", testCase.body, err, testCase.wantCode)
			}
		})
	}
}
