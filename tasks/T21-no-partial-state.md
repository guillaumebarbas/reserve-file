# T21 — No partial state after a failure mid-transaction

| | |
|---|---|
| Issue | [#21](https://github.com/guillaumebarbas/reserve-file/issues/21) |
| Branch | `feat/t21-no-partial-state` |
| Priority | P1 |
| Estimate | 15 min |
| Depends on | T06, T09 |
| References | [consistency.md, P10](../docs/consistency.md#p10--crashes) and [P11](../docs/consistency.md#p11--the-client-disconnects-mid-request) |

## Overview

Cases:
- A statement fails, or the process crashes, in the middle of a reserve or release transaction.
- The client disconnects while its request is being processed.

The transaction is either fully applied or not at all: never a reservation without its counter update, nor the reverse. Surviving a crash during the transaction is SQLite's own guarantee; the tests here inject a failure between the two statements and cancel the context, and T22 kills the container.

## Technical acceptance criteria

- [x] Any error between `BEGIN` and `COMMIT` rolls back the whole transaction.
- [x] A context cancelled during the transaction leaves no partial state.

## User acceptance criteria

The user is a client application.

- Given the service fails in the middle of a reservation, when the app reads its usage, then the reservation either exists with its space counted, or does not exist at all.
- Given the app disconnects during a reservation, when it retries with the same key, then it ends up with exactly one reservation.

## Tests

Integration (`internal/repository/sqlite`). The counter-update failures listed here at first were already proved by T06 and T09, under other names; this ticket reuses them and adds the rest:
- [x] `TestReserve_FailureMidTransactionWritesNothing` *(T06, reused)* — the insert or the counter update fails: no reservation row remains and the counters are unchanged.
- [x] `TestRelease_FailureMidTransactionWritesNothing` *(T09, reused)* — the reservation update or the counter update fails: the reservation stays `active`.
- [x] `TestReserve_RetryAfterAFailedStatementCreatesOnce` *(added)* — table over every statement of the reserve transaction: account read, key lookup, insert, counter update. No row and unchanged counters after the failure, then a retry with the same key creates the reservation and the next one replays it.
- [x] `TestRelease_RetryAfterAFailedStatementReleasesOnce` *(added)* — table over account read, reservation read, reservation update, counter update. The reservation stays `active`, then a retried release releases it once and a repeated one changes nothing.
- [x] `TestReserve_CancelledContextRollsBack` — the context is cancelled after the insert, and after the counter update just before the commit: no row, unchanged counters, and the retry with the same key creates the reservation once.
- [x] `TestRelease_CancelledContextRollsBack` *(added)* — cancelled after the reservation update, and just before the commit: the reservation stays `active`, and the retried release releases it once.

Container: `TestContainer_SigkillKeepsCommittedReservations` in T22.

## Test results

Run on 2026-11-01.

- `gofmt -l .` prints nothing; `go vet ./...` passes.
- `go test ./...` with `-shuffle=on` passes; the four new tests also pass three runs in a row in one process (each run registers its own SQL function, named with `crypto/rand`, so a second registration never collides). Every package stays at 100% on its own.
- `docker compose run --rm test`: passes with the race detector; coverage of `./internal/...` is 100.0%.
- How each failure is injected:
  - a write fails through a `BEFORE` trigger that raises `RAISE(ABORT, …)`;
  - a read fails because the test renames its table (`ALTER TABLE accounts RENAME TO hidden_accounts`), then renames it back to check the state and retry;
  - a client disconnect is an `AFTER` trigger that calls a SQL function registered with `sqlite.RegisterScalarFunction`; the function cancels the request context, so the cancellation always happens at the same point of the transaction, without sleeps.
- Where the cancellation surfaces, observed five times for each point with a temporary probe test, deleted afterwards:
  - after the insert, the counter update refuses to run (`save account counters: context canceled`);
  - after the counter update, `Commit` refuses (`commit reserve: context canceled`, 4 times out of 5); once the driver interrupted the counter update itself;
  - for a release, the same pattern.

  Either way the transaction is rolled back and nothing is written.
- Test first: the behaviour already existed, so the four new tests passed at once. Each was run against deliberate mutations of the code, reverted afterwards, and each mutation made it fail on its assertions:
  - `Reserve` ignoring a failed counter update: `counter_update_fails_after_the_insert` fails. The row stays without its counters, the invariants break, and the retry is replayed instead of created.
  - `Reserve` ignoring the cancellation (`context.WithoutCancel`): both rows of `TestReserve_CancelledContextRollsBack` fail, because the reservation commits.
  - `Release` ignoring a failed counter update: `counter_update_fails_after_the_reservation_update` fails. The reservation is released while the counters still count it.
  - `Release` ignoring the cancellation: both rows of `TestRelease_CancelledContextRollsBack` fail.
  - Driver errors losing the `ErrServiceUnavailable` sentinel: every row of both retry tests fails. The messages name the statement that failed (`read account`, `read reservation`, `insert reservation`, `save account counters`, `save released reservation`), which proves each row injects its failure at the intended statement.
- No production change: no test showed a gap.
- A failing `COMMIT` cannot be injected deterministically: there is no deferred constraint, and in WAL mode a commit does not wait for readers. A context cancelled just before the commit covers that point, and atomic commit itself is SQLite's guarantee (P10). The crash during a transaction is T22's container test.
