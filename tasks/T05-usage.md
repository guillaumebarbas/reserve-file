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

- [ ] `domain.AvailableBytes(limit, reserved)` returns `max(0, limit − reserved)`.
- [ ] The repository reads the account row in a single statement, without a transaction. An unknown account gives `ErrAccountNotFound`.
- [ ] The service adds `max_active_reservations` from `LimitProvider`, `null` when the limit is unknown.
- [ ] Response fields and names exactly as in api.md.
- [ ] `404 account_not_found`; a database failure gives `503 service_unavailable`.

## User acceptance criteria

The user is a client application.

- Given `acc_demo` with nothing reserved, when the app requests its usage, then it gets `limit_bytes: 10737418240`, `reserved_bytes: 0`, `available_bytes: 10737418240`, `active_reservations: 0` and `max_active_reservations: 5`.
- Given an unknown account `acc_x`, when the app requests its usage, then it gets `404` with "Account 'acc_x' does not exist."
- Given `MAX_PARALLEL_CHUNKS` has never been obtained, when the app requests usage, then `max_active_reservations` is `null` and the other fields are correct.

## Tests

Unit (`internal/domain`, `internal/service`):
- [ ] `TestAvailableBytes_NeverNegative` — table, including usage above the limit.
- [ ] `TestService_UsageCombinesAccountAndLimit` — with a known and with an unknown limit.

Integration (`internal/repository/sqlite`, `internal/handler`):
- [ ] `TestAccount_ReturnsCounters`
- [ ] `TestAccount_UnknownAccount`
- [ ] `TestAPI_Usage`
- [ ] `TestAPI_UsageUnknownAccount`
- [ ] `TestAPI_UsageWhenLimitUnknown`

## Test results

_Not run yet._
