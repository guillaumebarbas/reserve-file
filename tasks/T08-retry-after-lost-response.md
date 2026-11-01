# T08 — Retry after a lost response

| | |
|---|---|
| Issue | [#8](https://github.com/guillaumebarbas/reserve-file/issues/8) |
| Branch | `feat/t08-retry-after-lost-response` |
| Priority | P0 |
| Estimate | 20 min |
| Depends on | T06 |
| References | [consistency.md, P2](../docs/consistency.md#p2--the-response-is-lost-and-the-client-retries), [api.md, Idempotency](../docs/api.md#idempotency), D7 |

## Overview

Cases:
- The reservation was committed but the client never got the response (timeout, network cut, crash after the commit). It retries with the same `Idempotency-Key` and must get the same reservation, without reserving again or using a second slot.
- The first attempt was refused because the account was full. A later retry with the same key is evaluated again: refusals are not stored.
- A client that sends a new key on each retry gets a new reservation each time. That is the client contract, documented in api.md.

## Technical acceptance criteria

- [ ] `account.Reserve` answers a reservation already made with the same key first, before every admission rule (step 2). The repository loads that reservation by its key.
- [ ] Same key and same `bytes`: the stored reservation is returned in its current state, with `200` and `Idempotent-Replayed: true`. Counters are unchanged.
- [ ] A refused or failed attempt stores nothing, so the same key can succeed later.

## User acceptance criteria

The user is a client application. Account `acc_small`.

- Given a reservation was created with key K, when the app retries with K and the same size, then it gets `200`, the same reservation ID and `Idempotent-Replayed: true`, and usage is unchanged.
- Given the app retries K ten times, then usage still shows a single reservation.
- Given K was refused because the account was full, when space becomes available and the app retries K, then it gets `201`.
- Given the app sends the same size with two different keys, then it gets two reservations.

## Tests

Unit (`internal/domain`):
- [ ] `TestAccount_Reserve` — adds: an existing key with the same bytes gives `Replay`.

Integration (`internal/repository/sqlite`, `internal/handler`):
- [ ] `TestReserve_RetryReturnsSameReservation`
- [ ] `TestReserve_RefusedAttemptIsNotRemembered` — the limit is raised between the two attempts.
- [ ] `TestReserve_DifferentKeysCreateDifferentReservations`
- [ ] `TestAPI_ReserveRetry` — `200`, `Idempotent-Replayed: true`, same ID, usage unchanged.

## Test results

_Not run yet._
