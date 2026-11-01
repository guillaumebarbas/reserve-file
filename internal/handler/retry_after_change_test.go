package handler

import (
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/repository/sqlite"
	"github.com/guillaumebarbas/reserve-file/internal/service"
)

func TestAPI_ReplayOfReleasedReservation(t *testing.T) {
	routes := newRoutesWithAccounts(t, map[string]int64{"small_account": 104857600}, fakeLimitProvider{maxActive: 5, known: true})
	created := decodeObject(t, reserve(routes, "small_account", "key_1", 31457280))
	reservationID, _ := created["id"].(string)
	released := decodeObject(t, requestRelease(routes, "small_account", reservationID))

	retry := reserve(routes, "small_account", "key_1", 31457280)

	if retry.Code != http.StatusOK || retry.Header().Get("Idempotent-Replayed") != "true" {
		t.Errorf("retry after the release: got %d with Idempotent-Replayed %q, want %d with \"true\"", retry.Code, retry.Header().Get("Idempotent-Replayed"), http.StatusOK)
	}
	if body := decodeObject(t, retry); !maps.Equal(body, released) || body["status"] != "released" {
		t.Errorf("retry after the release:\n got  %v\n want the released reservation %v", body, released)
	}
	assertUsage(t, routes, "small_account", 0, 0)
}

func TestAPI_ReplayWhenAccountFull(t *testing.T) {
	testCases := []struct {
		name                   string
		otherReservationsBytes []int64
		wantNewKeyCode         string
		wantReservedBytes      int64
		wantActiveReservations int
	}{
		{
			name:                   "no_space_left",
			otherReservationsBytes: []int64{73400320},
			wantNewKeyCode:         "insufficient_storage",
			wantReservedBytes:      104857600,
			wantActiveReservations: 2,
		},
		{
			name:                   "every_slot_used_including_its_own",
			otherReservationsBytes: []int64{1048576, 1048576, 1048576, 1048576},
			wantNewKeyCode:         "too_many_active_reservations",
			wantReservedBytes:      31457280 + 4*1048576,
			wantActiveReservations: 5,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			routes := newRoutesWithAccounts(t, map[string]int64{"small_account": 104857600}, fakeLimitProvider{maxActive: 5, known: true})
			created := decodeObject(t, reserve(routes, "small_account", "key_1", 31457280))
			for index, bytes := range testCase.otherReservationsBytes {
				if other := reserve(routes, "small_account", fmt.Sprintf("other_key_%d", index), bytes); other.Code != http.StatusCreated {
					t.Fatalf("fill the account with %d bytes: got %d %s, want %d", bytes, other.Code, other.Body.String(), http.StatusCreated)
				}
			}

			retry := reserve(routes, "small_account", "key_1", 31457280)
			newKey := reserve(routes, "small_account", "new_key", 1048576)

			if retry.Code != http.StatusOK || retry.Header().Get("Idempotent-Replayed") != "true" {
				t.Errorf("retry on the full account: got %d with Idempotent-Replayed %q, want %d with \"true\"", retry.Code, retry.Header().Get("Idempotent-Replayed"), http.StatusOK)
			}
			if body := decodeObject(t, retry); !maps.Equal(body, created) {
				t.Errorf("retry on the full account:\n got  %v\n want the original reservation %v", body, created)
			}
			if code := decodeObject(t, newKey)["code"]; newKey.Code != http.StatusConflict || code != testCase.wantNewKeyCode {
				t.Errorf("a new key on the full account: got %d %v, want %d %s", newKey.Code, code, http.StatusConflict, testCase.wantNewKeyCode)
			}
			assertUsage(t, routes, "small_account", testCase.wantReservedBytes, testCase.wantActiveReservations)
		})
	}
}

func TestAPI_ReplayWhenLimitUnknown(t *testing.T) {
	limitProvider := &changingLimitProvider{maxActive: 5, known: true}
	routes := newRoutesWithChangingLimit(t, limitProvider)
	created := decodeObject(t, reserve(routes, "small_account", "key_1", 31457280))
	limitProvider.known = false

	retry := reserve(routes, "small_account", "key_1", 31457280)
	newKey := reserve(routes, "small_account", "key_2", 31457280)

	if retry.Code != http.StatusOK || retry.Header().Get("Idempotent-Replayed") != "true" {
		t.Errorf("retry while the limit is unknown: got %d with Idempotent-Replayed %q, want %d with \"true\"", retry.Code, retry.Header().Get("Idempotent-Replayed"), http.StatusOK)
	}
	if body := decodeObject(t, retry); !maps.Equal(body, created) {
		t.Errorf("retry while the limit is unknown:\n got  %v\n want the original reservation %v", body, created)
	}
	if code := decodeObject(t, newKey)["code"]; newKey.Code != http.StatusServiceUnavailable || code != "limit_unavailable" || newKey.Header().Get("Retry-After") != "5" {
		t.Errorf("a new key while the limit is unknown: got %d %v with Retry-After %q, want %d limit_unavailable with \"5\"", newKey.Code, code, newKey.Header().Get("Retry-After"), http.StatusServiceUnavailable)
	}
	assertUsage(t, routes, "small_account", 31457280, 1)
}

type changingLimitProvider struct {
	maxActive int
	known     bool
}

func (provider *changingLimitProvider) MaxActive() (int, bool) {
	return provider.maxActive, provider.known
}

func newRoutesWithChangingLimit(t *testing.T, limitProvider *changingLimitProvider) http.Handler {
	t.Helper()
	store, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "reserve.db"), map[string]int64{"small_account": 104857600})
	if err != nil {
		t.Fatalf("open the database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return New(slog.New(slog.DiscardHandler), store, limitProvider, service.New(store, limitProvider, time.Now))
}
