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

- [ ] Any error between `BEGIN` and `COMMIT` rolls back the whole transaction.
- [ ] A context cancelled during the transaction leaves no partial state.

## User acceptance criteria

The user is a client application.

- Given the service fails in the middle of a reservation, when the app reads its usage, then the reservation either exists with its space counted, or does not exist at all.
- Given the app disconnects during a reservation, when it retries with the same key, then it ends up with exactly one reservation.

## Tests

Integration (`internal/repository/sqlite`):
- [ ] `TestReserve_CounterUpdateFailureRollsBack` — a trigger on `accounts` raises an error: no reservation row remains.
- [ ] `TestRelease_CounterUpdateFailureRollsBack` — the reservation stays `active`.
- [ ] `TestReserve_CancelledContextRollsBack`

Container: `TestContainer_SigkillKeepsCommittedReservations` in T22.

## Test results

_Not run yet._
