# T06 — Reserve storage

| | |
|---|---|
| Issue | [#6](https://github.com/guillaumebarbas/reserve-file/issues/6) |
| Branch | `feat/t06-reserve` |
| Priority | P0 |
| Estimate | 30 min |
| Depends on | T05 |
| References | [consistency.md](../docs/consistency.md#reserve-transaction) (reserve transaction), [api.md, Reserve space](../docs/api.md#reserve-space), D4 to D8 |

## Overview

`POST /accounts/{accountID}/reservations` reserves space if it fits within the account's storage limit and its `MAX_PARALLEL_CHUNKS` slots, and refuses it otherwise. The `Idempotency-Key` header is required and stored with the reservation. This ticket handles one request at a time: concurrency is T07, retries T08, and the full input validation T13.

## Technical acceptance criteria

- [x] A missing or empty `Idempotency-Key` gives `400 invalid_idempotency_key`.
- [x] `bytes` is decoded into `*int64`; missing, `0` or negative gives `400 invalid_request`.
- [x] `account.Reserve` applies steps 4 to 7 of the reserve transaction, in order: limit unknown, slots, space (with a subtraction, so no overflow), then admission, which grows the counters. Its signature is `Reserve(candidate, maxActive) error` for now: the `existing` reservation of steps 2 and 3, and the outcome that tells a creation from a replay, come with T08.
- [x] The service builds the candidate reservation (UUIDv7, `created_at` from an injected clock, in UTC and to the millisecond, as stored) and passes the current limit.
- [x] The repository loads the account (unknown: `ErrAccountNotFound`, step 1) and calls `account.Reserve`, then inserts the reservation and saves the counters, in one transaction. A refusal writes nothing.
- [x] `201` with `Location` and the reservation body. Refusals follow the api.md catalogue, with their extension fields.
- [x] Invariants I1 and I2 hold after every test.

## User acceptance criteria

The user is a client application uploading files. Account `small_account`: 100 MiB, `MAX_PARALLEL_CHUNKS` = 5.

- Given nothing is reserved, when the app reserves 30 MiB with a new key, then it gets `201`, a reservation ID and a `Location` header, and usage shows 30 MiB reserved and 1 active reservation.
- Given 90 MiB are reserved, when the app asks for 20 MiB, then it gets `409 insufficient_storage` saying "Requested 20.00 MiB but only 10.00 MiB is available (limit 100.00 MiB). Release a reservation or request less space.", and usage is unchanged.
- Given 5 reservations are active, when the app asks for 1 MiB, then it gets `409 too_many_active_reservations`.
- Given the account does not exist, then the app gets `404 account_not_found`.
- Given the request has no `Idempotency-Key`, then the app gets `400 invalid_idempotency_key`.

## Tests

Unit (`internal/domain`, `internal/service`, `internal/handler`):
- [x] `TestAccount_Reserve` — admission cases: fits; exactly fills the space; one byte too many; larger than an empty account's limit; all slots used (checked before space); limit unknown (checked first); usage above a lowered limit; values near `MaxInt64`; `bytes ≤ 0` (`ErrBytesNotPositive`, a guard of the aggregate: the handler refuses such requests first).
- [x] `TestService_PassesCurrentLimit` — with a known and with an unknown limit.
- [x] `TestService_BuildsCandidate` — UUIDv7 ID, `created_at` from the clock in UTC and to the millisecond, status `active`.
- [x] `TestService_ReserveReturnsRepositoryErrors` *(added: covers the service's error branch)*.
- [x] `TestWriteError_Catalogue` *(row added)* — `invalid_idempotency_key`.

Integration (`internal/repository/sqlite`, `internal/handler`):
- [x] `TestReserve_CreatesAndUpdatesCounters`
- [x] `TestReserve_RefusalWritesNothing` — `insufficient_storage`, `too_many_active_reservations`, `limit_unavailable`.
- [x] `TestReserve_UnknownAccount`
- [x] `TestReserve_DatabaseUnavailable` *(added)* — a closed database gives `ErrServiceUnavailable`, wrapping nothing else.
- [x] `TestReserve_FailureMidTransactionWritesNothing` *(added)* — a trigger makes the insert, or the counter update after the insert, fail: nothing is written.
- [x] `TestAPI_ReserveCreated` — `201`, `Location`, body.
- [x] `TestAPI_ReserveRefused` — `insufficient_storage` and `too_many_active_reservations`, with their extension fields, and `404 account_not_found`.
- [x] `TestAPI_ReserveRequiresIdempotencyKey` — missing and empty header.
- [x] `TestAPI_ReserveRejectsInvalidBytes` *(added: the `bytes` criterion had no test)* — missing, `0`, negative, malformed body.

`assertInvariants(t, database)` runs the invariant query of testing.md after every repository test; the API tests check the same counters through `GET /accounts/{accountID}/usage`, since they cannot reach the store's connection.

## Test results

Run on 2026-11-01.

- `gofmt -l .` prints nothing; `go vet ./...` passes.
- `go test ./...` (also with `-shuffle=on`): 73 tests and 113 subtests pass, none fail or skip (`app` 12, `domain` 4, `handler` 19, `limits` 12, `repository/sqlite` 21, `service` 5). Every package is at 100%.
- `docker compose run --rm test`: passes with the race detector; coverage of `./internal/...` is 100.0%.
- Every test failed first on an assertion, except four repository tests that passed as soon as they were written: the domain refusals, the account read and the driver error translation already existed. `TestReserve_RefusalWritesNothing`, `TestReserve_UnknownAccount`, `TestReserve_DatabaseUnavailable` and `TestReserve_FailureMidTransactionWritesNothing` took `Store.Reserve` from 64.3% to 100%, and the package from 93.2% to 100%.
- Container, checked by hand on a separate Compose project with the real limit source, removed afterwards:
  - `POST /accounts/small_account/reservations` with a key and 30 MiB: `201`, `Location` and the body; usage then shows 31457280 bytes reserved and 1 active reservation;
  - 80 MiB more: `409 insufficient_storage`, "Requested 80.00 MiB but only 70.00 MiB is available (limit 100.00 MiB)…";
  - no key: `400 invalid_idempotency_key`;
  - the same request again with the same key: `503 service_unavailable`, logged as the `UNIQUE (account_id, idempotency_key)` violation. Expected until T08 adds the replay.
- `_txlock=immediate` is not added: T07 adds it with its failing concurrency test.
