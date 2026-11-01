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

func (account *Account) Reserve(candidate Reservation, existing *Reservation, maxActive MaxActive) (Reservation, Outcome, error) {
	if existing != nil {
		return *existing, Replayed, nil
	}
	if candidate.Bytes <= 0 {
		return Reservation{}, 0, ErrBytesNotPositive
	}
	if !maxActive.Known {
		return Reservation{}, 0, ErrLimitUnavailable
	}
	if account.activeReservations >= maxActive.Value {
		return Reservation{}, 0, TooManyActiveReservationsError{ActiveReservations: account.activeReservations, MaxActiveReservations: maxActive.Value}
	}
	if candidate.Bytes > account.LimitBytes-account.reservedBytes {
		return Reservation{}, 0, InsufficientStorageError{RequestedBytes: candidate.Bytes, AvailableBytes: account.AvailableBytes(), LimitBytes: account.LimitBytes}
	}
	account.reservedBytes += candidate.Bytes
	account.activeReservations++
	return candidate, Created, nil
}
