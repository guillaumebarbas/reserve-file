package handler

import (
	"log/slog"
	"maps"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/repository/sqlite"
	"github.com/guillaumebarbas/reserve-file/internal/service"
)

func TestAPI_ReserveWhenLimitUnknown(t *testing.T) {
	_, routesDuringOutage := newRoutesBeforeAndDuringOutage(t)

	recorder := reserve(routesDuringOutage, "small_account", "key_1", 31457280)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("POST /accounts/small_account/reservations with the limit unknown: got %d %s, want %d", recorder.Code, recorder.Body.String(), http.StatusServiceUnavailable)
	}
	if retryAfter := recorder.Header().Get("Retry-After"); retryAfter != "5" {
		t.Errorf("Retry-After: got %q, want %q", retryAfter, "5")
	}
	body := decodeObject(t, recorder)
	wantDetail := "Reservation limits are temporarily unavailable. Retry later; releases are not affected."
	if body["code"] != "limit_unavailable" || body["detail"] != wantDetail {
		t.Errorf("got code %v and detail %q, want limit_unavailable and %q", body["code"], body["detail"], wantDetail)
	}
	assertUsage(t, routesDuringOutage, "small_account", 0, 0)
}

func TestAPI_ReleaseWhenLimitUnknown(t *testing.T) {
	routesBeforeOutage, routesDuringOutage := newRoutesBeforeAndDuringOutage(t)
	created := decodeObject(t, reserve(routesBeforeOutage, "small_account", "key_1", 31457280))
	reservationID, _ := created["id"].(string)

	recorder := requestRelease(routesDuringOutage, "small_account", reservationID)

	if recorder.Code != http.StatusOK {
		t.Fatalf("POST /accounts/small_account/reservations/%s/release with the limit unknown: got %d %s, want %d", reservationID, recorder.Code, recorder.Body.String(), http.StatusOK)
	}
	if released := decodeObject(t, recorder); released["status"] != "released" {
		t.Errorf("released reservation status: got %v, want released", released["status"])
	}
	assertUsage(t, routesDuringOutage, "small_account", 0, 0)
}

func TestAPI_ReplayAndReadWhenLimitUnknown(t *testing.T) {
	routesBeforeOutage, routesDuringOutage := newRoutesBeforeAndDuringOutage(t)
	created := decodeObject(t, reserve(routesBeforeOutage, "small_account", "key_1", 31457280))
	reservationID, _ := created["id"].(string)

	replay := reserve(routesDuringOutage, "small_account", "key_1", 31457280)
	read := requestReservation(routesDuringOutage, "small_account", reservationID)
	usage := requestUsage(routesDuringOutage, "small_account")

	if replay.Code != http.StatusOK || replay.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("replay with the limit unknown: got %d, Idempotent-Replayed %q, %s; want 200 and true", replay.Code, replay.Header().Get("Idempotent-Replayed"), replay.Body.String())
	}
	if replayed := decodeObject(t, replay); !maps.Equal(replayed, created) {
		t.Errorf("replay body:\n got  %v\n want the body of its creation %v", replayed, created)
	}
	if read.Code != http.StatusOK {
		t.Fatalf("GET /accounts/small_account/reservations/%s with the limit unknown: got %d %s, want %d", reservationID, read.Code, read.Body.String(), http.StatusOK)
	}
	if readBody := decodeObject(t, read); !maps.Equal(readBody, created) {
		t.Errorf("read body:\n got  %v\n want the body of its creation %v", readBody, created)
	}
	if usage.Code != http.StatusOK {
		t.Fatalf("GET /accounts/small_account/usage with the limit unknown: got %d %s, want %d", usage.Code, usage.Body.String(), http.StatusOK)
	}
	usageBody := decodeObject(t, usage)
	if usageBody["reserved_bytes"] != 31457280.0 || usageBody["active_reservations"] != 1.0 || usageBody["max_active_reservations"] != nil {
		t.Errorf("usage with the limit unknown: got %v, want 31457280 bytes reserved, 1 active and max_active_reservations null", usageBody)
	}
}

func newRoutesBeforeAndDuringOutage(t *testing.T) (http.Handler, http.Handler) {
	t.Helper()
	store, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "reserve.db"), map[string]int64{"small_account": 104857600})
	if err != nil {
		t.Fatalf("open the database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	knownLimit := fakeLimitProvider{maxActive: 5, known: true}
	unknownLimit := fakeLimitProvider{}
	routesBeforeOutage := New(slog.New(slog.DiscardHandler), store, knownLimit, service.New(store, knownLimit, time.Now))
	routesDuringOutage := New(slog.New(slog.DiscardHandler), store, unknownLimit, service.New(store, unknownLimit, time.Now))
	return routesBeforeOutage, routesDuringOutage
}
