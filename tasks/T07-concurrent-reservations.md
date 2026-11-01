# T07 — Concurrent reservations never exceed the limits

| | |
|---|---|
| Issue | [#7](https://github.com/guillaumebarbas/reserve-file/issues/7) |
| Branch | `feat/t07-concurrent-reservations` |
| Priority | P0 |
| Estimate | 20 min |
| Depends on | T06 |
| References | [consistency.md, P1](../docs/consistency.md#p1--concurrent-reservations-exceed-the-limit), [D6](../docs/decisions.md#d6--writes-serialized-by-the-sqlite-write-lock) |

## Overview

Case: many reservations arrive at the same time and together ask for more than the account has. Only those that fit may succeed, and the counters must stay equal to the active reservations.

This ticket proves it under load and adds `_txlock=immediate`. The first concurrency test is expected to fail without it, which shows why the write lock must be taken at `BEGIN`.

## Technical acceptance criteria

- [ ] An `assertInvariants(t, db)` helper checks that the counters equal `SUM` / `COUNT` of the active rows, for every account.
- [ ] The first concurrency test is run **before** `_txlock=immediate` is added, and its failure is recorded in **Test results**.
- [ ] `_txlock=immediate` is added to the DSN, and the concurrency tests then pass.
- [ ] They also pass under the race detector (`docker compose run --rm test`).

## User acceptance criteria

The user is a client application uploading chunks in parallel. Account `acc_small`: 100 MiB, `MAX_PARALLEL_CHUNKS` = 5.

- Given 70 MiB are free, when 20 requests of 20 MiB arrive at the same time, then exactly 3 get `201` and 17 get `409 insufficient_storage`, and usage shows exactly 60 MiB more.
- Given nothing is reserved, when 50 requests of 1 MiB arrive at the same time, then exactly 5 get `201` and 45 get `409 too_many_active_reservations`.

## Tests

Integration (`internal/repository/sqlite`, `internal/handler`):
- [ ] `TestReserve_ConcurrentRequestsNeverExceedLimit` — 50 goroutines, room for exactly 3: 3 created, 47 `InsufficientStorageError`.
- [ ] `TestReserve_ConcurrentRequestsNeverExceedSlots` — 50 goroutines, 5 slots: 5 created.
- [ ] `TestAPI_ConcurrentReservations` — 20 parallel HTTP requests: exactly 3 `201`.

## Test results

_Not run yet._
