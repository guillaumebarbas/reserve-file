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
- [ ] `.gitignore` also covers `.env`.
- [ ] `go.mod` with module path `github.com/guillaumebarbas/reserve-file` and `go 1.27`.
- [ ] Only the packages needed now are created: `cmd/api`, `internal/app`, `internal/handler`.
- [ ] `cmd/api/main.go` sets up SIGINT/SIGTERM handling and calls `app.Run`, nothing else. Exit code 1 on error.
- [ ] `app.Run` reads `HTTP_ADDR` (default `:8080`) and `LOG_LEVEL` (default `info`), logs JSON with `log/slog`, serves HTTP, and on cancellation calls `http.Server.Shutdown` with a 10 s limit.
- [ ] `http.Server` sets `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout` and `IdleTimeout`; `WriteTimeout` is longer than the 5 s SQLite busy timeout, so a `503` sent after a lock wait is not cut off.
- [ ] `GET /healthz` returns `200` with `{"status":"ok"}`.
- [ ] An `api healthcheck` subcommand calls `/healthz` and exits with 0 or 1. Compose uses it, since distroless has no curl.
- [ ] Dockerfile: `golang:1.27` build stage with `CGO_ENABLED=0`; `gcr.io/distroless/static:nonroot` runtime; `/data` owned by `65532:65532`; `ENTRYPOINT` in exec form.
- [ ] `compose.yaml`:
  - `api`: build, host port `${API_PORT:-8080}` mapped to 8080, named volume `data` mounted on `/data`, health check. `API_PORT` exists because Windows can reserve port ranges (Hyper-V, WSL); a local `.env` file, ignored by git, overrides it;
  - `test` (profile `test`): `golang:1.27` with the source mounted, running `go vet ./...`, `go test -race -coverpkg=./internal/... -coverprofile=coverage.out ./...` and `scripts/check-coverage.sh coverage.out`.
- [ ] `scripts/check-coverage.sh` fails unless the total coverage is 100.0%.
- [ ] `gofmt -l .` prints nothing and `docker compose run --rm test` passes.

## User acceptance criteria

The user is the reviewer, who has only Docker.

- Given a fresh clone, when they run `docker compose up --build`, then the service starts and `curl -i localhost:8080/healthz` returns `200` with `{"status":"ok"}`.
- Given the service is running, when they run `docker compose stop`, then it stops within 10 seconds and its last log line says the server stopped.
- Given a fresh clone, when they run `docker compose run --rm test`, then the tests pass without Go installed on the host.

## Tests

Unit (`internal/app`):
- [ ] `TestLoadConfig_Defaults` — no variables set: `:8080` and `info`.
- [ ] `TestLoadConfig_InvalidValues` — table: unknown log level, malformed address. The error names the variable.

Integration (`internal/app`, `internal/handler`):
- [x] `TestHealthz` — `200`, JSON body, `Content-Type: application/json`.
- [ ] `TestRun_ServesUntilContextCancelled` — `Run` on `127.0.0.1:0` answers `/healthz`, then returns `nil` once the context is cancelled.
- [ ] `TestRun_FailsWhenAddressInUse` — returns an error instead of hanging.
- [ ] `TestHealthcheck_HealthyAndUnhealthy` — exit status 0 against a running server, 1 against a closed port.

Container: covered in T22 by `TestContainer_RunsAsNonRootAndWritesVolume`.

## Test results

_Not run yet._
