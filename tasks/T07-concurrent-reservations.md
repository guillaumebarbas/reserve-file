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

- [x] An `assertInvariants(t, database)` helper checks that the counters equal `SUM` / `COUNT` of the active rows, for every account.
- [x] The first concurrency test is run **before** `_txlock=immediate` is added, and its failure is recorded in **Test results**.
- [x] `_txlock=immediate` is added to the DSN, and the concurrency tests then pass.
- [x] They also pass under the race detector (`docker compose run --rm test`).

## User acceptance criteria

The user is a client application uploading chunks in parallel. Account `small_account`: 100 MiB, `MAX_PARALLEL_CHUNKS` = 5.

- Given 70 MiB are free, when 20 requests of 20 MiB arrive at the same time, then exactly 3 get `201` and 17 get `409 insufficient_storage`, and usage shows exactly 60 MiB more.
- Given nothing is reserved, when 50 requests of 1 MiB arrive at the same time, then exactly 5 get `201` and 45 get `409 too_many_active_reservations`.

## Tests

Integration (`internal/repository/sqlite`, `internal/handler`):
- [x] `TestReserve_ConcurrentRequestsNeverExceedLimit` — 50 goroutines of 20 MiB with 70 MiB free, room for exactly 3: 3 created, 47 `InsufficientStorageError`, counters at 90 MiB and 4 active, invariants hold.
- [x] `TestReserve_ConcurrentRequestsNeverExceedSlots` — 50 goroutines of 1 MiB, 5 slots: 5 created, 45 `TooManyActiveReservationsError`, invariants hold.
- [x] `TestAPI_ConcurrentReservations` — table, both user criteria through HTTP: 20 parallel requests of 20 MiB with 70 MiB free give exactly 3 `201` and 17 `409 insufficient_storage`, usage 60 MiB more; 50 parallel requests of 1 MiB give exactly 5 `201` and 45 `409 too_many_active_reservations`.

`assertInvariants` was written in T06 (`internal/repository/sqlite/reservations_test.go`) and is reused here. The concurrency tests live in their own files (`concurrency_test.go` in both packages); the repository is called from one helper, `reserveConcurrently`.

## Test results

Run on 2026-11-01.

**Before `_txlock=immediate`** (deferred `BEGIN`), two runs of the new tests:

```
run 1
concurrency_test.go:25: 50 concurrent requests of 20 MiB with 70 MiB free: got {created:3 insufficientStorage:28 tooManyActive:0 unexpected:map[service unavailable: insert reservation: database is locked (5) (SQLITE_BUSY):19]}, want 3 created and 47 refused for insufficient storage
--- FAIL: TestReserve_ConcurrentRequestsNeverExceedLimit
--- PASS: TestReserve_ConcurrentRequestsNeverExceedSlots
--- PASS: TestAPI_ConcurrentReservations

run 2
concurrency_test.go:25: 50 concurrent requests of 20 MiB with 70 MiB free: got {created:3 insufficientStorage:40 tooManyActive:0 unexpected:map[service unavailable: insert reservation: database is locked (5) (SQLITE_BUSY):7]}, want 3 created and 47 refused for insufficient storage
--- FAIL: TestReserve_ConcurrentRequestsNeverExceedLimit
concurrency_test.go:42: 50 concurrent requests of 1 MiB with 5 slots: got {created:5 insufficientStorage:0 tooManyActive:33 unexpected:map[service unavailable: insert reservation: database is locked (5) (SQLITE_BUSY):12]}, want 5 created and 45 refused for too many active reservations
--- FAIL: TestReserve_ConcurrentRequestsNeverExceedSlots
concurrency_test.go:61: 20 concurrent requests of 20971520 bytes: got map[201:3 503 service_unavailable:17], want map[201:3 409 insufficient_storage:17]
--- FAIL: TestAPI_ConcurrentReservations/room_for_three
```

The limit was never exceeded: in WAL mode, a transaction that read an older snapshot cannot write. But the requests that lost the race failed to upgrade their read lock with `SQLITE_BUSY`, which `busy_timeout` cannot help, and got `503` instead of their real answer. The failure is intermittent, which is why it was run twice.

**After `_txlock=immediate`**:
- the three tests pass five runs in a row each;
- `gofmt -l .` prints nothing; `go vet ./...` passes;
- `go test ./...` (also with `-shuffle=on`): 82 tests and 120 subtests pass, none fail or skip (`app` 12, `domain` 4, `handler` 20, `limits` 18, `repository/sqlite` 23, `service` 5);
- `docker compose run --rm test`: passes with the race detector; coverage of `./internal/...` is 100.0%.
