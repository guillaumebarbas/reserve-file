package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/repository/sqlite"
)

func TestRun_DrainsInFlightRequestsOnShutdown(t *testing.T) {
	limitSource, refresherFetching := startLimitSourceThatHangsAfterItsFirstAnswer(t)
	databasePath := newDatabasePath(t)
	address, stop, result := startRunInBackground(t, map[string]string{
		"HTTP_ADDR":          "127.0.0.1:0",
		"DB_PATH":            databasePath,
		"ENV_CONFIG_URL":     limitSource.URL,
		"ENV_CONFIG_REFRESH": "1ms",
		"ENV_CONFIG_TIMEOUT": "60s",
	})
	waitForSignal(t, refresherFetching, "the limits refresher to be in the middle of a fetch")
	idleConnection := openIdleConnection(t, address)
	inFlight := startReservationWithoutItsBody(t, address, `{"bytes": 31457280}`)

	stop()
	waitUntilClosedByServer(t, idleConnection)
	if connection, err := net.DialTimeout("tcp", address, deadlineIfRunHangs); err == nil {
		_ = connection.Close()
		t.Errorf("a new connection during the shutdown was accepted, want it refused")
	}
	select {
	case runErr := <-result:
		t.Fatalf("Run returned %v while a request was still in flight, want it to wait", runErr)
	default:
	}
	response := inFlight.finish(t)
	runErr := waitForRunResult(t, result)

	if response.StatusCode != http.StatusCreated {
		t.Errorf("in-flight reservation: got status %d, want %d", response.StatusCode, http.StatusCreated)
	}
	var reservation struct {
		Bytes  int64  `json:"bytes"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&reservation); err != nil {
		t.Fatalf("decode the in-flight reservation: %v", err)
	}
	if reservation.Bytes != 31457280 || reservation.Status != "active" {
		t.Errorf("in-flight reservation: got %+v, want 31457280 bytes, active", reservation)
	}
	if runErr != nil {
		t.Errorf("Run after the shutdown: got error %v, want nil", runErr)
	}
	if reservedBytes := readReservedBytes(t, databasePath, "small_account"); reservedBytes != 31457280 {
		t.Errorf("small_account after the shutdown: got %d bytes reserved, want 31457280", reservedBytes)
	}
}

func startLimitSourceThatHangsAfterItsFirstAnswer(t *testing.T) (*httptest.Server, <-chan struct{}) {
	t.Helper()
	refresherFetching := make(chan struct{})
	var answeredOnce atomic.Bool
	var signalOnce sync.Once
	limitSource := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if answeredOnce.CompareAndSwap(false, true) {
			_, _ = writer.Write([]byte(`{"MAX_PARALLEL_CHUNKS": 5}`))
			return
		}
		signalOnce.Do(func() { close(refresherFetching) })
		<-request.Context().Done()
	}))
	t.Cleanup(limitSource.Close)
	return limitSource, refresherFetching
}

func waitForSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(deadlineIfRunHangs):
		t.Fatalf("waited %s for %s", deadlineIfRunHangs, description)
	}
}

func openIdleConnection(t *testing.T, address string) net.Conn {
	t.Helper()
	connection, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatalf("connect to %s: %v", address, err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	if _, err := fmt.Fprintf(connection, "GET /healthz HTTP/1.1\r\nHost: %s\r\n\r\n", address); err != nil {
		t.Fatalf("send GET /healthz: %v", err)
	}
	response, err := http.ReadResponse(bufio.NewReader(connection), nil)
	if err != nil {
		t.Fatalf("read the /healthz response: %v", err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	return connection
}

func waitUntilClosedByServer(t *testing.T, connection net.Conn) {
	t.Helper()
	_ = connection.SetReadDeadline(time.Now().Add(deadlineIfRunHangs))
	if _, err := connection.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("idle connection: got %v, want the server to close it when the shutdown starts", err)
	}
}

type inFlightRequest struct {
	connection net.Conn
	reader     *bufio.Reader
	body       string
}

func startReservationWithoutItsBody(t *testing.T, address, body string) inFlightRequest {
	t.Helper()
	connection, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatalf("connect to %s: %v", address, err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	_, err = fmt.Fprintf(connection, "POST /accounts/small_account/reservations HTTP/1.1\r\nHost: %s\r\nIdempotency-Key: in-flight-request\r\nContent-Type: application/json\r\nContent-Length: %d\r\nExpect: 100-continue\r\n\r\n", address, len(body))
	if err != nil {
		t.Fatalf("send the request headers: %v", err)
	}
	reader := bufio.NewReader(connection)
	interimResponse, err := http.ReadResponse(reader, nil)
	if err != nil || interimResponse.StatusCode != http.StatusContinue {
		t.Fatalf("wait for the handler to read the body: got %v and error %v, want 100 Continue", interimResponse, err)
	}
	return inFlightRequest{connection: connection, reader: reader, body: body}
}

func (request inFlightRequest) finish(t *testing.T) *http.Response {
	t.Helper()
	if _, err := io.WriteString(request.connection, request.body); err != nil {
		t.Fatalf("send the request body: %v", err)
	}
	response, err := http.ReadResponse(request.reader, nil)
	if err != nil {
		t.Fatalf("read the response of the in-flight request: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	return response
}

func waitForRunResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case runErr := <-result:
		return runErr
	case <-time.After(deadlineIfRunHangs):
		t.Fatalf("Run did not return within %s after the shutdown", deadlineIfRunHangs)
		return nil
	}
}

func readReservedBytes(t *testing.T, databasePath, accountID string) int64 {
	t.Helper()
	store, err := sqlite.Open(t.Context(), databasePath, nil)
	if err != nil {
		t.Fatalf("reopen the database: %v", err)
	}
	defer store.Close()
	account, err := store.Account(t.Context(), accountID)
	if err != nil {
		t.Fatalf("read %s: %v", accountID, err)
	}
	return account.ReservedBytes()
}
