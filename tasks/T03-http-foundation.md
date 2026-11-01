# T03 — HTTP foundation and error format

| | |
|---|---|
| Issue | [#3](https://github.com/guillaumebarbas/reserve-file/issues/3) |
| Branch | `feat/t03-http-foundation` |
| Priority | P0 |
| Estimate | 30 min |
| Depends on | T01 |
| References | [api.md](../docs/api.md#errors) (conventions, errors), [architecture.md](../docs/architecture.md#errors-across-layers), D14 |

## Overview

Everything the endpoints share: the router, request IDs, access logs, panic recovery, strict JSON decoding, and a single place that turns errors into RFC 9457 problem details. The domain error types are defined here, because the error mapping needs all of them.

## Technical acceptance criteria

- [x] `internal/domain/errors.go` defines the typed errors `InsufficientStorageError`, `TooManyActiveReservationsError` and `IdempotencyKeyReusedError`, which carry their values, and the sentinels `ErrAccountNotFound`, `ErrReservationNotFound`, `ErrRequestInProgress`, `ErrLimitUnavailable` and `ErrServiceUnavailable`.
- [x] Middleware:
  - request ID (16 random hex characters), returned in `X-Request-Id` and attached to every log line;
  - access log: method, path, status, duration;
  - panic recovery, answering `500 internal_error`.
- [x] `writeError` maps every error of the api.md catalogue to its status, `code`, `detail`, extension fields and `Retry-After`. An unknown error gives `500 internal_error`; the error is logged and never returned.
- [x] `decodeJSON` enforces 4 KiB (`413 request_too_large`) and rejects unknown fields, trailing data and malformed JSON (`400 invalid_request`).
- [x] `detail` texts use human units (`5.00 GiB`, `30.00 MiB`).
- [x] Responses set `Content-Type` before the status; errors use `application/problem+json`.
- [x] `GET /readyz` returns `200` with `{"database":"ok"}` when the database answers a ping, and `503` otherwise.
- [x] Added at the user's request: every name is descriptive, in full words. The seeded accounts become `large_account` and `small_account`, and the existing code is renamed (`go-style.md`).

## User acceptance criteria

The user is a client application calling the API.

- Given any request, when the response arrives, then it carries an `X-Request-Id` header.
- Given a request that fails, when the response arrives, then its body is `application/problem+json` with `code`, `detail` and `request_id`, and the same `request_id` appears in the server log.
- Given a bug makes a handler panic, when the request is processed, then the client gets `500 internal_error` with a reference, and the following requests are served normally.
- Given the database is down, when `/readyz` is called, then it returns `503`.

## Tests

Unit (`internal/domain`, `internal/handler`, `internal/app`):
- [x] `TestErrors_Messages` — each typed error and sentinel has a readable `Error()` text and is still found by `errors.Is` once wrapped. `errors.As` on the typed errors is exercised by `TestWriteError_Catalogue`.
- [x] `TestWriteError_Catalogue` — table over the api.md codes, each error wrapped as a caller would: status, `type`, `title`, `code`, `detail`, extension fields (`available_bytes` included when it is 0), `Retry-After`, `request_id`; `5xx` errors are logged, `4xx` are not. `invalid_idempotency_key` takes the same path as `invalid_request` and gets its constructor with the header check (T06).
- [x] `TestWriteError_UnknownErrorHidesDetails` — `500`, no internal text in the body, error logged with the request ID.
- [x] `TestDecodeJSON_ValidBody` and `TestDecodeJSON_RejectsInvalidBodies` *(split, since the valid case checks the value and the others the error)* — malformed JSON, unknown field, trailing data, empty body, body over 4 KiB.
- [x] `TestFormatBytes` — bytes, KiB, MiB and GiB boundaries.
- [x] `TestRequestID_SetOnEveryResponse` — 16 hexadecimal characters, a new one per request, also on a `404`.
- [x] `TestAccessLog_LogsEveryRequest` *(added: the access log criterion had no test)* — one JSON line with `request_id`, method, path, status and duration.
- [x] `TestRecovery_Returns500AndKeepsServing` — the panic is logged with its stack trace.
- [x] `TestWriteJSON_ClientGone` — a `ResponseWriter` whose `Write` fails: the failure is logged with the request ID, nothing panics, and `Content-Type` was set before the status.
- [x] `TestLoadConfig_Defaults` *(updated)* — the default accounts are `large_account` and `small_account`.

Integration (`internal/handler`, `internal/repository/sqlite`):
- [x] `TestAPI_Readyz` — `200 {"database":"ok"}` with the database open, `503 {"database":"unavailable"}` once it is closed, and the failure logged.
- [x] `TestStore_Ping` *(added)* — `nil` on an open database; once closed, an error that wraps `domain.ErrServiceUnavailable` and nothing else.

## Test results

Run on 2026-11-01.

- `gofmt -l .` prints nothing; `go vet ./...` passes.
- `go test ./...` (also with `-shuffle=on`): 31 tests and 55 subtests pass, none fail or skip (`app` 9, `domain` 1, `handler` 11, `repository/sqlite` 10).
- `docker compose run --rm test`: passes with the race detector; coverage of `./internal/...` is 100.0%.
- Every test failed first, on an assertion, before the code that makes it pass.
- Container, checked by hand on a separate Compose project, removed afterwards: `/healthz` and `/readyz` answer `200`, an unknown path `404` and a wrong method `405` with `Allow`. Every response carries `X-Request-Id`, and each access log line carries the same ID. Docker's health check also writes an access log line every 10 s.
- The `too_many_active_reservations` detail now reads "You have 7 active reservations and the maximum is 5.", which stays true after a limit is lowered; api.md follows.
