# T05 — Account usage

| | |
|---|---|
| Issue | [#5](https://github.com/guillaumebarbas/reserve-file/issues/5) |
| Branch | `feat/t05-usage` |
| Priority | P0 |
| Estimate | 15 min |
| Depends on | T04 |
| References | [api.md, Usage](../docs/api.md#usage), D5, D11 |

## Overview

`GET /accounts/{accountID}/usage` returns the account's limit, reserved space and available space, and its active reservations against `MAX_PARALLEL_CHUNKS`. It is the first slice through every layer (handler, service, repository, domain), and the main way to observe the effect of the next tickets.

## Technical acceptance criteria

- [x] `domain.Account` is the aggregate ([D20](../docs/decisions.md#d20--a-rich-account-aggregate-with-private-counters)): unexported counters, `RestoreAccount` to rebuild it from a row, the accessors `ReservedBytes()` and `ActiveReservations()`, and `AvailableBytes()`, which returns `max(0, limit − reserved)`.
- [x] The repository reads the account row in a single statement, without a transaction, and returns it as a `domain.Account`. An unknown account gives `ErrAccountNotFound`.
- [x] The service adds `max_active_reservations` from `LimitProvider`, `null` when the limit is unknown.
- [x] Response fields and names exactly as in api.md.
- [x] `404 account_not_found`; a database failure gives `503 service_unavailable`.

## User acceptance criteria

The user is a client application.

- Given `large_account` with nothing reserved, when the app requests its usage, then it gets `limit_bytes: 10737418240`, `reserved_bytes: 0`, `available_bytes: 10737418240`, `active_reservations: 0` and `max_active_reservations: 5`.
- Given an unknown account `unknown_account`, when the app requests its usage, then it gets `404` with "Account 'unknown_account' does not exist."
- Given `MAX_PARALLEL_CHUNKS` has never been obtained, when the app requests usage, then `max_active_reservations` is `null` and the other fields are correct.

## Tests

Unit (`internal/domain`, `internal/service`):
- [x] `TestAccount_AvailableBytesNeverNegative` — table, including usage above the limit.
- [x] `TestRestoreAccount_KeepsCounters` *(added)* — the accessors return the restored counters.
- [x] `TestService_UsageCombinesAccountAndLimit` — with a known and with an unknown limit.
- [x] `TestService_UsageReturnsAccountErrors` *(added)* — the repository's error reaches the caller.

Integration (`internal/repository/sqlite`, `internal/handler`, `internal/app`):
- [x] `TestAccount_ReturnsCounters`
- [x] `TestAccount_UnknownAccount`
- [x] `TestAccount_DatabaseUnavailable` *(added)* — a closed database gives an error that wraps `domain.ErrServiceUnavailable` and nothing else.
- [x] `TestAPI_Usage`
- [x] `TestAPI_UsageUnknownAccount`
- [x] `TestAPI_UsageWhenLimitUnknown`
- [x] `TestRun_ServesAccountUsage` *(added)* — the usage route is wired through `app.Run`, with the default `large_account`.

## Test results

Run on 2026-11-01.

- `gofmt -l .` prints nothing; `go vet ./...` passes.
- `go test ./...` (also with `-shuffle=on`): 42 tests and 61 subtests pass, none fail or skip (`app` 10, `domain` 3, `handler` 14, `repository/sqlite` 13, `service` 2).
- `docker compose run --rm test`: passes with the race detector; coverage of `./internal/...` is 100.0%.
- Every test failed first on an assertion, except `TestService_UsageReturnsAccountErrors`, which passed at once because Go forces the error branch it covers: `service` coverage went from 83.3% to 100% with it.
- Container, checked by hand on a separate Compose project, removed afterwards: `GET /accounts/large_account/usage` answers `200` with `limit_bytes` and `available_bytes` at 10737418240 and `max_active_reservations: null`; `GET /accounts/unknown_account/usage` answers `404 account_not_found` in `application/problem+json`.
- `max_active_reservations` stays `null` in the running service until T04's limits cache replaces the `limitNotLoadedYet` placeholder in `internal/app`.
