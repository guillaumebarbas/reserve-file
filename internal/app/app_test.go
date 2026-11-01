package app

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
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
	address, stop, result := startRunInBackground(t, map[string]string{"HTTP_ADDR": "127.0.0.1:0", "DB_PATH": newDatabasePath(t)})
	client := &http.Client{Timeout: 5 * time.Second}

	response, err := client.Get("http://" + address + "/healthz")
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
	occupiedListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer occupiedListener.Close()
	address := occupiedListener.Addr().String()
	ctx, cancel := context.WithTimeout(context.Background(), deadlineIfRunHangs)
	defer cancel()

	err = Run(ctx, nil, fakeEnvironment(map[string]string{"HTTP_ADDR": address, "DB_PATH": newDatabasePath(t)}), io.Discard)

	if err == nil {
		t.Fatalf("Run on %s, already in use: got nil error, want an error", address)
	}
	if !strings.Contains(err.Error(), address) {
		t.Errorf("Run on %s, already in use: error %q does not name the address", address, err)
	}
}

func TestRun_FailsOnInvalidConfiguration(t *testing.T) {
	environment := map[string]string{"HTTP_ADDR": "127.0.0.1:0", "LOG_LEVEL": "verbose"}
	ctx, cancel := context.WithTimeout(context.Background(), deadlineIfRunHangs)
	defer cancel()

	err := Run(ctx, nil, fakeEnvironment(environment), io.Discard)

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

	err := Run(ctx, nil, fakeEnvironment(environment), io.Discard)

	if err == nil {
		t.Fatalf("Run with DB_PATH=%s, a directory: got nil error, want an error", directory)
	}
	if !strings.Contains(err.Error(), "open database") {
		t.Errorf("Run with DB_PATH=%s, a directory: error %q does not name the database", directory, err)
	}
}

func TestHealthcheck_HealthyAndUnhealthy(t *testing.T) {
	healthyServer := httptest.NewServer(handler.New(slog.New(slog.DiscardHandler), nil))
	defer healthyServer.Close()
	failingServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer failingServer.Close()
	noServerAddress := unusedAddress(t)

	healthyErr := runHealthCheckCommand(healthyServer.Listener.Addr().String())
	failingErr := runHealthCheckCommand(failingServer.Listener.Addr().String())
	noServerErr := runHealthCheckCommand(noServerAddress)

	if healthyErr != nil {
		t.Errorf("healthcheck against a healthy server: got error %v, want nil", healthyErr)
	}
	if failingErr == nil {
		t.Error("healthcheck against a server answering 503: got nil error, want an error")
	}
	if noServerErr == nil {
		t.Error("healthcheck against a closed port: got nil error, want an error")
	}
}

func TestHealthcheck_ProbesWhereTheServerListens(t *testing.T) {
	testCases := []struct {
		name       string
		listenHost string
		configHost string
	}{
		{name: "empty_host", listenHost: "127.0.0.1", configHost: ""},
		{name: "ipv4_wildcard", listenHost: "127.0.0.1", configHost: "0.0.0.0"},
		{name: "ipv6_wildcard", listenHost: "127.0.0.1", configHost: "::"},
		{name: "specific_host", listenHost: "127.0.0.2", configHost: "127.0.0.2"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			port := startServerOnHost(t, testCase.listenHost)
			httpAddress := net.JoinHostPort(testCase.configHost, port)

			err := runHealthCheckCommand(httpAddress)

			if err != nil {
				t.Errorf("healthcheck with HTTP_ADDR=%s, server on %s: got error %v, want nil", httpAddress, testCase.listenHost, err)
			}
		})
	}
}

func startRunInBackground(t *testing.T, environment map[string]string) (string, context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	logReader, logWriter := io.Pipe()
	result := make(chan error, 1)
	go func() {
		err := Run(ctx, nil, fakeEnvironment(environment), logWriter)
		_ = logWriter.Close()
		result <- err
	}()

	logLines := bufio.NewScanner(logReader)
	for logLines.Scan() {
		var logRecord struct {
			Message string `json:"msg"`
			Address string `json:"address"`
		}
		if err := json.Unmarshal(logLines.Bytes(), &logRecord); err != nil {
			t.Fatalf("log line %q is not JSON: %v", logLines.Text(), err)
		}
		if logRecord.Message == "server started" {
			go io.Copy(io.Discard, logReader)
			return logRecord.Address, cancel, result
		}
	}
	t.Fatalf("Run stopped before logging %q: %v", "server started", <-result)
	return "", nil, nil
}

func runHealthCheckCommand(httpAddress string) error {
	ctx, cancel := context.WithTimeout(context.Background(), deadlineIfRunHangs)
	defer cancel()
	environment := map[string]string{"HTTP_ADDR": httpAddress}
	return Run(ctx, []string{"healthcheck"}, fakeEnvironment(environment), io.Discard)
}

func startServerOnHost(t *testing.T, host string) string {
	t.Helper()
	listener, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		t.Skipf("cannot listen on %s on this machine: %v", host, err)
	}
	server := httptest.NewUnstartedServer(handler.New(slog.New(slog.DiscardHandler), nil))
	_ = server.Listener.Close()
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	return port
}

func newDatabasePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "reserve.db")
}

func unusedAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	return address
}
