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

- [ ] `internal/domain/errors.go` defines the typed errors `InsufficientStorageError`, `TooManyActiveError` and `KeyReusedError`, which carry their values, and the sentinels `ErrAccountNotFound`, `ErrReservationNotFound`, `ErrRequestInProgress`, `ErrLimitUnavailable` and `ErrUnavailable`.
- [ ] Middleware:
  - request ID (16 random hex characters), returned in `X-Request-Id` and attached to every log line;
  - access log: method, path, status, duration;
  - panic recovery, answering `500 internal_error`.
- [ ] `writeError` maps every error of the api.md catalogue to its status, `code`, `detail`, extension fields and `Retry-After`. An unknown error gives `500 internal_error`; the error is logged and never returned.
- [ ] `decodeJSON` enforces 4 KiB (`413 request_too_large`) and rejects unknown fields, trailing data and malformed JSON (`400 invalid_request`).
- [ ] `detail` texts use human units (`5.00 GiB`, `30.00 MiB`).
- [ ] Responses set `Content-Type` before the status; errors use `application/problem+json`.
- [ ] `GET /readyz` returns `200` with `{"database":"ok"}` when the database answers a ping, and `503` otherwise.

## User acceptance criteria

The user is a client application calling the API.

- Given any request, when the response arrives, then it carries an `X-Request-Id` header.
- Given a request that fails, when the response arrives, then its body is `application/problem+json` with `code`, `detail` and `request_id`, and the same `request_id` appears in the server log.
- Given a bug makes a handler panic, when the request is processed, then the client gets `500 internal_error` with a reference, and the following requests are served normally.
- Given the database is down, when `/readyz` is called, then it returns `503`.

## Tests

Unit (`internal/domain`, `internal/handler`):
- [ ] `TestErrors_Messages` — each typed error and sentinel has a readable `Error()` text and works with `errors.Is` / `errors.As`.
- [ ] `TestWriteError_Catalogue` — table over every code in api.md: status, `code`, extension fields, `Retry-After`.
- [ ] `TestWriteError_UnknownErrorHidesDetails` — `500`, no internal text in the body, error logged with the request ID.
- [ ] `TestDecodeJSON` — table: valid body, malformed JSON, unknown field, trailing data, empty body, body over 4 KiB.
- [ ] `TestFormatBytes` — bytes, KiB, MiB and GiB boundaries.
- [ ] `TestRequestID_SetOnEveryResponse`
- [ ] `TestRecovery_Returns500AndKeepsServing`
- [ ] `TestWriteJSON_ClientGone` — a `ResponseWriter` whose `Write` fails: the failure is logged, nothing panics.

Integration (`internal/handler`):
- [ ] `TestAPI_Readyz` — `200` with the database open, `503` once it is closed.

## Test results

_Not run yet._
