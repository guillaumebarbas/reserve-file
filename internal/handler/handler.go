package handler

import "net/http"

func New() http.Handler {
	routes := http.NewServeMux()
	routes.HandleFunc("GET /healthz", reportLiveness)
	return routes
}

func reportLiveness(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte(`{"status":"ok"}`))
}
