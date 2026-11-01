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

- [x] The replay path reads neither the storage limit, nor the slots, nor `MAX_PARALLEL_CHUNKS`. More precisely, the answer does not depend on them: the reserve transaction still loads the account row, which must exist (`404` otherwise), and the service still reads the cached limit, but `account.Reserve` answers a known key before any limit, slot or space rule. The tests below prove the answer is the same when each of them would refuse.
- [x] The replay of a released reservation returns `status: released` and does not change the counters.

## User acceptance criteria

The user is a client application. Account `small_account`.

- Given the reservation made with K was released, when the app retries K, then it gets `200` with the released reservation, and nothing is reserved.
- Given the account is full or has 5 active reservations, when the app retries K, then it gets `200` with its reservation.
- Given the limit source has never answered, when the app retries K, then it gets `200` with its reservation, while a new key gets `503 limit_unavailable`.

## Tests

Unit (`internal/domain`):
- [x] `TestAccount_Reserve` — adds replay cases: account full, all slots used, limit unknown, reservation released.

Integration (`internal/repository/sqlite`) *(added, so that each case is also proved with the real transaction, the stored rows and the invariants)*:
- [x] `TestReserve_ReplayOfReleasedReservation` — the retry returns the released reservation with its `released_at`; still one row, counters at zero.
- [x] `TestReserve_ReplayWhenAccountFull` — the retry is replayed while a new key gets `InsufficientStorageError`.
- [x] `TestReserve_ReplayWhenEverySlotIsUsed` — the retry is replayed while its own reservation fills the last slot; a new key gets `TooManyActiveReservationsError`.
- [x] `TestReserve_ReplayWhenLimitUnknown` — the retry is replayed while a new key gets `ErrLimitUnavailable`.

Integration (`internal/handler`):
- [x] `TestAPI_ReplayOfReleasedReservation` — `200`, `Idempotent-Replayed: true`, the body of the release response; usage at zero.
- [x] `TestAPI_ReplayWhenAccountFull` — table: no space left, and every slot used including its own. `200` with the original body, while a new key gets `409 insufficient_storage` or `409 too_many_active_reservations`.
- [x] `TestAPI_ReplayWhenLimitUnknown` — `200` with the original body, while a new key gets `503 limit_unavailable` with `Retry-After: 5`.

## Test results

Run on 2026-11-01.

- `gofmt -l .` prints nothing; `go vet ./...` passes.
- `go test ./...`: 122 tests and 169 subtests pass, none fail or skip.
- `docker compose run --rm test`: passes with the race detector; coverage of `./internal/...` is 100.0%.
- No production change: the behaviour existed since T08 (the key is looked up before every rule) and the T09 integration (the key lookup reads `released_at`). So every new test passed as soon as it was written. Each was then run against deliberate mutations of the code, reverted afterwards, and failed as expected:
  - **Mutation A**, in `account.Reserve`: the limit, slot and space rules moved before the key lookup.
    - the domain rows `replay_when_the_account_is_full`, `replay_when_every_slot_is_used_including_its_own` and `replay_when_the_limit_is_unknown` fail;
    - `TestReserve_ReplayWhenAccountFull`, `TestReserve_ReplayWhenEverySlotIsUsed` and `TestReserve_ReplayWhenLimitUnknown` fail with `insufficient storage`, `too many active reservations` and `limit unavailable`;
    - `TestAPI_ReplayWhenAccountFull` (both rows) gets `409` and `TestAPI_ReplayWhenLimitUnknown` gets `503`, without `Idempotent-Replayed`.
  - **Mutation B**, in `account.Reserve`: only an active reservation is replayed.
    - the domain row `replay_of_a_released_reservation` fails: it creates `reservation_1` and reserves 30 bytes;
    - `TestAPI_ReplayOfReleasedReservation` gets `503`, because the new row hits the `UNIQUE (account_id, idempotency_key)` constraint.
  - **Mutation C**, in `selectReservationByKey`: the lookup only finds active rows (`AND status = 'active'`). `TestReserve_ReplayOfReleasedReservation` fails on the `UNIQUE` constraint.
