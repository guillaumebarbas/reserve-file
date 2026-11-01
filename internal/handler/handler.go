package handler

import (
	"context"
	"log/slog"
	"net/http"
)

type DatabasePinger interface {
	Ping(ctx context.Context) error
}

type apiHandlers struct {
	logger   *slog.Logger
	database DatabasePinger
}

type livenessBody struct {
	Status string `json:"status"`
}

type readinessBody struct {
	Database string `json:"database"`
}

func New(logger *slog.Logger, database DatabasePinger) http.Handler {
	handlers := &apiHandlers{logger: logger, database: database}
	routes := http.NewServeMux()
	routes.HandleFunc("GET /healthz", handlers.reportLiveness)
	routes.HandleFunc("GET /readyz", handlers.reportReadiness)
	return withRequestID(withAccessLog(logger, withPanicRecovery(handlers, routes)))
}

func (handlers *apiHandlers) reportLiveness(writer http.ResponseWriter, request *http.Request) {
	handlers.writeJSON(request.Context(), writer, http.StatusOK, jsonContentType, livenessBody{Status: "ok"})
}

func (handlers *apiHandlers) reportReadiness(writer http.ResponseWriter, request *http.Request) {
	if err := handlers.database.Ping(request.Context()); err != nil {
		handlers.logger.Warn("database not ready", "request_id", requestIDFrom(request.Context()), "error", err)
		handlers.writeJSON(request.Context(), writer, http.StatusServiceUnavailable, jsonContentType, readinessBody{Database: "unavailable"})
		return
	}
	handlers.writeJSON(request.Context(), writer, http.StatusOK, jsonContentType, readinessBody{Database: "ok"})
}
