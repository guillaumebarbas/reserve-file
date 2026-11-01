# T01 — Scaffold and walking skeleton

| | |
|---|---|
| Issue | [#1](https://github.com/guillaumebarbas/reserve-file/issues/1) |
| Branch | `chore/t01-scaffold` |
| Priority | P0 |
| Estimate | 30 min |
| Depends on | — |
| References | [architecture.md](../docs/architecture.md) (layout, lifecycle, container), [D1](../docs/decisions.md#d1--standard-library-for-http-logs-and-configuration), [D18](../docs/decisions.md#d18--docker-compose-delivery), [D19](../docs/decisions.md#d19--unit-integration-and-container-tests) |

## Overview

Create the repository and a service that does almost nothing but goes through the whole chain: Go module, package layout, a binary that serves `/healthz` and stops cleanly, the Docker image, `compose.yaml`, and the test pipeline with the race detector and the 100% coverage check. Every later ticket plugs into this skeleton, and Docker problems (volume permissions, signals) show up now instead of at the end.

## Technical acceptance criteria

- [x] Git repository and `.gitignore` (coverage profile, SQLite files, `/bin`, local Claude Code settings, the exercise brief): done when the repository was bootstrapped.
- [x] `.gitignore` also covers `.env`.
- [x] `go.mod` with module path `github.com/guillaumebarbas/reserve-file` and `go 1.27`.
- [x] Only the packages needed now are created: `cmd/api`, `internal/app`, `internal/handler`.
- [x] `cmd/api/main.go` sets up SIGINT/SIGTERM handling and calls `app.Run`, nothing else. Exit code 1 on error.
- [x] `app.Run` reads `HTTP_ADDR` (default `:8080`) and `LOG_LEVEL` (default `info`), logs JSON with `log/slog`, serves HTTP, and on cancellation calls `http.Server.Shutdown` with a 10 s limit.
- [x] `http.Server` sets `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout` and `IdleTimeout`; `WriteTimeout` is longer than the 5 s SQLite busy timeout, so a `503` sent after a lock wait is not cut off.
- [x] `GET /healthz` returns `200` with `{"status":"ok"}`.
- [x] An `api healthcheck` subcommand calls `/healthz` and exits with 0 or 1. Compose uses it, since distroless has no curl.
- [x] Dockerfile: `golang:1.27` build stage with `CGO_ENABLED=0`; `gcr.io/distroless/static:nonroot` runtime; `/data` owned by `65532:65532`; `ENTRYPOINT` in exec form.
- [x] `compose.yaml`:
  - `api`: build, host port `${API_PORT:-8080}` mapped to 8080, named volume `data` mounted on `/data`, health check. `API_PORT` exists because Windows can reserve port ranges (Hyper-V, WSL); a local `.env` file, ignored by git, overrides it;
  - `test` (profile `test`): `golang:1.27` with the source mounted, running `go vet ./...`, `go test -race -coverpkg=./internal/... -coverprofile=coverage.out ./...` and `scripts/check-coverage.sh coverage.out`.
- [x] `scripts/check-coverage.sh` fails unless the total coverage is 100.0%.
- [x] `gofmt -l .` prints nothing and `docker compose run --rm test` passes.

## User acceptance criteria

The user is the reviewer, who has only Docker.

- Given a fresh clone, when they run `docker compose up --build`, then the service starts and `curl -i localhost:8080/healthz` returns `200` with `{"status":"ok"}`.
- Given the service is running, when they run `docker compose stop`, then it stops within 10 seconds and its last log line says the server stopped.
- Given a fresh clone, when they run `docker compose run --rm test`, then the tests pass without Go installed on the host.

## Tests

Unit (`internal/app`):
- [x] `TestLoadConfig_Defaults` — no variables set: `:8080` and `info`.
- [x] `TestLoadConfig_InvalidValues` — table: unknown log level, malformed address. The error names the variable.

Integration (`internal/app`, `internal/handler`):
- [x] `TestHealthz` — `200`, JSON body, `Content-Type: application/json`.
- [x] `TestRun_ServesUntilContextCancelled` — `Run` on `127.0.0.1:0` answers `/healthz`, then returns `nil` once the context is cancelled.
- [x] `TestRun_FailsWhenAddressInUse` — returns an error instead of hanging.
- [x] `TestRun_FailsOnInvalidConfiguration` — added during implementation: an invalid variable stops `Run` with an error naming it, instead of starting.
- [x] `TestHealthcheck_HealthyAndUnhealthy` — no error against a running server; an error against a closed port, and against a server answering `503` (case added during implementation, for the status check). `main` turns the error into exit status 1.

Container: covered in T22 by `TestContainer_RunsAsNonRootAndWritesVolume`.

## Test results

Run on 2026-11-01, on Windows 11 with Go 1.27.3 and Docker 29.4.3.

| Check | Result |
|---|---|
| `go test ./...` | 7 tests and 2 subtests pass |
| `gofmt -l .`, `go vet ./...` | Nothing reported |
| `docker compose run --rm test` | Pass with the race detector; coverage of `./internal/...`: 100.0% |
| `docker compose up --build -d --wait` | Healthy after about 5 s; `curl -i localhost:18080/healthz` returns `200`, `application/json`, `{"status":"ok"}` |
| `docker compose stop` | Stopped in 0.4 s with exit code 0; last log line `server stopped` |
| Image | 6.9 MB, user `nonroot:nonroot`; `/data` owned by `65532:65532` in the named volume |
| Exit status, checked by hand on the binary | 1 for an invalid `LOG_LEVEL` and for a healthcheck against a closed port, with the error logged as JSON |

Notes:
- The host port was 18080, set in a local `.env`: Windows reserves 8067–8666 on this machine. The default stays 8080.
- `TestRun_FailsWhenAddressInUse` and `TestRun_FailsOnInvalidConfiguration` passed at their first run, because Go forced both errors to be handled in the previous cycle. Coverage shows they exercise exactly those branches: `Run` went from 80% to 100%, `serve` from 95.5% to 100%.
- Two cases were added to the planned tests, both needed for 100% coverage: `TestRun_FailsOnInvalidConfiguration`, and the `503` case of the healthcheck test.
- If `Serve` stops on its own, `Run` shuts down and returns its error. No test can provoke it, so the path is covered by the normal shutdown but not asserted.
- An unknown subcommand is ignored and the service starts.
- Checked from the working tree, not from a fresh clone: the fresh-clone check belongs to T12.
