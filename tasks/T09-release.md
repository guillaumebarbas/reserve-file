# T09 — Release and read a reservation

| | |
|---|---|
| Issue | [#9](https://github.com/guillaumebarbas/reserve-file/issues/9) |
| Branch | `feat/t09-release` |
| Priority | P0 |
| Estimate | 20 min |
| Depends on | T06 |
| References | [consistency.md](../docs/consistency.md#release-transaction) (release transaction), [api.md](../docs/api.md#release-a-reservation), D10 |

## Overview

`POST /accounts/{accountID}/reservations/{reservationID}/release` gives back the space of an active reservation. `GET /accounts/{accountID}/reservations/{reservationID}` returns a reservation in its current state. This ticket covers the normal path: repeated and concurrent releases are T10, unknown or foreign reservations T17.

## Technical acceptance criteria

- [x] One transaction: load the reservation and the account, call `account.Release`, then save the reservation (`UPDATE … WHERE status = 'active'`, exactly one row) and the counters.
- [x] `200` with the reservation, now `released`, with its `released_at`.
- [x] `GET` returns the reservation as stored.
- [x] Release does not call `LimitProvider`: it depends only on the database.
- [x] Invariants I1 and I2 hold after every test.

## User acceptance criteria

The user is a client application. Account `small_account`.

- Given an active 30 MiB reservation, when the app releases it, then it gets `200` with `status: released` and a `released_at`, and usage shows 30 MiB and one active reservation less.
- Given a reservation, when the app reads it, then it gets its ID, size, status and dates.

## Tests

Unit (`internal/domain`, `internal/service`):
- [x] `TestAccount_Release` — an active reservation is marked released at the given time, and its bytes and its slot come back.
- [x] `TestService_ReleaseDoesNotUseLimit` — a fake limit provider fails the test if it is called; the repository gets the clock's time in UTC, to the millisecond.
- [x] `TestService_ReservationReturnsStoredState` *(added)* — the read returns what the repository stored.

Integration (`internal/repository/sqlite`, `internal/handler`):
- [x] `TestRelease_ActiveReservation` — the reservation is stored as released with its `released_at`, and the counters come back to zero.
- [x] `TestGet_ReturnsCurrentState` — active before the release, released with its `released_at` after.
- [x] `TestRelease_SecondReleaseGivesNothingBack` *(added; became `TestRelease_Twice` in T10, and the guard is now tested by `TestRelease_RollsBackWhenTheRowIsNotUpdated`)* — the second guard: releasing a reservation already released never gives its space back again, even with another active reservation to hide the error.
- [x] `TestRelease_NotFound` *(added)* — an unknown account gives `ErrAccountNotFound`, an unknown reservation `ErrReservationNotFound`; nothing changes.
- [x] `TestGet_UnknownReservation` *(added)* — `ErrReservationNotFound`.
- [x] `TestRelease_DatabaseUnavailable` *(added)* — release and read on a closed database give `ErrServiceUnavailable` and nothing else to unwrap.
- [x] `TestRelease_FailureMidTransactionWritesNothing` *(added)* — a failure on the reservation update, or on the counter update after it, leaves the reservation active and the counters unchanged.
- [x] `TestAPI_ReleaseReservation` — `200` with the reservation now `released` and its `released_at`; usage comes back to zero.
- [x] `TestAPI_GetReservation` — `200` with the same body as its creation.
- [x] `TestAPI_ReservationNotFound` *(added)* — `404 reservation_not_found` for a read or a release of an unknown reservation, `404 account_not_found` for a release on an unknown account.

## Test results

Run on 2026-11-01.

- `gofmt -l .` prints nothing; `go vet ./...` passes; no comment in the Go code apart from `//go:embed`.
- `go test ./...` (also with `-shuffle=on`): 92 tests and 125 subtests pass, none fail or skip (`app` 12, `domain` 5, `handler` 22, `limits` 18, `repository/sqlite` 28, `service` 7). Every package is at 100% on its own.
- `docker compose run --rm test`: passes with the race detector; coverage of `./internal/...` is 100.0%.
- Every test failed first on an assertion, except two repository tests that passed as soon as they were written, because Go had already forced the error branches they cover: `TestRelease_DatabaseUnavailable` and `TestRelease_FailureMidTransactionWritesNothing` took `Store.Release` from 81.2% to 100% and `scanReservation` from 91.7% to 100%.
- `TestRelease_SecondReleaseGivesNothingBack` failed first with the counters at 0 instead of 30 MiB and the invariants broken: without the one-row guard, a second release gave the space back twice. With a single reservation, the `CHECK (reserved_bytes >= 0)` constraint already refused it, so the test uses two.
- Container, checked by hand on a separate Compose project with the real limit source, removed afterwards: 30 MiB reserved on `small_account`, read back `active`, released with `200` and its `released_at`, read back `released`; usage went from 31457280 bytes and 1 active reservation to 0 and 0. An unknown reservation gives `404 reservation_not_found`, a release on an unknown account `404 account_not_found`. A second release answers `500 internal_error`, logged as "it is no longer active", and usage stays at 0: the guard holds until T10 returns the same body.
