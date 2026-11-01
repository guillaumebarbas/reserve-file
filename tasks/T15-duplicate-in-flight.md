# T15 — Same request twice at the same time

| | |
|---|---|
| Issue | [#15](https://github.com/guillaumebarbas/reserve-file/issues/15) |
| Branch | `feat/t15-duplicate-in-flight` |
| Priority | P1 |
| Estimate | 25 min |
| Depends on | T08 |
| References | [consistency.md, P3](../docs/consistency.md#p3--the-same-request-arrives-twice-at-the-same-time), [D9](../docs/decisions.md#d9--in-flight-duplicates-get-409) |

## Overview

Case: a double click, or a client retrying while its first request is still being processed.

Before this ticket, the second request waits for the database lock and then gets the replay: correct, but blocked requests pile up during retry storms. With it, the second request is told at once to try again (`409 request_in_progress`), as the IETF draft specifies, and its next retry gets the replay.

## Technical acceptance criteria

- [ ] In-flight guard in `service`, keyed by `(account_id, key)`, checked before any database access.
- [ ] A second request with the same pair gets `409 request_in_progress` with `Retry-After: 1`.
- [ ] The entry is released with `defer`, whether the request succeeds, fails or panics.
- [ ] A unique-constraint violation on `(account_id, idempotency_key)` is translated to `ErrRequestInProgress`.
- [ ] Release does not go through the guard.

## User acceptance criteria

The user is a client application. Account `acc_small`.

- Given a request with key K is still being processed, when another request with K arrives, then it gets `409 request_in_progress` with `Retry-After: 1`; retrying a moment later returns the reservation.
- Given 50 requests with the same key arrive at once, then exactly one reservation exists; one response is `201` and the others are `200` or `409`.
- Given two requests with different keys, or with the same key on two accounts, arrive at once, then neither blocks the other.

## Tests

Unit (`internal/service`):
- [ ] `TestService_DuplicateInFlightReturnsInProgress` — the fake repository is blocked on a channel.
- [ ] `TestService_DifferentKeysAreNotBlocked`
- [ ] `TestService_SameKeyOnDifferentAccountsIsNotBlocked`
- [ ] `TestService_GuardReleasedAfterError`
- [ ] `TestService_GuardReleasedAfterPanic`

Integration (`internal/repository/sqlite`, `internal/handler`):
- [ ] `TestReserve_ConcurrentSameKeyCreatesOneRow` — 50 goroutines, no guard at this layer: 1 `Create`, 49 `Replay`, 1 row. Expected to pass already: it shows that the database alone prevents a duplicate.
- [ ] `TestReserve_UniqueViolationMapsToInProgress` — a trigger created by the test inserts the same key first.
- [ ] `TestAPI_ConcurrentSameKey`

## Test results

_Not run yet._
