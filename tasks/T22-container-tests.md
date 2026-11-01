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

- [ ] Tests live in `e2e/`, behind the `e2e` build tag, and use only the standard library. They drive `docker compose` through `os/exec`.
- [ ] `compose.e2e.yaml` adds an `env-stub` service (nginx serving `e2e/testdata/env.json` with `MAX_PARALLEL_CHUNKS: 5`), points `ENV_CONFIG_URL` at it, and publishes the API on port 18080.
- [ ] The Compose project is named `reserve-e2e`, so the developer's stack is untouched. `docker compose down -v` runs at the end, even after a failure.
- [ ] Tests wait for `/readyz` instead of sleeping for a fixed time.
- [ ] These tests are not part of the coverage metric.

## User acceptance criteria

The user is the developer or the reviewer, with Go and Docker installed.

- Given no stack is running, when they run `go test -tags e2e ./e2e/...`, then the image is built, the scenarios run against the container, and everything is removed afterwards.
- Given a reservation was created, when the container is restarted or killed with SIGKILL, then the reservation and usage are unchanged once it is back.
- Given the container receives SIGTERM, then it exits with code 0 within 10 seconds.

## Tests

Container (`e2e`):
- [ ] `TestContainer_RunsAsNonRootAndWritesVolume` — the image user is `nonroot`, and a reservation is stored.
- [ ] `TestContainer_ReserveRetryRelease` — the main flow over the network.
- [ ] `TestContainer_ConcurrentBurstRespectsLimit` — 20 parallel requests.
- [ ] `TestContainer_RestartKeepsReservations`
- [ ] `TestContainer_SigkillKeepsCommittedReservations`
- [ ] `TestContainer_SigtermStopsGracefully` — exit code 0, and the log says the server stopped.
- [ ] `TestContainer_LimitSourceDownKeepsLastValue` — stub stopped, service restarted: reservations are still accepted.
- [ ] `TestContainer_FirstStartWithoutLimitSource` — fresh volume, stub stopped: reserve gets `503 limit_unavailable`, usage gets `200`.

## Test results

_Not run yet._
