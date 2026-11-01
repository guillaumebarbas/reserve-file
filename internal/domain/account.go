package domain

type Account struct {
	ID                 string
	LimitBytes         int64
	reservedBytes      int64
	activeReservations int
}

func RestoreAccount(accountID string, limitBytes, reservedBytes int64, activeReservations int) Account {
	return Account{
		ID:                 accountID,
		LimitBytes:         limitBytes,
		reservedBytes:      reservedBytes,
		activeReservations: activeReservations,
	}
}

func (account Account) AvailableBytes() int64 {
	return max(0, account.LimitBytes-account.reservedBytes)
}

func (account Account) ReservedBytes() int64 {
	return account.reservedBytes
}

func (account Account) ActiveReservations() int {
	return account.activeReservations
}
