//go:build e2e

package e2e

import (
	"fmt"
	"maps"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestContainer_RunsAsNonRootAndWritesVolume(t *testing.T) {
	startStack(t)
	containerID := apiContainerID(t)

	imageUser := mustRunDocker(t, "inspect", "--format", "{{.Config.User}}", containerID)
	processes := mustRunCompose(t, "top", "api")
	created := reserve(t, "non-root-key", 30*mebibyte)
	usage := readUsage(t)

	if imageUser != "nonroot:nonroot" {
		t.Errorf("image user: got %q, want %q", imageUser, "nonroot:nonroot")
	}
	if !strings.Contains(processes, "65532") {
		t.Errorf("docker compose top api: want the process to run as UID 65532, got\n%s", processes)
	}
	if created.status != http.StatusCreated {
		t.Errorf("reserve: got %d %s, want 201", created.status, created.body)
	}
	if usage.ReservedBytes != 30*mebibyte || usage.ActiveReservations != 1 {
		t.Errorf("usage after the reservation: got %+v, want 30 MiB and 1 active reservation", usage)
	}
}

func TestContainer_ReserveRetryRelease(t *testing.T) {
	startStack(t)
	waitForKnownLimit(t)

	created := reserve(t, "flow-key", 30*mebibyte)
	replayed := reserve(t, "flow-key", 30*mebibyte)
	reservation := decodeReservation(t, created)
	reservationPath := accountPath + "/reservations/" + reservation.ID
	read := mustSendRequest(t, http.MethodGet, reservationPath, "", "")
	released := mustSendRequest(t, http.MethodPost, reservationPath+"/release", "", "")
	releasedAgain := mustSendRequest(t, http.MethodPost, reservationPath+"/release", "", "")
	usage := readUsage(t)

	if created.status != http.StatusCreated || created.headers.Get("Location") != reservationPath {
		t.Errorf("reserve: got %d with Location %q, want 201 with %q", created.status, created.headers.Get("Location"), reservationPath)
	}
	if replayed.status != http.StatusOK || replayed.headers.Get("Idempotent-Replayed") != "true" || decodeReservation(t, replayed).ID != reservation.ID {
		t.Errorf("retry with the same key: got %d %s, want 200, Idempotent-Replayed: true and reservation %s", replayed.status, replayed.body, reservation.ID)
	}
	if read.status != http.StatusOK || decodeReservation(t, read).Status != "active" {
		t.Errorf("read: got %d %s, want 200 and an active reservation", read.status, read.body)
	}
	firstRelease := decodeReservation(t, released)
	secondRelease := decodeReservation(t, releasedAgain)
	if released.status != http.StatusOK || firstRelease.Status != "released" || firstRelease.ReleasedAt == nil {
		t.Fatalf("release: got %d %s, want 200 and a released reservation with released_at", released.status, released.body)
	}
	if releasedAgain.status != http.StatusOK || secondRelease.ReleasedAt == nil || *secondRelease.ReleasedAt != *firstRelease.ReleasedAt {
		t.Errorf("second release: got %d %s, want 200 with released_at %s", releasedAgain.status, releasedAgain.body, *firstRelease.ReleasedAt)
	}
	if usage.ReservedBytes != 0 || usage.ActiveReservations != 0 {
		t.Errorf("usage after the release: got %+v, want nothing reserved", usage)
	}
}

func TestContainer_ConcurrentBurstRespectsLimit(t *testing.T) {
	startStack(t)
	waitForKnownLimit(t)
	const requestCount = 20
	start := make(chan struct{})
	var requests sync.WaitGroup
	statuses := make([]int, requestCount)
	requestErrors := make([]error, requestCount)

	for index := range requestCount {
		requests.Go(func() {
			<-start
			response, err := sendRequest(http.MethodPost, accountPath+"/reservations", fmt.Sprintf("burst-key-%d", index), reserveRequestBody(30*mebibyte))
			statuses[index], requestErrors[index] = response.status, err
		})
	}
	close(start)
	requests.Wait()
	usage := readUsage(t)

	outcomes := map[int]int{}
	for index, status := range statuses {
		if requestErrors[index] != nil {
			t.Errorf("request %d: %v", index, requestErrors[index])
			continue
		}
		outcomes[status]++
	}
	wantOutcomes := map[int]int{http.StatusCreated: 3, http.StatusConflict: 17}
	if !maps.Equal(outcomes, wantOutcomes) {
		t.Errorf("20 concurrent reservations of 30 MiB with 100 MiB free: got %v, want %v", outcomes, wantOutcomes)
	}
	if usage.ReservedBytes != 3*30*mebibyte || usage.ActiveReservations != 3 {
		t.Errorf("usage after the burst: got %+v, want 90 MiB and 3 active reservations", usage)
	}
}

func TestContainer_RestartKeepsReservations(t *testing.T) {
	startStack(t)
	waitForKnownLimit(t)
	created := reserve(t, "restart-key", 30*mebibyte)

	mustRunCompose(t, "restart", "api")
	waitForReadiness(t)
	usage := readUsage(t)
	replayed := reserve(t, "restart-key", 30*mebibyte)

	if created.status != http.StatusCreated {
		t.Fatalf("reserve before the restart: got %d %s, want 201", created.status, created.body)
	}
	if usage.ReservedBytes != 30*mebibyte || usage.ActiveReservations != 1 {
		t.Errorf("usage after the restart: got %+v, want 30 MiB and 1 active reservation", usage)
	}
	if replayed.status != http.StatusOK || decodeReservation(t, replayed).ID != decodeReservation(t, created).ID {
		t.Errorf("retry after the restart: got %d %s, want 200 and the reservation made before it", replayed.status, replayed.body)
	}
}

func TestContainer_SigkillKeepsCommittedReservations(t *testing.T) {
	startStack(t)
	waitForKnownLimit(t)
	created := reserve(t, "sigkill-key", 30*mebibyte)

	mustRunCompose(t, "kill", "--signal", "SIGKILL", "api")
	mustRunCompose(t, "start", "api")
	waitForReadiness(t)
	usage := readUsage(t)
	replayed := reserve(t, "sigkill-key", 30*mebibyte)

	if created.status != http.StatusCreated {
		t.Fatalf("reserve before the kill: got %d %s, want 201", created.status, created.body)
	}
	if usage.ReservedBytes != 30*mebibyte || usage.ActiveReservations != 1 {
		t.Errorf("usage after SIGKILL: got %+v, want 30 MiB and 1 active reservation", usage)
	}
	if replayed.status != http.StatusOK || decodeReservation(t, replayed).ID != decodeReservation(t, created).ID {
		t.Errorf("retry after SIGKILL: got %d %s, want 200 and the reservation committed before it", replayed.status, replayed.body)
	}
}

func TestContainer_SigtermStopsGracefully(t *testing.T) {
	startStack(t)
	containerID := apiContainerID(t)

	stopStartedAt := time.Now()
	mustRunCompose(t, "stop", "api")
	stopDuration := time.Since(stopStartedAt)
	exitCode := mustRunDocker(t, "inspect", "--format", "{{.State.ExitCode}}", containerID)
	logLines := strings.Split(strings.TrimSpace(mustRunCompose(t, "logs", "--no-log-prefix", "api")), "\n")

	if exitCode != "0" {
		t.Errorf("exit code after SIGTERM: got %s, want 0", exitCode)
	}
	if stopDuration >= 10*time.Second {
		t.Errorf("docker compose stop: took %s, want less than 10 s", stopDuration)
	}
	if lastLine := logLines[len(logLines)-1]; !strings.Contains(lastLine, `"msg":"server stopped"`) {
		t.Errorf("last log line: got %s, want the server stopped message", lastLine)
	}
}

func TestContainer_LimitSourceDownKeepsLastValue(t *testing.T) {
	startStack(t)
	waitForKnownLimit(t)

	mustRunCompose(t, "stop", "env-stub")
	mustRunCompose(t, "restart", "api")
	readiness := waitForReadiness(t)
	created := reserve(t, "source-down-key", 30*mebibyte)

	if readiness.Limits != "ok" {
		t.Errorf("readiness after a restart with the limit source down: got limits %q, want %q, reloaded from the database", readiness.Limits, "ok")
	}
	if created.status != http.StatusCreated {
		t.Errorf("reserve with the limit source down: got %d %s, want 201", created.status, created.body)
	}
}

func TestContainer_FirstStartWithoutLimitSource(t *testing.T) {
	startStack(t, "--no-deps", "api")

	readiness := waitForReadiness(t)
	refused := reserve(t, "first-start-key", 30*mebibyte)
	usage := readUsage(t)

	if readiness.Limits != "unavailable" {
		t.Errorf("readiness on a first start without the limit source: got limits %q, want %q", readiness.Limits, "unavailable")
	}
	if refused.status != http.StatusServiceUnavailable || decodeProblem(t, refused).Code != "limit_unavailable" || refused.headers.Get("Retry-After") != "5" {
		t.Errorf("reserve without a known limit: got %d %s with Retry-After %q, want 503 limit_unavailable with Retry-After 5", refused.status, refused.body, refused.headers.Get("Retry-After"))
	}
	if usage.MaxActiveReservations != nil || usage.ReservedBytes != 0 {
		t.Errorf("usage without a known limit: got %+v, want max_active_reservations null and nothing reserved", usage)
	}
}
