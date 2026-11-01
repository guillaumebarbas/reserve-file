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

- [ ] A missing or empty `Idempotency-Key` gives `400 invalid_idempotency_key`.
- [ ] `bytes` is decoded into `*int64`; missing, `0` or negative gives `400 invalid_request`.
- [ ] `account.Reserve(candidate, existing, maxActive)` applies steps 4 to 7 of the reserve transaction, in order: limit unknown, slots, space (with a subtraction, so no overflow), then admission, which grows the counters.
- [ ] The service builds the candidate reservation (UUIDv7, `created_at` from an injected clock) and passes the current limit.
- [ ] The repository loads the account (unknown: `ErrAccountNotFound`, step 1) and calls `account.Reserve`, then inserts the reservation and saves the counters, in one transaction. A refusal writes nothing.
- [ ] `201` with `Location` and the reservation body. Refusals follow the api.md catalogue, with their extension fields.
- [ ] Invariants I1 and I2 hold after every test.

## User acceptance criteria

The user is a client application uploading files. Account `small_account`: 100 MiB, `MAX_PARALLEL_CHUNKS` = 5.

- Given nothing is reserved, when the app reserves 30 MiB with a new key, then it gets `201`, a reservation ID and a `Location` header, and usage shows 30 MiB reserved and 1 active reservation.
- Given 90 MiB are reserved, when the app asks for 20 MiB, then it gets `409 insufficient_storage` saying "Requested 20.00 MiB but only 10.00 MiB is available (limit 100.00 MiB). Release a reservation or request less space.", and usage is unchanged.
- Given 5 reservations are active, when the app asks for 1 MiB, then it gets `409 too_many_active_reservations`.
- Given the account does not exist, then the app gets `404 account_not_found`.
- Given the request has no `Idempotency-Key`, then the app gets `400 invalid_idempotency_key`.

## Tests

Unit (`internal/domain`, `internal/service`):
- [ ] `TestAccount_Reserve` — admission cases: fits; exactly fills the space; one byte too many; larger than an empty account's limit; all slots used (checked before space); limit unknown; values near `MaxInt64`; `bytes ≤ 0`.
- [ ] `TestService_PassesCurrentLimit` — with a known and with an unknown limit.
- [ ] `TestService_BuildsCandidate` — UUIDv7 ID, `created_at` from the clock, status `active`.

Integration (`internal/repository/sqlite`, `internal/handler`):
- [ ] `TestReserve_CreatesAndUpdatesCounters`
- [ ] `TestReserve_RefusalWritesNothing`
- [ ] `TestReserve_UnknownAccount`
- [ ] `TestAPI_ReserveCreated` — `201`, `Location`, body.
- [ ] `TestAPI_ReserveRefused` — `insufficient_storage` and `too_many_active_reservations`, with their extension fields.
- [ ] `TestAPI_ReserveRequiresIdempotencyKey`

## Test results

_Not run yet._
