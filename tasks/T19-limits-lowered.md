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

- [x] `account.Reserve` refuses when `limit − reserved` is negative, without overflow.
- [x] `available_bytes` never goes below 0; `reserved_bytes` and `active_reservations` may exceed their limits.
- [x] Release works in this state and brings usage back down.

## User acceptance criteria

The user is a client application. Account `small_account`.

- Given 90 MiB are reserved, when the limit is lowered to 50 MiB, then the reservations stay active, usage shows `available_bytes: 0`, a new reservation gets `409 insufficient_storage`, and releases work.
- Given 5 reservations are active, when `MAX_PARALLEL_CHUNKS` drops to 3, then new reservations get `409 too_many_active_reservations` until fewer than 3 are active.

## Tests

Unit (`internal/domain`):
- [x] `TestAccount_Reserve` — adds `active_reservations_above_a_lowered_maximum`; `usage_above_a_lowered_limit` already existed since T06.

Integration (`internal/repository/sqlite`, `internal/handler`, `internal/app`):
- [x] `TestReserve_AfterStorageLimitLowered` — 90 MiB reserved, the store reopened with `small_account` lowered to 50 MiB: 1 MiB more is refused with `available_bytes` 0, nothing is revoked or written.
- [x] `TestRelease_BringsUsageBackUnderALoweredLimit` *(added)* — one release leaves 60 MiB, still refused; a second leaves 30 MiB, and the 20 MiB left fit exactly.
- [x] `TestReserve_AfterMaxActiveLowered` — 5 active reservations and `MAX_PARALLEL_CHUNKS` lowered to 3: refused with 5 active for 3 allowed, nothing written.
- [x] `TestRelease_BringsActiveReservationsBackUnderALoweredMaximum` *(added)* — still refused at exactly 3 active, accepted once 2 are left.
- [x] `TestAPI_UsageAfterLimitLowered` — `available_bytes: 0`, `reserved_bytes` above `limit_bytes`, and `409 insufficient_storage` with `available_bytes` 0.
- [x] `TestAPI_ReleaseAfterLimitLowered` *(added)* — releases answer `200` above the lowered limit, and a reservation fits again once usage is back under it.
- [x] `TestAPI_ReserveAfterMaxActiveLowered` *(added)* — usage shows 5 active for a maximum of 3, `409 too_many_active_reservations`, then releases and a new reservation.
- [x] `TestRun_AppliesALoweredLimitAtRestart` *(added)* — the operator's path: `Run` restarted on the same database with `SEED_ACCOUNTS` lowering `small_account` to 50 MiB.

## Test results

Run on 2026-11-01.

- `gofmt -l .` prints nothing; `go vet ./...` passes; no comments in the Go code apart from `//go:embed`.
- `go test ./...` (also with `-shuffle=on`): 123 tests and 164 subtests pass, none fail or skip (`app` 14, `domain` 6, `handler` 35, `limits` 18, `repository/sqlite` 43, `service` 7). Every package is at 100% on its own.
- `docker compose run --rm test`: passes with the race detector; coverage of `./internal/...` is 100.0%.
- No production change: the behaviour already existed. The seed updates only the limit (T02), `AvailableBytes` never goes below 0 (T05), the space check subtracts (T06), and release never looks at a limit (T09, T10). So every new test passed as soon as it was written, and each one was run against a deliberate mutation of the code, reverted afterwards. Each mutation made it fail:
  - the slot check refusing only at exactly the maximum (`==` instead of `>=`): the new domain row, `TestReserve_AfterMaxActiveLowered` and `TestAPI_ReserveAfterMaxActiveLowered` admit a sixth reservation;
  - the slot check letting one in at the maximum (`>` instead of `>=`): `TestRelease_BringsActiveReservationsBackUnderALoweredMaximum` admits a fourth at 3 for 3;
  - the seed resetting the counters at restart: `TestReserve_AfterStorageLimitLowered` sees 0 reserved, admits 1 MiB and breaks the invariants;
  - the space check against the limit alone, forgetting what is reserved: `TestReserve_AfterStorageLimitLowered` and `TestRelease_BringsUsageBackUnderALoweredLimit` admit 1 MiB above the lowered limit;
  - an exact fit refused (`>=` instead of `>` in the space check): `TestRelease_BringsUsageBackUnderALoweredLimit` and `TestAPI_ReleaseAfterLimitLowered` refuse the 20 MiB left;
  - `AvailableBytes` without `max(0, …)`: `TestAPI_UsageAfterLimitLowered` shows `available_bytes` at −41943040, in the usage and in the refusal;
  - the seed ignoring a changed limit (`ON CONFLICT DO NOTHING`): `TestRun_AppliesALoweredLimitAtRestart` keeps the 100 MiB limit and admits 1 MiB.
- Container, checked by hand on a separate Compose project, removed afterwards:
  - 3 × 30 MiB reserved on `small_account`;
  - the container recreated with `SEED_ACCOUNTS=large_account:10737418240,small_account:52428800`: usage shows `limit_bytes` 52428800, `reserved_bytes` 94371840, `available_bytes` 0 and 3 active; 1 MiB more gets `409 insufficient_storage` with `available_bytes` 0;
  - two releases answer `200`, leaving 30 MiB and 20 MiB available, then 20 MiB fit exactly (`201`).
