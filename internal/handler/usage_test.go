package handler

import (
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/guillaumebarbas/reserve-file/internal/repository/sqlite"
	"github.com/guillaumebarbas/reserve-file/internal/service"
)

func TestAPI_Usage(t *testing.T) {
	routes := newRoutesWithLargeAccount(t, fakeLimitProvider{maxActive: 5, known: true})

	recorder := requestUsage(routes, "large_account")

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /accounts/large_account/usage: got %d %s, want %d", recorder.Code, recorder.Body.String(), http.StatusOK)
	}
	want := map[string]any{
		"account_id":              "large_account",
		"limit_bytes":             10737418240.0,
		"reserved_bytes":          0.0,
		"available_bytes":         10737418240.0,
		"active_reservations":     0.0,
		"max_active_reservations": 5.0,
	}
	if got := decodeObject(t, recorder); !maps.Equal(got, want) {
		t.Errorf("usage body:\n got  %v\n want %v", got, want)
	}
}

func TestAPI_UsageUnknownAccount(t *testing.T) {
	routes := newRoutesWithLargeAccount(t, fakeLimitProvider{maxActive: 5, known: true})

	recorder := requestUsage(routes, "unknown_account")

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("GET /accounts/unknown_account/usage: got %d %s, want %d", recorder.Code, recorder.Body.String(), http.StatusNotFound)
	}
	body := decodeObject(t, recorder)
	if body["code"] != "account_not_found" || body["detail"] != "Account 'unknown_account' does not exist." {
		t.Errorf("GET /accounts/unknown_account/usage: got code %v and detail %q, want account_not_found and %q", body["code"], body["detail"], "Account 'unknown_account' does not exist.")
	}
}

func TestAPI_UsageWhenLimitUnknown(t *testing.T) {
	routes := newRoutesWithLargeAccount(t, fakeLimitProvider{})

	recorder := requestUsage(routes, "large_account")

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /accounts/large_account/usage: got %d %s, want %d", recorder.Code, recorder.Body.String(), http.StatusOK)
	}
	want := map[string]any{
		"account_id":              "large_account",
		"limit_bytes":             10737418240.0,
		"reserved_bytes":          0.0,
		"available_bytes":         10737418240.0,
		"active_reservations":     0.0,
		"max_active_reservations": nil,
	}
	if got := decodeObject(t, recorder); !maps.Equal(got, want) {
		t.Errorf("usage body:\n got  %v\n want %v", got, want)
	}
}

func newRoutesWithLargeAccount(t *testing.T, limitProvider fakeLimitProvider) http.Handler {
	t.Helper()
	store, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "reserve.db"), map[string]int64{"large_account": 10737418240})
	if err != nil {
		t.Fatalf("open the database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return New(slog.New(slog.DiscardHandler), store, service.New(store, limitProvider))
}

func requestUsage(routes http.Handler, accountID string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	routes.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/accounts/"+accountID+"/usage", nil))
	return recorder
}

func decodeObject(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &object); err != nil {
		t.Fatalf("decode body %q: %v", recorder.Body.String(), err)
	}
	return object
}

type fakeLimitProvider struct {
	maxActive int
	known     bool
}

func (provider fakeLimitProvider) MaxActive() (int, bool) {
	return provider.maxActive, provider.known
}
