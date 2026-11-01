package handler

import (
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/repository/sqlite"
	"github.com/guillaumebarbas/reserve-file/internal/service"
)

func TestAPI_UsageAfterLimitLowered(t *testing.T) {
	routes, _ := newRoutesWithLimitLowered(t)

	usage := decodeObject(t, requestUsage(routes, "small_account"))
	refusal := reserve(routes, "small_account", "key_after_lowering", 1048576)

	wantUsage := map[string]any{
		"account_id":              "small_account",
		"limit_bytes":             52428800.0,
		"reserved_bytes":          94371840.0,
		"available_bytes":         0.0,
		"active_reservations":     3.0,
		"max_active_reservations": 5.0,
	}
	if !maps.Equal(usage, wantUsage) {
		t.Errorf("usage with 90 MiB reserved under a limit lowered to 50 MiB:\n got  %v\n want %v", usage, wantUsage)
	}
	if refusal.Code != http.StatusConflict {
		t.Fatalf("reserve 1 MiB under the lowered limit: got %d %s, want %d", refusal.Code, refusal.Body.String(), http.StatusConflict)
	}
	refusalBody := decodeObject(t, refusal)
	if refusalBody["code"] != "insufficient_storage" || refusalBody["available_bytes"] != 0.0 || refusalBody["limit_bytes"] != 52428800.0 || refusalBody["requested_bytes"] != 1048576.0 {
		t.Errorf("refusal under the lowered limit: got %v, want insufficient_storage with 0 bytes available out of 52428800", refusalBody)
	}
	assertUsage(t, routes, "small_account", 94371840, 3)
}

func TestAPI_ReleaseAfterLimitLowered(t *testing.T) {
	routes, reservationIDs := newRoutesWithLimitLowered(t)

	firstRelease := requestRelease(routes, "small_account", reservationIDs[0])
	usageStillAbove := decodeObject(t, requestUsage(routes, "small_account"))
	secondRelease := requestRelease(routes, "small_account", reservationIDs[1])
	fitsAgain := reserve(routes, "small_account", "key_after_releases", 20971520)

	if firstRelease.Code != http.StatusOK || secondRelease.Code != http.StatusOK {
		t.Fatalf("releases above the lowered limit: got %d and %d, want 200 both times", firstRelease.Code, secondRelease.Code)
	}
	if usageStillAbove["reserved_bytes"] != 62914560.0 || usageStillAbove["available_bytes"] != 0.0 {
		t.Errorf("usage after one release: got %v bytes reserved and %v available, want 62914560 and 0", usageStillAbove["reserved_bytes"], usageStillAbove["available_bytes"])
	}
	if fitsAgain.Code != http.StatusCreated {
		t.Errorf("reserve the 20 MiB left under 50 MiB: got %d %s, want %d", fitsAgain.Code, fitsAgain.Body.String(), http.StatusCreated)
	}
	assertUsage(t, routes, "small_account", 52428800, 2)
}

func TestAPI_ReserveAfterMaxActiveLowered(t *testing.T) {
	store := openDatabaseForRoutes(t, filepath.Join(t.TempDir(), "reserve.db"), map[string]int64{"small_account": 104857600})
	routesWithFiveSlots := newRoutesOnStore(store, fakeLimitProvider{maxActive: 5, known: true})
	var reservationIDs []string
	for reservationNumber := 1; reservationNumber <= 5; reservationNumber++ {
		reservationIDs = append(reservationIDs, reserveAndReadID(t, routesWithFiveSlots, fmt.Sprintf("key_%d", reservationNumber), 1048576))
	}
	routes := newRoutesOnStore(store, fakeLimitProvider{maxActive: 3, known: true})

	usage := decodeObject(t, requestUsage(routes, "small_account"))
	refusal := reserve(routes, "small_account", "key_6", 1048576)
	var releaseStatuses []int
	for _, reservationID := range reservationIDs[:3] {
		releaseStatuses = append(releaseStatuses, requestRelease(routes, "small_account", reservationID).Code)
	}
	fitsAgain := reserve(routes, "small_account", "key_7", 1048576)

	if usage["active_reservations"] != 5.0 || usage["max_active_reservations"] != 3.0 {
		t.Errorf("usage after MAX_PARALLEL_CHUNKS was lowered: got %v active for a maximum of %v, want 5 for 3", usage["active_reservations"], usage["max_active_reservations"])
	}
	refusalBody := decodeObject(t, refusal)
	if refusal.Code != http.StatusConflict || refusalBody["code"] != "too_many_active_reservations" || refusalBody["active_reservations"] != 5.0 || refusalBody["max_active_reservations"] != 3.0 {
		t.Errorf("reserve with 5 active and a maximum of 3: got %d %v, want 409 too_many_active_reservations with 5 active and a maximum of 3", refusal.Code, refusalBody)
	}
	wantReleaseStatuses := []int{http.StatusOK, http.StatusOK, http.StatusOK}
	if !slices.Equal(releaseStatuses, wantReleaseStatuses) {
		t.Errorf("releases above the lowered maximum: got %v, want %v", releaseStatuses, wantReleaseStatuses)
	}
	if fitsAgain.Code != http.StatusCreated {
		t.Errorf("reserve with 2 active and a maximum of 3: got %d %s, want %d", fitsAgain.Code, fitsAgain.Body.String(), http.StatusCreated)
	}
	assertUsage(t, routes, "small_account", 3*1048576, 3)
}

func newRoutesWithLimitLowered(t *testing.T) (http.Handler, []string) {
	t.Helper()
	limitProvider := fakeLimitProvider{maxActive: 5, known: true}
	databasePath := filepath.Join(t.TempDir(), "reserve.db")
	storeBeforeRestart := openDatabaseForRoutes(t, databasePath, map[string]int64{"small_account": 104857600})
	routesBeforeRestart := newRoutesOnStore(storeBeforeRestart, limitProvider)
	var reservationIDs []string
	for reservationNumber := 1; reservationNumber <= 3; reservationNumber++ {
		reservationIDs = append(reservationIDs, reserveAndReadID(t, routesBeforeRestart, fmt.Sprintf("key_%d", reservationNumber), 31457280))
	}
	_ = storeBeforeRestart.Close()
	storeAfterRestart := openDatabaseForRoutes(t, databasePath, map[string]int64{"small_account": 52428800})
	return newRoutesOnStore(storeAfterRestart, limitProvider), reservationIDs
}

func openDatabaseForRoutes(t *testing.T, path string, accounts map[string]int64) *sqlite.Store {
	t.Helper()
	store, err := sqlite.Open(t.Context(), path, accounts)
	if err != nil {
		t.Fatalf("open the database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func newRoutesOnStore(store *sqlite.Store, limitProvider fakeLimitProvider) http.Handler {
	return New(slog.New(slog.DiscardHandler), store, limitProvider, service.New(store, limitProvider, time.Now))
}

func reserveAndReadID(t *testing.T, routes http.Handler, idempotencyKey string, bytes int64) string {
	t.Helper()
	recorder := reserve(routes, "small_account", idempotencyKey, bytes)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("reserve %d bytes with %s: got %d %s, want %d", bytes, idempotencyKey, recorder.Code, recorder.Body.String(), http.StatusCreated)
	}
	reservationID, _ := decodeObject(t, recorder)["id"].(string)
	return reservationID
}
