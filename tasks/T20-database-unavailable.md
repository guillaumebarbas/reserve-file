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

- [ ] Busy, locked, full, read-only, I/O and closed-database errors are translated to `ErrServiceUnavailable`, which gives `503 service_unavailable` with `Retry-After: 1` and no internal text.
- [ ] Nothing is written when the request fails.
- [ ] `/readyz` returns `503` while the database does not answer.

## User acceptance criteria

The user is a client application.

- Given the database stays locked for more than 5 s, when the app reserves, then it gets `503 service_unavailable` with `Retry-After: 1`; when it retries later with the same key, the space is reserved exactly once.
- Given the disk is full or the file is read-only, when the app reserves or releases, then it gets `503`, and usage is unchanged.

## Tests

Integration (`internal/repository/sqlite`, `internal/handler`):
- [ ] `TestReserve_DatabaseLockedReturnsUnavailable` — a second connection holds `BEGIN IMMEDIATE`; nothing is written; the call succeeds once the lock is released.
- [ ] `TestRepository_DiskFullReturnsUnavailable` — `PRAGMA max_page_count` lowered by the test.
- [ ] `TestRepository_ReadOnlyDatabaseReturnsUnavailable` — the file opened with `mode=ro`.
- [ ] `TestRepository_ClosedDatabaseReturnsUnavailable`
- [ ] `TestAPI_DatabaseClosedReturns503`
- [ ] `TestAPI_RetryAfterDatabaseRecovers` — `503`, then `201` with the same key, and a single reservation.

## Test results

_Not run yet._
