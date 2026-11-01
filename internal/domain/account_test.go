package domain

import "testing"

func TestAccount_AvailableBytesNeverNegative(t *testing.T) {
	testCases := []struct {
		name          string
		limitBytes    int64
		reservedBytes int64
		want          int64
	}{
		{"nothing_reserved", 1000, 0, 1000},
		{"partly_reserved", 1000, 300, 700},
		{"fully_reserved", 1000, 1000, 0},
		{"reserved_above_a_lowered_limit", 500, 800, 0},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			account := RestoreAccount("large_account", testCase.limitBytes, testCase.reservedBytes, 1)

			got := account.AvailableBytes()

			if got != testCase.want {
				t.Errorf("AvailableBytes with a limit of %d and %d reserved: got %d, want %d", testCase.limitBytes, testCase.reservedBytes, got, testCase.want)
			}
		})
	}
}

func TestRestoreAccount_KeepsCounters(t *testing.T) {
	account := RestoreAccount("small_account", 104857600, 30, 1)

	if account.ID != "small_account" || account.LimitBytes != 104857600 || account.ReservedBytes() != 30 || account.ActiveReservations() != 1 {
		t.Errorf("RestoreAccount: got %s with a limit of %d, %d reserved and %d active, want small_account with a limit of 104857600, 30 reserved and 1 active",
			account.ID, account.LimitBytes, account.ReservedBytes(), account.ActiveReservations())
	}
}
