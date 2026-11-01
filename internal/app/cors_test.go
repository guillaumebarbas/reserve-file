package app

import (
	"net/http"
	"testing"
	"time"
)

func TestRun_AllowsTheConfiguredCORSOrigin(t *testing.T) {
	environment := map[string]string{
		"HTTP_ADDR":           "127.0.0.1:0",
		"DB_PATH":             newDatabasePath(t),
		"ENV_CONFIG_URL":      unreachableLimitSource(t),
		"CORS_ALLOWED_ORIGIN": "http://localhost:8081",
	}
	address, stop, result := startRunInBackground(t, environment)
	defer func() {
		stop()
		<-result
	}()
	preflight, err := http.NewRequest(http.MethodOptions, "http://"+address+"/accounts/large_account/reservations", nil)
	if err != nil {
		t.Fatalf("build the preflight request: %v", err)
	}
	preflight.Header.Set("Origin", "http://localhost:8081")
	preflight.Header.Set("Access-Control-Request-Method", http.MethodPost)
	client := &http.Client{Timeout: 5 * time.Second}

	response, err := client.Do(preflight)

	if err != nil {
		t.Fatalf("OPTIONS /accounts/large_account/reservations: %v", err)
	}
	_ = response.Body.Close()
	allowOrigin := response.Header.Get("Access-Control-Allow-Origin")
	if response.StatusCode != http.StatusNoContent || allowOrigin != "http://localhost:8081" {
		t.Errorf("preflight from http://localhost:8081: got %d with Access-Control-Allow-Origin %q, want 204 with http://localhost:8081", response.StatusCode, allowOrigin)
	}
}
