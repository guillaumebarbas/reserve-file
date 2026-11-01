package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRun_FirstStartDuringOutage(t *testing.T) {
	limitSource := startSwitchableLimitSource(t, false)
	address, stop, result := startRunInBackground(t, map[string]string{"HTTP_ADDR": "127.0.0.1:0", "DB_PATH": newDatabasePath(t), "ENV_CONFIG_URL": limitSource.url})
	client := &http.Client{Timeout: deadlineIfRunHangs}

	readinessStatus, readiness := getThroughRun(t, client, address, "/readyz")
	usageStatus, usage := getThroughRun(t, client, address, "/accounts/small_account/usage")
	refusal := reserveThroughRun(t, client, address, "key_1", 31457280)
	limitSource.startAnswering()
	waitForSignal(t, limitSource.answered, "the limit source to answer a retry of the refresher")
	waitUntilLimitKnown(t, client, address)
	acceptance := reserveThroughRun(t, client, address, "key_1", 31457280)
	stop()
	runErr := waitForRunResult(t, result)

	if readinessStatus != http.StatusOK || readiness["database"] != "ok" || readiness["limits"] != "unavailable" {
		t.Errorf("GET /readyz during the outage: got %d %v, want 200 with database ok and limits unavailable", readinessStatus, readiness)
	}
	if usageStatus != http.StatusOK || usage["max_active_reservations"] != nil {
		t.Errorf("GET /accounts/small_account/usage during the outage: got %d %v, want 200 with max_active_reservations null", usageStatus, usage)
	}
	if refusal.status != http.StatusServiceUnavailable || refusal.header.Get("Retry-After") != "5" || refusal.body["code"] != "limit_unavailable" {
		t.Errorf("reservation during the outage: got %d, Retry-After %q, %v; want 503, 5 and limit_unavailable", refusal.status, refusal.header.Get("Retry-After"), refusal.body)
	}
	if acceptance.status != http.StatusCreated {
		t.Errorf("reservation once the source answers, with the same key: got %d %v, want 201", acceptance.status, acceptance.body)
	}
	if runErr != nil {
		t.Errorf("Run after the shutdown: got error %v, want nil", runErr)
	}
}

func TestRun_KeepsReservingWhenSourceGoesDown(t *testing.T) {
	limitSource := startSwitchableLimitSource(t, true)
	address, stop, result := startRunInBackground(t, map[string]string{"HTTP_ADDR": "127.0.0.1:0", "DB_PATH": newDatabasePath(t), "ENV_CONFIG_URL": limitSource.url, "ENV_CONFIG_REFRESH": "10ms"})
	client := &http.Client{Timeout: deadlineIfRunHangs}

	beforeOutage := reserveThroughRun(t, client, address, "key_1", 31457280)
	limitSource.stopAnswering()
	waitForSignal(t, limitSource.failedRequests, "a first failed refresh")
	waitForSignal(t, limitSource.failedRequests, "a second failed refresh, which comes after the first one was handled")
	duringOutage := reserveThroughRun(t, client, address, "key_2", 31457280)
	_, readiness := getThroughRun(t, client, address, "/readyz")
	reservationID, _ := beforeOutage.body["id"].(string)
	releaseStatus, released := postThroughRun(t, client, address, "/accounts/small_account/reservations/"+reservationID+"/release")
	stop()
	runErr := waitForRunResult(t, result)

	if beforeOutage.status != http.StatusCreated {
		t.Errorf("reservation before the outage: got %d %v, want 201", beforeOutage.status, beforeOutage.body)
	}
	if duringOutage.status != http.StatusCreated {
		t.Errorf("reservation after two failed refreshes: got %d %v, want 201 with the last value still in use", duringOutage.status, duringOutage.body)
	}
	if readiness["limits"] != "ok" {
		t.Errorf("GET /readyz during the outage: got %v, want limits ok", readiness)
	}
	if releaseStatus != http.StatusOK || released["status"] != "released" {
		t.Errorf("release during the outage: got %d %v, want 200 and released", releaseStatus, released)
	}
	if runErr != nil {
		t.Errorf("Run after the shutdown: got error %v, want nil", runErr)
	}
}

