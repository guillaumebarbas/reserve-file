package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/guillaumebarbas/reserve-file/internal/service"
)

type DatabasePinger interface {
	Ping(ctx context.Context) error
}

type LimitProvider interface {
	MaxActive() (int, bool)
}

type apiHandlers struct {
	logger        *slog.Logger
	database      DatabasePinger
	limitProvider LimitProvider
	storage       *service.Service
}

type livenessBody struct {
	Status string `json:"status"`
}

type readinessBody struct {
	Database string `json:"database"`
	Limits   string `json:"limits"`
}

func New(logger *slog.Logger, database DatabasePinger, limitProvider LimitProvider, storage *service.Service) http.Handler {
	handlers := &apiHandlers{logger: logger, database: database, limitProvider: limitProvider, storage: storage}
	routes := http.NewServeMux()
	routes.HandleFunc("GET /healthz", handlers.reportLiveness)
	routes.HandleFunc("GET /readyz", handlers.reportReadiness)
	routes.HandleFunc("GET /accounts/{accountID}/usage", handlers.reportUsage)
	routes.HandleFunc("POST /accounts/{accountID}/reservations", handlers.reserve)
	routes.HandleFunc("GET /accounts/{accountID}/reservations", handlers.listActiveReservations)
	routes.HandleFunc("GET /accounts/{accountID}/reservations/{reservationID}", handlers.readReservation)
	routes.HandleFunc("POST /accounts/{accountID}/reservations/{reservationID}/release", handlers.releaseReservation)
	return withRequestID(withAccessLog(logger, withPanicRecovery(handlers, routes)))
}

func (handlers *apiHandlers) reportLiveness(writer http.ResponseWriter, request *http.Request) {
	handlers.writeJSON(request.Context(), writer, http.StatusOK, jsonContentType, livenessBody{Status: "ok"})
}

func (handlers *apiHandlers) reportReadiness(writer http.ResponseWriter, request *http.Request) {
	readiness := readinessBody{Database: "ok", Limits: "ok"}
	if _, known := handlers.limitProvider.MaxActive(); !known {
		readiness.Limits = "unavailable"
	}
	if err := handlers.database.Ping(request.Context()); err != nil {
		handlers.logger.Warn("database not ready", "request_id", requestIDFrom(request.Context()), "error", err)
		readiness.Database = "unavailable"
		handlers.writeJSON(request.Context(), writer, http.StatusServiceUnavailable, jsonContentType, readiness)
		return
	}
	handlers.writeJSON(request.Context(), writer, http.StatusOK, jsonContentType, readiness)
}
