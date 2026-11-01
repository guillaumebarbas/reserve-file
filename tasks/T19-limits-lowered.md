# T19 — Limits lowered below current usage

| | |
|---|---|
| Issue | [#19](https://github.com/guillaumebarbas/reserve-file/issues/19) |
| Branch | `feat/t19-limits-lowered` |
| Priority | P1 |
| Estimate | 15 min |
| Depends on | T06, T09 |
| References | [consistency.md, P7](../docs/consistency.md#p7--a-limit-drops-below-current-usage), [D11](../docs/decisions.md#d11--lowered-limits-never-revoke-reservations) |

## Overview

Cases:
- The account's storage limit is lowered below what is already reserved (a plan change, applied through `SEED_ACCOUNTS` at restart).
- `MAX_PARALLEL_CHUNKS` drops below the number of active reservations.

Nothing is revoked: existing reservations stay active, and new ones are refused until usage is back under the limit.

## Technical acceptance criteria

- [ ] `DecideReserve` refuses when `limit − reserved` is negative, without overflow.
- [ ] `available_bytes` never goes below 0; `reserved_bytes` and `active_reservations` may exceed their limits.
- [ ] Release works in this state and brings usage back down.

## User acceptance criteria

The user is a client application. Account `acc_small`.

- Given 90 MiB are reserved, when the limit is lowered to 50 MiB, then the reservations stay active, usage shows `available_bytes: 0`, a new reservation gets `409 insufficient_storage`, and releases work.
- Given 5 reservations are active, when `MAX_PARALLEL_CHUNKS` drops to 3, then new reservations get `409 too_many_active_reservations` until fewer than 3 are active.

## Tests

Unit (`internal/domain`):
- [ ] `TestDecideReserve` — adds: usage above a lowered limit; active reservations above a lowered maximum.

Integration (`internal/repository/sqlite`, `internal/handler`):
- [ ] `TestReserve_AfterStorageLimitLowered`
- [ ] `TestReserve_AfterMaxActiveLowered`
- [ ] `TestAPI_UsageAfterLimitLowered` — `available_bytes: 0`, `reserved_bytes` above `limit_bytes`.

## Test results

_Not run yet._
