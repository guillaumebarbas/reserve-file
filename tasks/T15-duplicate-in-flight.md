# T15 — Same request twice at the same time

| | |
|---|---|
| Issue | [#15](https://github.com/guillaumebarbas/reserve-file/issues/15) |
| Branch | `feat/t15-duplicate-in-flight` |
| Priority | P1 |
| Estimate | 25 min |
| Depends on | T08 |
| References | [consistency.md, P3](../docs/consistency.md#p3--the-same-request-arrives-twice-at-the-same-time), [D9](../docs/decisions.md#d9--in-flight-duplicates-get-409) |

## Overview

Case: a double click, or a client retrying while its first request is still being processed.

Before this ticket, the second request waits for the database lock and then gets the replay: correct, but blocked requests pile up during retry storms. With it, the second request is told at once to try again (`409 request_in_progress`), as the IETF draft specifies, and its next retry gets the replay.

## Technical acceptance criteria

- [x] In-flight guard in `service`, keyed by `(account_id, key)`, checked before any database access.
- [x] A second request with the same pair gets `409 request_in_progress` with `Retry-After: 1`.
- [x] The entry is released with `defer`, whether the request succeeds, fails or panics.
- [x] A unique-constraint violation on `(account_id, idempotency_key)` is translated to `ErrRequestInProgress`.
- [x] Release does not go through the guard.

## User acceptance criteria

The user is a client application. Account `small_account`.

- Given a request with key K is still being processed, when another request with K arrives, then it gets `409 request_in_progress` with `Retry-After: 1`; retrying a moment later returns the reservation.
- Given 50 requests with the same key arrive at once, then exactly one reservation exists; one response is `201` and the others are `200` or `409`.
- Given two requests with different keys, or with the same key on two accounts, arrive at once, then neither blocks the other.

## Tests

Unit (`internal/service`):
- [x] `TestService_DuplicateInFlightReturnsInProgress` — the fake repository is blocked on a channel.
- [x] `TestService_DifferentKeysAreNotBlocked`
- [x] `TestService_SameKeyOnDifferentAccountsIsNotBlocked`
- [x] `TestService_GuardReleasedAfterError`
- [x] `TestService_GuardReleasedAfterPanic`

Integration (`internal/repository/sqlite`, `internal/handler`):
- [x] `TestReserve_ConcurrentSameKeyCreatesOneRow` — 50 goroutines, no guard at this layer: 1 `Create`, 49 `Replay`, 1 row. Expected to pass already: it shows that the database alone prevents a duplicate.
- [x] `TestReserve_UniqueViolationMapsToInProgress` — a trigger created by the test inserts the same key first.
- [x] `TestAPI_ConcurrentSameKey`

## Test results

Run on 2026-11-01.

- `gofmt -l .` prints nothing; `go vet ./...` passes; no Go comment apart from `//go:embed`.
- `go test ./...` (also with `-shuffle=on`): 123 tests and 163 subtests pass, none fail or skip (`app` 13, `domain` 6, `handler` 33, `limits` 18, `repository/sqlite` 41, `service` 12).
- `docker compose run --rm test`: passes with the race detector; coverage of `./internal/...` is 100.0%.
- Test first:
  - `TestService_DuplicateInFlightReturnsInProgress` failed first: without a guard the duplicate reached the repository (`got error <nil>`, 2 repository calls instead of 1). Its fake blocks only the first call of the pair, so the test fails instead of hanging.
  - `TestReserve_UniqueViolationMapsToInProgress` failed first: the violation came out as `service unavailable: insert reservation: … UNIQUE constraint failed: reservations.account_id, reservations.idempotency_key (2067)`.
  - These passed as soon as they were written, because they describe what the guard must not break or what the database already did. Each one was run against a deliberate mutation of the code, reverted afterwards, and failed:
    - `TestService_DifferentKeysAreNotBlocked`: a guard keyed by the account only gives `request in progress` for another key.
    - `TestService_SameKeyOnDifferentAccountsIsNotBlocked`: a guard keyed by the key only gives `request in progress` on another account.
    - `TestService_GuardReleasedAfterError` and `TestService_GuardReleasedAfterPanic`: a pair released only after a success, without `defer`, leaves the key stuck after an error or a panic (`request in progress`, 1 repository call instead of 2).
    - `TestReserve_ConcurrentSameKeyCreatesOneRow` (expected to pass already, as the ticket says): with the key lookup disabled, 49 of the 50 requests fail on the `UNIQUE` constraint and none is replayed.
    - `TestAPI_ConcurrentSameKey`: with the guard, the `UNIQUE` constraint and the key lookup all removed, several requests get `201` and the rest `409 too_many_active_reservations`. With only the constraint and the lookup removed, it still passes: the guard alone keeps the 49 duplicates out, which is its job.
- The unique-violation translation uses the driver's own error type and extended result code (`SQLITE_CONSTRAINT_UNIQUE`, 2067), never the message text. The primary key has its own code (`SQLITE_CONSTRAINT_PRIMARYKEY`), so a duplicate reservation ID is not mistaken for a key in flight.
- Release does not go through the guard: `Service.Release` never calls it, as the code shows.
