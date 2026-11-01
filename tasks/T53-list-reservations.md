# T53 — List active reservations

| | |
|---|---|
| Issue | [#53](https://github.com/guillaumebarbas/reserve-file/issues/53) |
| Branch | `feat/t53-list-reservations` |
| Priority | P2 |
| Estimate | 30 min |
| Depends on | T09 |
| References | [api.md](../docs/api.md#list-active-reservations), D21, gaps [G1 and G2](../docs/consistency.md#gaps-that-would-block-a-production-release) |

## Overview

`GET /accounts/{accountID}/reservations` lists the account's active reservations, oldest first. The brief does not ask for it. Reservations do not expire (G1), so a client that crashed and lost its reservation IDs holds their space and slots forever: this list lets it, or an operator, find them and release them.

Released reservations are not listed: they are history kept for idempotent replays, and listing them would need pagination and a retention window (G2). They can come later as `?status=released` with a cursor, without breaking this contract.

## Technical acceptance criteria

- [x] `200` with `{"reservations": [...]}`: the active reservations only, oldest first (`created_at`, then `id`), each with the same body as `GET` of one reservation.
- [x] No active reservation: `{"reservations": []}`, never `null`.
- [x] Unknown account: `404 account_not_found`; database failure: `503 service_unavailable`, with nothing else to unwrap.
- [x] Read-only: no transaction, no write, no call to `LimitProvider`. The `Account` aggregate is not loaded with its reservations (D5, D20).
- [x] No schema change: the account filter uses the index of `UNIQUE (account_id, idempotency_key)`.
- [x] `docs/api.md`, `api/openapi.yaml`, the README and decision D21 describe the endpoint.

## User acceptance criteria

The user is a client application. Account `small_account`.

- Given two active reservations and a released one, when the app lists the reservations, then it gets the two active ones, oldest first.
- Given no active reservation, when the app lists them, then it gets an empty list.
- Given an unknown account, when the app lists its reservations, then it gets `404 account_not_found`.

## Tests

Unit (`internal/service`):
- [x] `TestService_ListActiveReservationsChecksTheAccount` — an unknown account returns `ErrAccountNotFound` without reading the list; a known one returns what the repository listed.

Integration (`internal/repository/sqlite`, `internal/handler`):
- [x] `TestListActiveReservations_OnlyActiveOldestFirst` — released reservations and those of other accounts are left out; the order follows `created_at`, not the key, the ID or the insertion order.
- [x] `TestListActiveReservations_DatabaseUnavailable` — a closed database gives `ErrServiceUnavailable` and nothing else to unwrap.
- [x] `TestListActiveReservations_UnreadableRowFailsTheWholeList` — a row that cannot be read gives `ErrServiceUnavailable`, never a partial list.
- [x] `TestAPI_ListActiveReservations` — `200` with the active reservations, oldest first, each with the body of a single read.
- [x] `TestAPI_ListActiveReservationsEmpty` — `200` with `{"reservations":[]}`.
- [x] `TestAPI_ListActiveReservationsUnknownAccount` — `404 account_not_found`.
- [x] `TestListActiveReservations_ErrorOnTheSecondRowFailsTheWholeList` *(added in the review)* — an error raised by one row fails the whole query: `ErrServiceUnavailable`, never the rows before it.
- [x] `TestAPI_DatabaseClosedReturns503` *(case `list_active_reservations` added in the review)* — `503 service_unavailable` with `Retry-After` and the log line, as for the other endpoints.

## Test results

Run on 2026-11-01.

- `gofmt -l .` prints nothing; `go vet ./...` passes; no comment in the Go code.
- `go test ./...` (also with `-shuffle=on`): 168 tests and 200 subtests pass, none fail or skip; the eight tests of this ticket are new, and `TestAPI_DatabaseClosedReturns503` has one more case.
- `docker compose run --rm test`: passes with the race detector; coverage of `./internal/...` is 100.0%.
- `docker compose run --rm openapi-lint`: valid.
- Failed first on an assertion: `TestService_ListActiveReservationsChecksTheAccount` and `TestListActiveReservations_OnlyActiveOldestFirst` (the stubs returned nothing), `TestAPI_ListActiveReservations` (`405`: the path only accepted `POST`), `TestAPI_ListActiveReservationsEmpty` (`{"reservations":null}`: a nil slice encodes as `null`).
- Passed as soon as written, because Go had already forced the branches they cover: `TestListActiveReservations_DatabaseUnavailable`, `TestListActiveReservations_UnreadableRowFailsTheWholeList` and `TestAPI_ListActiveReservationsUnknownAccount`. Each one fails under a mutation of its branch: the driver error returned untranslated, an unreadable row skipped, or the service listing an unknown account.
- `TestListActiveReservations_OnlyActiveOldestFirst` fails under each mutation of the query: without `ORDER BY`, with `ORDER BY id`, without the status filter, without the account filter.
- `EXPLAIN QUERY PLAN` of the list: `SEARCH reservations USING INDEX sqlite_autoindex_reservations_2 (account_id=?)`, then `USE TEMP B-TREE FOR ORDER BY`.
- Container, checked by hand on the local Compose stack: `[]` on an empty `small_account`; two reservations listed oldest first; after releasing the first, only the second; `404 account_not_found` for an unknown account; both released at the end. Swagger UI serves the new operation.
- Review of #56, three findings:
  - The `503` of the list was proved only by composition: `TestAPI_DatabaseClosedReturns503` gained the `list_active_reservations` case, which fails if the handler ignores the service's error.
  - The `any` of `rowScanner` is kept with the author's agreement, recorded as an exception in `.claude/rules/go-style.md`: it repeats `database/sql`'s `Scan` and avoids a second copy of the column mapping.
  - The explicit `rows.Err()` check suggested by the review was not added. With `ORDER BY`, SQLite sorts every matching row in the first step, which the driver runs inside `QueryContext`, so an error raised by the second row surfaces there: `TestListActiveReservations_ErrorOnTheSecondRowFailsTheWholeList` fails when that branch returns the driver error untranslated, and still passes when the `rows.Err()` line returns `nil`. No test could reach an explicit branch, and the list is all or nothing.
