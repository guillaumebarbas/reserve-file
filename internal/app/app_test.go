package app

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/handler"
)

const deadlineIfRunHangs = 5 * time.Second

func TestRun_ServesUntilContextCancelled(t *testing.T) {
	addr, stop, result := startRun(t, map[string]string{"HTTP_ADDR": "127.0.0.1:0", "DB_PATH": newDatabasePath(t)})
	client := &http.Client{Timeout: 5 * time.Second}

	response, err := client.Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	_ = response.Body.Close()
	stop()
	runErr := <-result

	if response.StatusCode != http.StatusOK {
		t.Errorf("GET /healthz status: got %d, want %d", response.StatusCode, http.StatusOK)
	}
	if runErr != nil {
		t.Errorf("Run after cancellation: got error %v, want nil", runErr)
	}
}

func TestRun_FailsWhenAddressInUse(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer occupied.Close()
	addr := occupied.Addr().String()
	ctx, cancel := context.WithTimeout(context.Background(), deadlineIfRunHangs)
	defer cancel()

	err = Run(ctx, nil, fakeGetenv(map[string]string{"HTTP_ADDR": addr, "DB_PATH": newDatabasePath(t)}), io.Discard)

	if err == nil {
		t.Fatalf("Run on %s, already in use: got nil error, want an error", addr)
	}
	if !strings.Contains(err.Error(), addr) {
		t.Errorf("Run on %s, already in use: error %q does not name the address", addr, err)
	}
}

func TestRun_FailsOnInvalidConfiguration(t *testing.T) {
	environment := map[string]string{"HTTP_ADDR": "127.0.0.1:0", "LOG_LEVEL": "verbose"}
	ctx, cancel := context.WithTimeout(context.Background(), deadlineIfRunHangs)
	defer cancel()

	err := Run(ctx, nil, fakeGetenv(environment), io.Discard)

	if err == nil {
		t.Fatalf("Run with %v: got nil error, want a configuration error", environment)
	}
	if !strings.Contains(err.Error(), "LOG_LEVEL") {
		t.Errorf("Run with %v: error %q does not name LOG_LEVEL", environment, err)
	}
}

func TestRun_FailsWhenDatabaseCannotOpen(t *testing.T) {
	directory := t.TempDir()
	environment := map[string]string{"HTTP_ADDR": "127.0.0.1:0", "DB_PATH": directory}
	ctx, cancel := context.WithTimeout(context.Background(), deadlineIfRunHangs)
	defer cancel()

	err := Run(ctx, nil, fakeGetenv(environment), io.Discard)

	if err == nil {
		t.Fatalf("Run with DB_PATH=%s, a directory: got nil error, want an error", directory)
	}
	if !strings.Contains(err.Error(), "open database") {
		t.Errorf("Run with DB_PATH=%s, a directory: error %q does not name the database", directory, err)
	}
}

func TestHealthcheck_HealthyAndUnhealthy(t *testing.T) {
	healthy := httptest.NewServer(handler.New())
	defer healthy.Close()
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer failing.Close()
	closed := closedAddress(t)

	healthyErr := runHealthcheck(healthy.Listener.Addr().String())
	failingErr := runHealthcheck(failing.Listener.Addr().String())
	closedErr := runHealthcheck(closed)

	if healthyErr != nil {
		t.Errorf("healthcheck against a healthy server: got error %v, want nil", healthyErr)
	}
	if failingErr == nil {
		t.Error("healthcheck against a server answering 503: got nil error, want an error")
	}
	if closedErr == nil {
		t.Error("healthcheck against a closed port: got nil error, want an error")
	}
}

func TestHealthcheck_ProbesWhereTheServerListens(t *testing.T) {
	tests := []struct {
		name       string
		listenHost string
		configHost string
	}{
		{name: "empty_host", listenHost: "127.0.0.1", configHost: ""},
		{name: "ipv4_wildcard", listenHost: "127.0.0.1", configHost: "0.0.0.0"},
		{name: "ipv6_wildcard", listenHost: "127.0.0.1", configHost: "::"},
		{name: "specific_host", listenHost: "127.0.0.2", configHost: "127.0.0.2"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			port := startServerOn(t, tc.listenHost)
			httpAddr := net.JoinHostPort(tc.configHost, port)

			err := runHealthcheck(httpAddr)

			if err != nil {
				t.Errorf("healthcheck with HTTP_ADDR=%s, server on %s: got error %v, want nil", httpAddr, tc.listenHost, err)
			}
		})
	}
}

// startRun runs Run in the background and waits for its "server started" log
// line. It returns the address read from that line, a function that cancels
// Run's context, and a channel that receives Run's result.
func startRun(t *testing.T, environment map[string]string) (string, context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	logReader, logWriter := io.Pipe()
	result := make(chan error, 1)
	go func() {
		err := Run(ctx, nil, fakeGetenv(environment), logWriter)
		_ = logWriter.Close()
		result <- err
	}()

	logLines := bufio.NewScanner(logReader)
	for logLines.Scan() {
		var record struct {
			Msg  string `json:"msg"`
			Addr string `json:"addr"`
		}
		if err := json.Unmarshal(logLines.Bytes(), &record); err != nil {
			t.Fatalf("log line %q is not JSON: %v", logLines.Text(), err)
		}
		if record.Msg == "server started" {
			// Keep reading, so that Run never blocks on its next log lines.
			go io.Copy(io.Discard, logReader)
			return record.Addr, cancel, result
		}
	}
	t.Fatalf("Run stopped before logging %q: %v", "server started", <-result)
	return "", nil, nil
}

func runHealthcheck(httpAddr string) error {
	ctx, cancel := context.WithTimeout(context.Background(), deadlineIfRunHangs)
	defer cancel()
	environment := map[string]string{"HTTP_ADDR": httpAddr}
	return Run(ctx, []string{"healthcheck"}, fakeGetenv(environment), io.Discard)
}

// startServerOn serves the API on host and returns the port it listens on. It
// skips the test when the machine cannot listen on host.
func startServerOn(t *testing.T, host string) string {
	t.Helper()
	listener, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		t.Skipf("cannot listen on %s on this machine: %v", host, err)
	}
	server := httptest.NewUnstartedServer(handler.New())
	_ = server.Listener.Close()
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	return port
}

func newDatabasePath(t *testing.T) string {
	return filepath.Join(t.TempDir(), "reserve.db")
}

func closedAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	return addr
}
