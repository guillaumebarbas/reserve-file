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

- [x] Nothing needed for correctness is kept only in memory.
- [x] Shutdown order: stop accepting connections, finish in-flight requests (up to 10 s), stop the limits refresher, close the database. The refresher runs on the application context, so a signal stops it while the requests drain rather than after them; `app` waits for both, and the database is closed last, which is what correctness needs. When the server stops on its own, the order is exactly the listed one.

## User acceptance criteria

The user is a client application.

- Given reservations exist, when the service restarts, then usage, the reservations and the replays of their keys are unchanged.
- Given a request is being processed, when the service is stopped, then that request still gets its response.

## Tests

Integration (`internal/repository/sqlite`, `internal/app`):
- [x] `TestRepository_ReopenKeepsState` — close and reopen the file: the counters are the same, a retry with an active reservation's key is replayed, a retry with a released reservation's key is replayed as released with its `released_at`, the counters stay put, and a release after the restart gives the space back. Invariants hold.
- [x] `TestRun_DrainsInFlightRequestsOnShutdown` — a reservation is in flight in the real `app.Run` stack, its handler waiting for the body (`Expect: 100-continue`), and the limits refresher is in the middle of a 60 s fetch when the context is cancelled. The server closes an idle connection and refuses new ones, `Run` does not return, the request then completes with `201`, and `Run` returns `nil` within 5 s. The reservation is in the database afterwards.

Container: `TestContainer_RestartKeepsReservations` and `TestContainer_SigtermStopsGracefully` in T22.

## Test results

Run on 2026-11-01.

- `gofmt -l .` prints nothing; `go vet ./...` passes; no comment in the Go code apart from `//go:embed`.
- `go test ./...` (also with `-shuffle=on`): 108 tests and 161 subtests pass, none fail or skip (`app` 13, `domain` 5, `handler` 30, `limits` 18, `repository/sqlite` 35, `service` 7). `TestRun_DrainsInFlightRequestsOnShutdown` also passed 20 runs in a row.
- `docker compose run --rm test`: passes with the race detector; coverage of `./internal/...` is 100.0%, every package at 100% on its own.
- Both tests passed as soon as they were written: the state lives in the database since T02 to T09, and the shutdown order exists since T01 and T04. Coverage was already 100%, so each test was run against deliberate mutations of the code, reverted afterwards:
  - `TestRepository_ReopenKeepsState`:
    - a seed that resets the counters at startup: the counters read 0 and 0 after the restart, and the release after the restart breaks `CHECK (reserved_bytes >= 0)`;
    - a key lookup that never finds the stored reservation: the retry after the restart is not replayed and hits the `UNIQUE` constraint.
  - `TestRun_DrainsInFlightRequestsOnShutdown`:
    - `server.Close()` instead of `server.Shutdown`: the in-flight connection is aborted before its response;
    - the database closed as soon as the signal arrives: the in-flight reservation gets `503`;
    - a refresher that is never stopped: `Run` does not return within 5 s.
- The limit reload after a restart is already proved by T04's `TestLimits_ReloadsPersistedValue` and not duplicated.
- Container, checked by hand on a separate Compose project, removed afterwards:
  - 30 MiB reserved on `small_account`, then `docker compose restart api`: usage still shows 31457280 bytes and 1 active reservation, a retry with the same key gives `200` with `Idempotent-Replayed: true` and the same reservation, a read gives it `active`, and a release brings usage back to 0;
  - `max_active_reservations` is 5 right after the restart;
  - `docker compose stop`: the log shows `shutting down`, then `server stopped`, and the exit code is 0.
- The drain of a request in flight during a container stop is left to T22's `TestContainer_SigtermStopsGracefully`; here it is proved in-process by `TestRun_DrainsInFlightRequestsOnShutdown`.
