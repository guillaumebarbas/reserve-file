# T20 — Database unavailable

| | |
|---|---|
| Issue | [#20](https://github.com/guillaumebarbas/reserve-file/issues/20) |
| Branch | `feat/t20-database-unavailable` |
| Priority | P1 |
| Estimate | 20 min |
| Depends on | T06, T09 |
| References | [consistency.md, P9](../docs/consistency.md#p9--the-database-is-unavailable), [testing.md](../docs/testing.md#how-failures-are-simulated) |

## Overview

Cases: the database stays locked longer than the busy timeout, the disk is full, the file is read-only, or the database is closed. The request fails with `503` and a `Retry-After`, the transaction is rolled back, and a retry with the same key is safe.

## Technical acceptance criteria

- [x] Busy, locked, full, read-only, I/O and closed-database errors are translated to `ErrServiceUnavailable`, which gives `503 service_unavailable` with `Retry-After: 1` and no internal text.
- [x] Nothing is written when the request fails.
- [x] `/readyz` returns `503` while the database does not answer.

## User acceptance criteria

The user is a client application.

- Given the database stays locked for more than 5 s, when the app reserves, then it gets `503 service_unavailable` with `Retry-After: 1`; when it retries later with the same key, the space is reserved exactly once.
- Given the disk is full or the file is read-only, when the app reserves or releases, then it gets `503`, and usage is unchanged.

## Tests

Integration (`internal/repository/sqlite`, `internal/handler`):
- [x] `TestReserve_DatabaseLockedReturnsUnavailable` — a second connection holds `BEGIN IMMEDIATE`; nothing is written; once the lock is released, the same key is created once, then replayed.
- [x] `TestRelease_DatabaseLockedReturnsUnavailable` *(added)* — the same for a release: the reservation stays active while locked, then is released.
- [x] `TestRepository_DiskFullReturnsUnavailable` — `PRAGMA max_page_count` capped at the current size; a new reservation and the release of a stored one both need new pages and fail; nothing changes.
- [x] `TestRepository_ReadOnlyDatabaseReturnsUnavailable` — a second store on the same file, opened with `mode=ro`, cannot reserve or release; nothing changes.
- [x] `TestRepository_ClosedDatabaseReturnsUnavailable` — already proved by `TestReserve_DatabaseUnavailable` (T06), `TestRelease_DatabaseUnavailable` (T09) and `TestAccount_DatabaseUnavailable` (T05), which close the store before the call; not duplicated.
- [x] `TestAPI_DatabaseClosedReturns503` — reserve, release, read and usage on a closed database: `503 service_unavailable`, `Retry-After: 1`, no internal text in the body, the cause in the log with the request ID.
- [x] `TestAPI_RetryAfterDatabaseRecovers` — `503`, then `201` with the same key, then a replay, and a single reservation.
- `/readyz` answering `503` while the database does not answer is T03's `TestAPI_Readyz`; not duplicated.

## Test results

Run on 2026-11-01. No production code changed: every driver error already went through `translateDriverError` to `ErrServiceUnavailable`, and `writeError` already answered `503` with `Retry-After: 1` and logged the cause.

- `gofmt -l .` prints nothing; `go vet ./...` passes.
- `go test ./...` (also with `-shuffle=on`): 121 tests and 167 subtests pass, none fail or skip (`app` 13, `domain` 6, `handler` 34, `limits` 18, `repository/sqlite` 43, `service` 7). Every package is at 100% on its own.
- `docker compose run --rm test`: passes with the race detector; coverage of `./internal/...` is 100.0%.
- The new tests passed as soon as they were written, since the behaviour already existed. Each was run against deliberate mutations of the code, reverted afterwards:
  - `translateDriverError` without the sentinel (`fmt.Errorf("%s: %w", action, err)`): the four repository tests fail, and show the SQLite error that would have leaked (`database is locked (5) (SQLITE_BUSY)`, `database or disk is full (13)`, `attempt to write a readonly database (8)`). With `%v` instead, the two API tests get `500 internal_error` instead of `503`.
  - The failed insert and the failed release update ignored: `TestRepository_DiskFullReturnsUnavailable` fails, because the counters moved without the rows (`reservedBytes` 1048576 instead of 31457280, invariants broken). The read-only test does not catch this one, since there every write fails, the counters included.
  - `Retry-After` removed from `service_unavailable`: both API tests fail.
  - `writeError` logging only `500`: both API tests fail, because the cause no longer reaches the log.
- The lock case takes about 0.1 s instead of 5 s: the store's pool is limited to one connection in the test, and the test lowers that connection's `busy_timeout` to 100 ms, without any production API. Through HTTP the store keeps its 5 s busy timeout, so `TestAPI_RetryAfterDatabaseRecovers` uses a failure it can lift at once: a trigger, created by a second connection, aborts every insert, then is dropped.
- A full disk is simulated by capping `max_page_count` at the current page count. A normal reservation still fits in the existing pages, so the test uses a key of 10,000 characters, which spills over several pages. Storing it, or rewriting its row on release, then needs new pages and fails with `SQLITE_FULL`. Keys are limited to 255 characters at the API; the repository is called directly here.
- An I/O error cannot be provoked reliably; it takes the same `translateDriverError` path as the cases above.
