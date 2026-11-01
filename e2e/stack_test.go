//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const (
	projectName           = "reserve-e2e"
	serviceURL            = "http://127.0.0.1:18090"
	accountPath           = "/accounts/small_account"
	requestTimeout        = 10 * time.Second
	readinessTimeout      = 90 * time.Second
	readinessPollInterval = 250 * time.Millisecond
	mebibyte              = 1048576
)

type apiResponse struct {
	status  int
	headers http.Header
	body    []byte
}

type reservationBody struct {
	ID         string  `json:"id"`
	Bytes      int64   `json:"bytes"`
	Status     string  `json:"status"`
	ReleasedAt *string `json:"released_at"`
}

type usageBody struct {
	ReservedBytes         int64 `json:"reserved_bytes"`
	ActiveReservations    int   `json:"active_reservations"`
	MaxActiveReservations *int  `json:"max_active_reservations"`
}

type readinessBody struct {
	Database string `json:"database"`
	Limits   string `json:"limits"`
}

type problemBody struct {
	Code string `json:"code"`
}

func startStack(t *testing.T, upArguments ...string) {
	t.Helper()
	t.Cleanup(func() {
		if output, err := runCompose("down", "--volumes", "--rmi", "local"); err != nil {
			t.Errorf("docker compose down: %v\n%s", err, output)
		}
	})
	mustRunCompose(t, append([]string{"up", "--detach", "--build"}, upArguments...)...)
	waitForReadiness(t)
}

func runCompose(arguments ...string) (string, error) {
	composeArguments := []string{"compose", "--project-name", projectName, "--file", "../compose.yaml", "--file", "../compose.e2e.yaml"}
	output, err := exec.Command("docker", append(composeArguments, arguments...)...).CombinedOutput()
	return string(output), err
}

func mustRunCompose(t *testing.T, arguments ...string) string {
	t.Helper()
	output, err := runCompose(arguments...)
	if err != nil {
		t.Fatalf("docker compose %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return output
}

func mustRunDocker(t *testing.T, arguments ...string) string {
	t.Helper()
	output, err := exec.Command("docker", arguments...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func apiContainerID(t *testing.T) string {
	t.Helper()
	return strings.TrimSpace(mustRunCompose(t, "ps", "--all", "--quiet", "api"))
}

func waitForReadiness(t *testing.T) readinessBody {
	t.Helper()
	return pollReadiness(t, "a 200 from /readyz", func(readiness readinessBody) bool { return true })
}

func waitForKnownLimit(t *testing.T) {
	t.Helper()
	pollReadiness(t, `"limits":"ok" from /readyz`, func(readiness readinessBody) bool { return readiness.Limits == "ok" })
}

func pollReadiness(t *testing.T, description string, isWanted func(readinessBody) bool) readinessBody {
	t.Helper()
	deadline := time.Now().Add(readinessTimeout)
	for {
		readiness, ready := readReadiness()
		if ready && isWanted(readiness) {
			return readiness
		}
		if time.Now().After(deadline) {
			t.Fatalf("no %s within %s", description, readinessTimeout)
		}
		time.Sleep(readinessPollInterval)
	}
}

func readReadiness() (readinessBody, bool) {
	response, err := sendRequest(http.MethodGet, "/readyz", "", "")
	if err != nil || response.status != http.StatusOK {
		return readinessBody{}, false
	}
	var readiness readinessBody
	if err := json.Unmarshal(response.body, &readiness); err != nil {
		return readinessBody{}, false
	}
	return readiness, true
}

func sendRequest(method, path, idempotencyKey, body string) (apiResponse, error) {
	request, err := http.NewRequest(method, serviceURL+path, strings.NewReader(body))
	if err != nil {
		return apiResponse{}, err
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: requestTimeout}
	response, err := client.Do(request)
	if err != nil {
		return apiResponse{}, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return apiResponse{}, err
	}
	return apiResponse{status: response.StatusCode, headers: response.Header, body: responseBody}, nil
}

func mustSendRequest(t *testing.T, method, path, idempotencyKey, body string) apiResponse {
	t.Helper()
	response, err := sendRequest(method, path, idempotencyKey, body)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return response
}

func reserve(t *testing.T, idempotencyKey string, bytes int64) apiResponse {
	t.Helper()
	return mustSendRequest(t, http.MethodPost, accountPath+"/reservations", idempotencyKey, reserveRequestBody(bytes))
}

func reserveRequestBody(bytes int64) string {
	return fmt.Sprintf(`{"bytes": %d}`, bytes)
}

func readUsage(t *testing.T) usageBody {
	t.Helper()
	response := mustSendRequest(t, http.MethodGet, accountPath+"/usage", "", "")
	if response.status != http.StatusOK {
		t.Fatalf("GET %s/usage: got %d %s, want 200", accountPath, response.status, response.body)
	}
	var usage usageBody
	if err := json.Unmarshal(response.body, &usage); err != nil {
		t.Fatalf("decode usage %q: %v", response.body, err)
	}
	return usage
}

func decodeReservation(t *testing.T, response apiResponse) reservationBody {
	t.Helper()
	var reservation reservationBody
	if err := json.Unmarshal(response.body, &reservation); err != nil {
		t.Fatalf("decode reservation %q: %v", response.body, err)
	}
	return reservation
}

func decodeProblem(t *testing.T, response apiResponse) problemBody {
	t.Helper()
	var problem problemBody
	if err := json.Unmarshal(response.body, &problem); err != nil {
		t.Fatalf("decode problem %q: %v", response.body, err)
	}
	return problem
}
