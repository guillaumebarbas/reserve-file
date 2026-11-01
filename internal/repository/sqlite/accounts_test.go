package sqlite

import (
	"errors"
	"testing"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

func TestAccount_ReturnsCounters(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	execute(t, store, "INSERT INTO reservations (id, account_id, idempotency_key, bytes, status, created_at) VALUES ('reservation_1', 'small_account', 'key_1', 30, 'active', 0)")
	execute(t, store, "UPDATE accounts SET reserved_bytes = 30, active_reservations = 1 WHERE id = 'small_account'")

	account, err := store.Account(t.Context(), "small_account")

	if err != nil {
		t.Fatalf("Account: unexpected error: %v", err)
	}
	want := domain.RestoreAccount("small_account", 104857600, 30, 1)
	if account != want {
		t.Errorf("Account: got %+v, want %+v", account, want)
	}
}

func TestAccount_UnknownAccount(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})

	_, err := store.Account(t.Context(), "unknown_account")

	if !errors.Is(err, domain.ErrAccountNotFound) {
		t.Errorf("Account(unknown_account): got error %v, want domain.ErrAccountNotFound", err)
	}
}

func TestAccount_DatabaseUnavailable(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	_ = store.Close()

	_, err := store.Account(t.Context(), "small_account")

	if !errors.Is(err, domain.ErrServiceUnavailable) {
		t.Errorf("Account on a closed database: got error %v, want domain.ErrServiceUnavailable", err)
	}
	if errors.Unwrap(err) != domain.ErrServiceUnavailable {
		t.Errorf("Account on a closed database: error %v wraps more than domain.ErrServiceUnavailable", err)
	}
}
