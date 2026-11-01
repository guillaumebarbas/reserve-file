# T16 — Retry after the situation changed

| | |
|---|---|
| Issue | [#16](https://github.com/guillaumebarbas/reserve-file/issues/16) |
| Branch | `feat/t16-retry-after-change` |
| Priority | P1 |
| Estimate | 15 min |
| Depends on | T08, T09 |
| References | [consistency.md, P5](../docs/consistency.md#p5--a-retry-arrives-after-the-situation-changed) |

## Overview

Cases, when the retry of an accepted request arrives:
- the reservation has already been released;
- the account is now full, or all its slots are used, possibly by this very reservation;
- env.hivenet.com is down and the limit is unknown.

In every case the retry returns the original reservation, because the key is looked up before any other check. A released reservation is returned as released and never created again.

## Technical acceptance criteria

- [ ] The replay path reads neither the storage limit, nor the slots, nor `MAX_PARALLEL_CHUNKS`.
- [ ] The replay of a released reservation returns `status: released` and does not change the counters.

## User acceptance criteria

The user is a client application. Account `acc_small`.

- Given the reservation made with K was released, when the app retries K, then it gets `200` with the released reservation, and nothing is reserved.
- Given the account is full or has 5 active reservations, when the app retries K, then it gets `200` with its reservation.
- Given the limit source has never answered, when the app retries K, then it gets `200` with its reservation, while a new key gets `503 limit_unavailable`.

## Tests

Unit (`internal/domain`):
- [ ] `TestDecideReserve` — adds replay cases: account full, all slots used, limit unknown, reservation released.

Integration (`internal/handler`):
- [ ] `TestAPI_ReplayOfReleasedReservation`
- [ ] `TestAPI_ReplayWhenAccountFull`
- [ ] `TestAPI_ReplayWhenLimitUnknown`

## Test results

_Not run yet._
