// Package handler exposes the service over HTTP.
package handler

import "net/http"

// New returns the HTTP handler of the service, with every route registered.
func New() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	return mux
}

func healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	// A write error means the client has gone: there is nobody left to tell.
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
