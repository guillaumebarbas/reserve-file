package app

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRun_AppliesALoweredLimitAtRestart(t *testing.T) {
	limitSource := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"MAX_PARALLEL_CHUNKS": 5}`))
	}))
	defer limitSource.Close()
	environment := map[string]string{"HTTP_ADDR": "127.0.0.1:0", "DB_PATH": newDatabasePath(t), "ENV_CONFIG_URL": limitSource.URL}
	client := &http.Client{Timeout: 5 * time.Second}
	address, stop, result := startRunInBackground(t, environment)
	for reservationNumber := 1; reservationNumber <= 3; reservationNumber++ {
		if status := postReservation(t, client, address, fmt.Sprintf("key_%d", reservationNumber), 31457280); status != http.StatusCreated {
			t.Fatalf("reserve 30 MiB before the restart: got %d, want %d", status, http.StatusCreated)
		}
	}
	stop()
	if err := <-result; err != nil {
		t.Fatalf("Run before the restart: %v", err)
	}
	loweredEnvironment := maps.Clone(environment)
	loweredEnvironment["SEED_ACCOUNTS"] = "large_account:10737418240,small_account:52428800"

	address, stop, result = startRunInBackground(t, loweredEnvironment)
	defer func() {
		stop()
		<-result
	}()
	usage := getUsage(t, client, address)
	refusalStatus := postReservation(t, client, address, "key_after_restart", 1048576)

	wantUsage := usageAfterRestart{LimitBytes: 52428800, ReservedBytes: 94371840, AvailableBytes: 0, ActiveReservations: 3}
	if usage != wantUsage {
		t.Errorf("usage after a restart with SEED_ACCOUNTS lowering small_account to 50 MiB: got %+v, want %+v", usage, wantUsage)
	}
	if refusalStatus != http.StatusConflict {
		t.Errorf("reserve 1 MiB above the lowered limit: got %d, want %d", refusalStatus, http.StatusConflict)
	}
}

type usageAfterRestart struct {
	LimitBytes         int64 `json:"limit_bytes"`
	ReservedBytes      int64 `json:"reserved_bytes"`
	AvailableBytes     int64 `json:"available_bytes"`
	ActiveReservations int   `json:"active_reservations"`
}

func postReservation(t *testing.T, client *http.Client, address, idempotencyKey string, bytes int64) int {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, "http://"+address+"/accounts/small_account/reservations", strings.NewReader(fmt.Sprintf(`{"bytes": %d}`, bytes)))
	if err != nil {
		t.Fatalf("build the reservation request: %v", err)
	}
	request.Header.Set("Idempotency-Key", idempotencyKey)
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("POST a reservation: %v", err)
	}
	_ = response.Body.Close()
	return response.StatusCode
}

func getUsage(t *testing.T, client *http.Client, address string) usageAfterRestart {
	t.Helper()
	response, err := client.Get("http://" + address + "/accounts/small_account/usage")
	if err != nil {
		t.Fatalf("GET the usage: %v", err)
	}
	defer response.Body.Close()
	var usage usageAfterRestart
	if err := json.NewDecoder(response.Body).Decode(&usage); err != nil {
		t.Fatalf("decode the usage: %v", err)
	}
	return usage
}
