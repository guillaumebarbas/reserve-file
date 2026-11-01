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

func (account *Account) Reserve(candidate Reservation, maxActive MaxActive) error {
	if candidate.Bytes <= 0 {
		return ErrBytesNotPositive
	}
	if !maxActive.Known {
		return ErrLimitUnavailable
	}
	if account.activeReservations >= maxActive.Value {
		return TooManyActiveReservationsError{ActiveReservations: account.activeReservations, MaxActiveReservations: maxActive.Value}
	}
	if candidate.Bytes > account.LimitBytes-account.reservedBytes {
		return InsufficientStorageError{RequestedBytes: candidate.Bytes, AvailableBytes: account.AvailableBytes(), LimitBytes: account.LimitBytes}
	}
	account.reservedBytes += candidate.Bytes
	account.activeReservations++
	return nil
}
