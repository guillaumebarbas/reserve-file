package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

type requestIDKey struct{}

type statusRecorder struct {
	writer http.ResponseWriter
	status int
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestID := newRequestID()
		writer.Header().Set("X-Request-Id", requestID)
		ctx := context.WithValue(request.Context(), requestIDKey{}, requestID)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

func withAccessLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		startedAt := time.Now()
		recorder := &statusRecorder{writer: writer, status: http.StatusOK}
		next.ServeHTTP(recorder, request)
		logger.Info("request handled",
			"request_id", requestIDFrom(request.Context()),
			"method", request.Method,
			"path", request.URL.Path,
			"status", recorder.status,
			"duration", time.Since(startedAt).String(),
		)
	})
}

func withPanicRecovery(handlers *apiHandlers, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				handlers.writeError(writer, request, fmt.Errorf("panic: %v\n%s", recovered, debug.Stack()))
			}
		}()
		next.ServeHTTP(writer, request)
	})
}

func newRequestID() string {
	randomBytes := make([]byte, 8)
	_, _ = rand.Read(randomBytes)
	return hex.EncodeToString(randomBytes)
}

func requestIDFrom(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDKey{}).(string)
	return requestID
}

func (recorder *statusRecorder) Header() http.Header {
	return recorder.writer.Header()
}

func (recorder *statusRecorder) WriteHeader(status int) {
	recorder.status = status
	recorder.writer.WriteHeader(status)
}

func (recorder *statusRecorder) Write(body []byte) (int, error) {
	return recorder.writer.Write(body)
}
