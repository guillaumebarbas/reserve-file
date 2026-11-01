package handler

import "net/http"

const (
	corsAllowedMethods = "GET, POST"
	corsAllowedHeaders = "Content-Type, Idempotency-Key"
	corsExposedHeaders = "Idempotent-Replayed, Location, Retry-After, X-Request-Id"
)

func WithCORS(allowedOrigin string, next http.Handler) http.Handler {
	if allowedOrigin == "" {
		return next
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Add("Vary", "Origin")
		if request.Header.Get("Origin") != allowedOrigin {
			next.ServeHTTP(writer, request)
			return
		}
		writer.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		isPreflight := request.Method == http.MethodOptions && request.Header.Get("Access-Control-Request-Method") != ""
		if isPreflight {
			writer.Header().Set("Access-Control-Allow-Methods", corsAllowedMethods)
			writer.Header().Set("Access-Control-Allow-Headers", corsAllowedHeaders)
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		writer.Header().Set("Access-Control-Expose-Headers", corsExposedHeaders)
		next.ServeHTTP(writer, request)
	})
}
