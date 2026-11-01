# T11 — Restart and graceful shutdown

| | |
|---|---|
| Issue | [#11](https://github.com/guillaumebarbas/reserve-file/issues/11) |
| Branch | `feat/t11-restart-and-shutdown` |
| Priority | P0 |
| Estimate | 15 min |
| Depends on | T09 |
| References | [consistency.md, P12](../docs/consistency.md#p12--restart-and-shutdown) |

## Overview

Cases:
- The service restarts: active reservations, counters and idempotency keys must be exactly as before, since all the state lives in the database.
- The service receives SIGTERM while requests are being processed: they finish before the database is closed.

## Technical acceptance criteria

- [ ] Nothing needed for correctness is kept only in memory.
- [ ] Shutdown order: stop accepting connections, finish in-flight requests (up to 10 s), stop the limits refresher, close the database.

## User acceptance criteria

The user is a client application.

- Given reservations exist, when the service restarts, then usage, the reservations and the replays of their keys are unchanged.
- Given a request is being processed, when the service is stopped, then that request still gets its response.

## Tests

Integration (`internal/repository/sqlite`, `internal/app`):
- [ ] `TestRepository_ReopenKeepsState` — close and reopen the file: usage, replay and release behave as before.
- [ ] `TestRun_DrainsInFlightRequestsOnShutdown`

Container: `TestContainer_RestartKeepsReservations` and `TestContainer_SigtermStopsGracefully` in T22.

## Test results

_Not run yet._