func TestRun_RestartDuringOutageKeepsTheLastLimit(t *testing.T) {
	databasePath := newDatabasePath(t)
	limitSource := startSwitchableLimitSource(t, true)
	_, stopFirstRun, firstResult := startRunInBackground(t, map[string]string{"HTTP_ADDR": "127.0.0.1:0", "DB_PATH": databasePath, "ENV_CONFIG_URL": limitSource.url})
	stopFirstRun()
	firstRunErr := waitForRunResult(t, firstResult)

	address, stop, result := startRunInBackground(t, map[string]string{"HTTP_ADDR": "127.0.0.1:0", "DB_PATH": databasePath, "ENV_CONFIG_URL": unreachableLimitSource(t)})
	client := &http.Client{Timeout: deadlineIfRunHangs}
	reservation := reserveThroughRun(t, client, address, "key_1", 31457280)
	_, usage := getThroughRun(t, client, address, "/accounts/small_account/usage")
	stop()
	runErr := waitForRunResult(t, result)

	if firstRunErr != nil {
		t.Errorf("first Run with the source up: got error %v, want nil", firstRunErr)
	}
	if reservation.status != http.StatusCreated {
		t.Errorf("reservation after a restart during the outage: got %d %v, want 201 with the saved limit", reservation.status, reservation.body)
	}
	if usage["max_active_reservations"] != 5.0 {
		t.Errorf("usage after a restart during the outage: got %v, want max_active_reservations 5", usage)
	}
	if runErr != nil {
		t.Errorf("second Run after the shutdown: got error %v, want nil", runErr)
	}
}

type switchableLimitSource struct {
	url            string
	answering      *atomic.Bool
	answered       <-chan struct{}
	failedRequests <-chan struct{}
}

func startSwitchableLimitSource(t *testing.T, answering bool) switchableLimitSource {
	t.Helper()
	var answeringNow atomic.Bool
	answeringNow.Store(answering)
	answered := make(chan struct{})
	var answeredOnce sync.Once
	failedRequests := make(chan struct{}, 2)
	limitSource := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if answeringNow.Load() {
			_, _ = writer.Write([]byte(`{"MAX_PARALLEL_CHUNKS": 5}`))
			answeredOnce.Do(func() { close(answered) })
			return
		}
		writer.WriteHeader(http.StatusServiceUnavailable)
		select {
		case failedRequests <- struct{}{}:
		default:
		}
	}))
	t.Cleanup(limitSource.Close)
	return switchableLimitSource{url: limitSource.URL, answering: &answeringNow, answered: answered, failedRequests: failedRequests}
}

func (limitSource switchableLimitSource) startAnswering() {
	limitSource.answering.Store(true)
}

func (limitSource switchableLimitSource) stopAnswering() {
	limitSource.answering.Store(false)
}

type responseThroughRun struct {
	status int
	header http.Header
	body   map[string]any
}

func reserveThroughRun(t *testing.T, client *http.Client, address, idempotencyKey string, bytes int64) responseThroughRun {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, "http://"+address+"/accounts/small_account/reservations", strings.NewReader(fmt.Sprintf(`{"bytes": %d}`, bytes)))
	if err != nil {
		t.Fatalf("build the reservation request: %v", err)
	}
	request.Header.Set("Idempotency-Key", idempotencyKey)
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("POST /accounts/small_account/reservations: %v", err)
	}
	return responseThroughRun{status: response.StatusCode, header: response.Header, body: readJSONObject(t, response)}
}

func getThroughRun(t *testing.T, client *http.Client, address, path string) (int, map[string]any) {
	t.Helper()
	response, err := client.Get("http://" + address + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return response.StatusCode, readJSONObject(t, response)
}

func postThroughRun(t *testing.T, client *http.Client, address, path string) (int, map[string]any) {
	t.Helper()
	response, err := client.Post("http://"+address+path, "application/json", nil)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return response.StatusCode, readJSONObject(t, response)
}

func readJSONObject(t *testing.T, response *http.Response) map[string]any {
	t.Helper()
	defer response.Body.Close()
	var object map[string]any
	if err := json.NewDecoder(response.Body).Decode(&object); err != nil {
		t.Fatalf("decode the response body: %v", err)
	}
	return object
}

func waitUntilLimitKnown(t *testing.T, client *http.Client, address string) {
	t.Helper()
	deadline := time.Now().Add(deadlineIfRunHangs)
	for time.Now().Before(deadline) {
		if _, readiness := getThroughRun(t, client, address, "/readyz"); readiness["limits"] == "ok" {
			return
		}
	}
	t.Fatalf("the service did not report the limit as known within %s", deadlineIfRunHangs)
}
