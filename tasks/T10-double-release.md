# T10 — Double and concurrent release

| | |
|---|---|
| Issue | [#10](https://github.com/guillaumebarbas/reserve-file/issues/10) |
| Branch | `feat/t10-double-release` |
| Priority | P0 |
| Estimate | 15 min |
| Depends on | T09 |
| References | [consistency.md, P6](../docs/consistency.md#p6--double-release-and-concurrent-releases), [D10](../docs/decisions.md#d10--release-endpoint) |

## Overview

Case: the same reservation is released twice, by a retry or from two devices, possibly at the same instant. The space must come back exactly once; the later calls succeed without changing anything.

## Technical acceptance criteria

- [x] An already released reservation gives `200` with the unchanged body, `released_at` included.
- [x] `account.Release` on a released reservation changes nothing and returns `false`, so the counters are decremented only by the call that moved the reservation from `active` to `released`.
- [x] Concurrent releases and reservations keep invariants I1 and I2, also under the race detector.

## User acceptance criteria

The user is a client application. Account `small_account`.

- Given an active 30 MiB reservation, when the app releases it twice, then both calls return `200` with the same body, and usage shows 30 MiB less, once.
- Given 20 releases of the same reservation arrive at the same time, then usage shows 30 MiB less, once.

## Tests

Unit (`internal/domain`):
- [x] `TestAccount_ReleaseTwice` — the second call returns `false` and changes neither the reservation nor the counters. `TestAccount_Release` now also checks that the first call returns `true`.

Integration (`internal/repository/sqlite`, `internal/handler`):
- [x] `TestRelease_Twice` — the second call returns the same reservation, `released_at` of the first call included, without an error; the counters changed once. It replaces T09's `TestRelease_SecondReleaseGivesNothingBack` and keeps its second active reservation, without which a double credit would only show as a `CHECK` failure.
- [x] `TestRelease_ConcurrentReleasesCreditOnce` — 20 goroutines release the same reservation: all 20 succeed with the stored reservation, and the counters changed once.
- [x] `TestReserveRelease_ConcurrentMix` — at the same instant: 10 releases, the same 10 releases again, and 10 new reservations. All 30 succeed, and the account ends with exactly the 10 new reservations.
- [x] `TestRelease_RollsBackWhenTheRowIsNotUpdated` *(added)* — the one-row guard: a trigger with `RAISE(IGNORE)` makes the `UPDATE` change no row, the release fails, and the counters are untouched.
- [x] `TestAPI_ReleaseIdempotent` — `200` twice, same body; usage shows 30 MiB less, once.

## Test results

Run on 2026-11-01.

- `gofmt -l .` prints nothing; `go vet ./...` passes.
- `go test ./...` (also with `-shuffle=on`): 111 tests and 161 subtests pass, none fail or skip (`app` 12, `domain` 6, `handler` 31, `limits` 18, `repository/sqlite` 37, `service` 7). Every package is at 100% on its own.
- `docker compose run --rm test`: passes with the race detector; coverage of `./internal/...` is 100.0%.
- Failed first, on an assertion:
  - `TestAccount_ReleaseTwice`: the second call returned `true`, moved `released_at` forward and left the account at −10485760 bytes and 0 active.
  - With the domain fixed but the repository not yet using its result, the one-row guard refused every later call: `TestRelease_Twice` got "it is no longer active", `TestRelease_ConcurrentReleasesCreditOnce` got that error for 19 of its 20 calls, `TestReserveRelease_ConcurrentMix` for some of its repeated releases, and `TestAPI_ReleaseIdempotent` got `200` then `500 internal_error`. The counters were right in every case: the guard rolled the second transaction back.
- `TestRelease_RollsBackWhenTheRowIsNotUpdated` passed at once, because the guard already existed. Once a second release returns early, it is the only test that reaches the guard's error branch: without it, `repository/sqlite` coverage drops from 100% to 99.2%.
- The one-row guard stays. With `BEGIN IMMEDIATE`, the reservation is read under the write lock, so the `UPDATE … WHERE status = 'active'` cannot miss its row unless something changed it in between. The guard turns that impossible case into a rollback instead of a silent double credit, at the cost of one branch, tested with a trigger.
- Container, checked by hand on a separate Compose project with the real limit source, removed afterwards:
  - two reservations of 30 MiB on `small_account`, then the first one released twice: `200` twice with the same body, `released_at` included; usage went from 62914560 to 31457280 bytes and from 2 to 1 active reservation;
  - 20 releases of the second reservation sent at the same time with `curl`: twenty `200`; usage 0 bytes and 0 active.
