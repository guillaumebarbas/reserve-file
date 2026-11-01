# T22 — Container tests

| | |
|---|---|
| Issue | [#22](https://github.com/guillaumebarbas/reserve-file/issues/22) |
| Branch | `chore/t22-container-tests` |
| Priority | P1 |
| Estimate | 40 min |
| Depends on | T11 |
| References | [testing.md](../docs/testing.md#test-types), [architecture.md](../docs/architecture.md#container), D18, D19 |

## Overview

Black-box tests of the packaged service. They build the image, start it with the same `compose.yaml` the reviewer uses, and talk to it over the network. They check what in-process tests cannot see: the non-root user and its volume, persistence across container restarts, durability after a kill, and a clean stop on SIGTERM. A stub container serves a fixed `MAX_PARALLEL_CHUNKS`, so the tests do not depend on env.hivenet.com.

## Technical acceptance criteria

- [x] Tests live in `e2e/`, behind the `e2e` build tag, and use only the standard library. They drive `docker compose` through `os/exec`.
- [x] `compose.e2e.yaml` adds an `env-stub` service (nginx serving `e2e/testdata/env.json` with `MAX_PARALLEL_CHUNKS: 5`), points `ENV_CONFIG_URL` at it, and publishes the API on port 18090, outside the ports a developer is likely to use for the main stack.
- [x] The Compose project is named `reserve-e2e`, so the developer's stack is untouched. `docker compose down -v` runs at the end, even after a failure.
- [x] Tests wait for `/readyz` instead of sleeping for a fixed time.
- [x] These tests are not part of the coverage metric.

## User acceptance criteria

The user is the developer or the reviewer, with Go and Docker installed.

- Given no stack is running, when they run `go test -tags e2e ./e2e/...`, then the image is built, the scenarios run against the container, and everything is removed afterwards.
- Given a reservation was created, when the container is restarted or killed with SIGKILL, then the reservation and usage are unchanged once it is back.
- Given the container receives SIGTERM, then it exits with code 0 within 10 seconds.

## Tests

Container (`e2e`):
- [x] `TestContainer_RunsAsNonRootAndWritesVolume` — the image user is `nonroot`, and a reservation is stored.
- [x] `TestContainer_ReserveRetryRelease` — the main flow over the network.
- [x] `TestContainer_ConcurrentBurstRespectsLimit` — 20 parallel requests.
- [x] `TestContainer_RestartKeepsReservations`
- [x] `TestContainer_SigkillKeepsCommittedReservations`
- [x] `TestContainer_SigtermStopsGracefully` — exit code 0, and the log says the server stopped.
- [x] `TestContainer_LimitSourceDownKeepsLastValue` — stub stopped, service restarted: reservations are still accepted.
- [x] `TestContainer_FirstStartWithoutLimitSource` — fresh volume, stub stopped: reserve gets `503 limit_unavailable`, usage gets `200`.

## Test results

Run on 2026-11-01, with `go test -tags e2e -count=1 -v ./e2e/...` on Windows with Docker Desktop (Compose v5.1.4):

```
--- PASS: TestContainer_RunsAsNonRootAndWritesVolume (48.36s)
--- PASS: TestContainer_ReserveRetryRelease (5.29s)
--- PASS: TestContainer_ConcurrentBurstRespectsLimit (5.07s)
--- PASS: TestContainer_RestartKeepsReservations (7.08s)
--- PASS: TestContainer_SigkillKeepsCommittedReservations (10.20s)
--- PASS: TestContainer_SigtermStopsGracefully (13.20s)
--- PASS: TestContainer_LimitSourceDownKeepsLastValue (13.26s)
--- PASS: TestContainer_FirstStartWithoutLimitSource (9.77s)
PASS
ok  	github.com/guillaumebarbas/reserve-file/e2e	113.309s
```

- The first test includes the image build. Each test starts from a fresh volume and removes its containers, volume and image in `t.Cleanup`; afterwards no container, volume or image of the `reserve-e2e` project was left.
- The tests check behaviour that already existed, so they passed on their first run. Two deliberate mutations, reverted afterwards, show that they fail when it breaks:
  - the stub serving `MAX_PARALLEL_CHUNKS: 2`: `TestContainer_ConcurrentBurstRespectsLimit` failed with `map[201:2 409:18]` instead of `map[201:3 409:17]`;
  - `stop_signal: SIGKILL` on the api service: `TestContainer_SigtermStopsGracefully` failed with exit code 137 and a last log line other than `server stopped`.
- Without the tag, `go list ./...` does not list `e2e`, so `go test ./...` and `docker compose run --rm test` (race detector, 100.0% of `./internal/...`) run as before.
- `compose.e2e.yaml` replaces the published port with `ports: !override`, so a host port 8080 reserved by Windows does not matter, and sets `SEED_ACCOUNTS` and `ENV_CONFIG_URL` explicitly, so a developer's `.env` cannot change the scenario.
