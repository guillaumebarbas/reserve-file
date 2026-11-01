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

- [ ] One transaction: load the reservation and the account, call `account.Release`, then save the reservation (`UPDATE … WHERE status = 'active'`, exactly one row) and the counters.
- [ ] `200` with the reservation, now `released`, with its `released_at`.
- [ ] `GET` returns the reservation as stored.
- [ ] Release does not call `LimitProvider`: it depends only on the database.
- [ ] Invariants I1 and I2 hold after every test.

## User acceptance criteria

The user is a client application. Account `acc_small`.

- Given an active 30 MiB reservation, when the app releases it, then it gets `200` with `status: released` and a `released_at`, and usage shows 30 MiB and one active reservation less.
- Given a reservation, when the app reads it, then it gets its ID, size, status and dates.

## Tests

Unit (`internal/domain`, `internal/service`):
- [ ] `TestAccount_Release` — an active reservation is marked released at the given time, and its bytes and its slot come back.
- [ ] `TestService_ReleaseDoesNotUseLimit` — a fake limit provider fails the test if it is called.

Integration (`internal/repository/sqlite`, `internal/handler`):
- [ ] `TestRelease_ActiveReservation`
- [ ] `TestGet_ReturnsCurrentState`
- [ ] `TestAPI_ReleaseReservation`
- [ ] `TestAPI_GetReservation`

## Test results

_Not run yet._
