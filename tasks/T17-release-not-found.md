# T17 — Release of an unknown or foreign reservation

| | |
|---|---|
| Issue | [#17](https://github.com/guillaumebarbas/reserve-file/issues/17) |
| Branch | `feat/t17-release-not-found` |
| Priority | P1 |
| Estimate | 10 min |
| Depends on | T09 |
| References | [consistency.md, P6](../docs/consistency.md#p6--double-release-and-concurrent-releases), [api.md, Errors](../docs/api.md#errors) |

## Overview

Case: the app releases or reads a reservation ID that does not exist, or that belongs to another account. Both get the same `404`, so the service never reveals that a reservation exists in another account.

## Technical acceptance criteria

- [x] An unknown ID, a malformed ID, or the ID of another account's reservation gives `404 reservation_not_found`, with the same body shape in every case.
- [x] Nothing is written.

## User acceptance criteria

The user is a client application.

- Given a reservation of `large_account`, when the app releases or reads it through `small_account`, then it gets `404`, exactly as for an ID that does not exist.

## Tests

Integration (`internal/repository/sqlite`, `internal/handler`):
- [x] `TestRelease_UnknownReservation` — already covered since T09 by the `unknown_reservation` row of `TestRelease_NotFound` (and `TestGet_UnknownReservation` for the read); reused, not duplicated.
- [x] `TestRelease_OtherAccountIsNotFound` — releasing `large_account`'s reservation through `small_account` gives `ErrReservationNotFound`; the reservation stays active, both accounts' counters are unchanged, and the invariants hold.
- [x] `TestGet_OtherAccountIsNotFound` — reading it through `small_account` gives `ErrReservationNotFound`.
- [x] `TestAPI_ReleaseUnknownAndForeignLookTheSame` — an unknown ID, a malformed ID (`not-a-uuid`) and another account's ID, each through GET and release: `404 reservation_not_found`, exactly the fields `type`, `title`, `status`, `code`, `detail`, `request_id`, and the same detail template. Afterwards both accounts' usage is unchanged and the other account's reservation is still active.

## Test results

Run on 2026-11-01.

- `gofmt -l .` prints nothing; `go vet ./...` passes.
- `go test ./...` (also with `-shuffle=on`): 118 tests and 166 subtests pass, none fail or skip; every package is at 100% on its own.
- `docker compose run --rm test`: passes with the race detector; coverage of `./internal/...` is 100.0%.
- No production code changed: since T09, the read by ID is scoped by `account_id` and nothing validates the ID's format, so the three new tests passed as soon as they were written. Each was proved against a deliberate mutation of the code, reverted afterwards:
  - the read by ID ignoring the account (`WHERE id = ? AND ? IS NOT NULL`): `TestGet_OtherAccountIsNotFound` got the reservation; `TestRelease_OtherAccountIsNotFound` got `service unavailable … CHECK constraint failed: reserved_bytes >= 0` instead of `ErrReservationNotFound`; through the API, the GET answered `200` with the other account's reservation and the release `503`;
  - a handler refusing an ID that is not 36 characters with `400 invalid_request`: the malformed row answered `400` instead of `404`;
  - a not-found body with one more field (`requested_bytes`): every row reported the extra field.
- Container, checked by hand on a separate Compose project, removed afterwards: 30 MiB reserved on `large_account`, then GET and release through `small_account` of an unknown ID, of `not-a-uuid` and of that reservation's ID: six `404 reservation_not_found` with the same body apart from the ID in the detail; `large_account` still at 31457280 bytes and 1 active, its reservation still `active`, `small_account` at 0.
