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

- [x] `account.Reserve` answers a reservation already made with the same key first, before every admission rule (step 2). The repository loads that reservation by its key.
- [x] Same key and same `bytes`: the stored reservation is returned in its current state, with `200` and `Idempotent-Replayed: true`. Counters are unchanged.
- [x] A refused or failed attempt stores nothing, so the same key can succeed later.

## User acceptance criteria

The user is a client application. Account `small_account`.

- Given a reservation was created with key K, when the app retries with K and the same size, then it gets `200`, the same reservation ID and `Idempotent-Replayed: true`, and usage is unchanged.
- Given the app retries K ten times, then usage still shows a single reservation.
- Given K was refused because the account was full, when space becomes available and the app retries K, then it gets `201`.
- Given the app sends the same size with two different keys, then it gets two reservations.

## Tests

Unit (`internal/domain`):
- [x] `TestAccount_Reserve` — adds: an existing key with the same bytes gives `Replayed` with the stored reservation and unchanged counters, even when the account is full, every slot is used and the limit is unknown.

Integration (`internal/repository/sqlite`, `internal/handler`):
- [x] `TestReserve_RetryReturnsSameReservation` — the retry returns the stored reservation, one row, counters unchanged.
- [x] `TestReserve_RefusedAttemptIsNotRemembered` — the limit is raised between the two attempts.
- [x] `TestReserve_DifferentKeysCreateDifferentReservations`
- [x] `TestReserve_KeyLookupFailureWritesNothing` *(added)* — the lookup by key fails inside the transaction: `ErrServiceUnavailable`, nothing written.
- [x] `TestAPI_ReserveRetry` — `200`, `Idempotent-Replayed: true`, same ID, usage unchanged, over ten retries.

## Test results

Run on 2026-11-01.

- `gofmt -l .` prints nothing; `go vet ./...` passes.
- `go test ./...` (also with `-shuffle=on`): 84 tests and 120 subtests pass, none fail or skip (`app` 12, `domain` 4, `handler` 20, `limits` 18, `repository/sqlite` 25, `service` 5). Every package is at 100% on its own.
- `docker compose run --rm test`: passes with the race detector; coverage of `./internal/...` is 100.0%.
- Test first: the replay rows of `TestAccount_Reserve`, `TestReserve_RetryReturnsSameReservation` (the retry hit the `UNIQUE` constraint) and `TestAPI_ReserveRetry` (`201` without the header) failed first. Three tests passed at once:
  - `TestReserve_KeyLookupFailureWritesNothing` covers the error branch Go forced when the lookup was written: `repository/sqlite` coverage went from 97.7% to 100%.
  - `TestReserve_RefusedAttemptIsNotRemembered` and `TestReserve_DifferentKeysCreateDifferentReservations` prove behaviour T06 and the lookup already had. Each was run against a deliberately broken copy of the code, reverted afterwards: a repository that stores and commits refused attempts made the first fail (the retry was replayed), and a lookup that ignores the key made the second fail (the second key was replayed).
- Container, checked by hand on a separate Compose project, removed afterwards: 30 MiB on `small_account` gives `201` with `Location`; the retry with the same key gives `200`, `Idempotent-Replayed: true` and the same body; usage still shows 31457280 bytes and 1 active reservation.
- Not covered here: the same key with another size is replayed for now; T14 refuses it with `422 idempotency_key_reused`. The replay of a released reservation (T16) and the `UNIQUE` violation of two simultaneous requests (T15) come later.
