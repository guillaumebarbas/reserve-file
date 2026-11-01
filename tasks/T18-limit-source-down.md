# T18 — Limit source down or invalid

| | |
|---|---|
| Issue | [#18](https://github.com/guillaumebarbas/reserve-file/issues/18) |
| Branch | `feat/t18-limit-source-down` |
| Priority | P1 |
| Estimate | 25 min |
| Depends on | T04, T06, T09 |
| References | [consistency.md, P8](../docs/consistency.md#p8--envhivenetcom-is-down-slow-or-returns-garbage), [D12](../docs/decisions.md#d12--external-limit-refreshed-in-the-background), [D13](../docs/decisions.md#d13--fail-closed-until-the-limit-is-known) |

## Overview

Cases:
- env.hivenet.com is down, slow, or answers garbage after a value was obtained: the last valid value stays in use.
- The service restarts during the outage: it reloads the last valid value from the database.
- The very first start happens during the outage: new reservations are refused with `503`.
- In every case, release, usage, reads and replays keep working: they never need the limit.

## Technical acceptance criteria

- [x] A failed fetch (error status, timeout, body over 1 MiB, invalid payload) keeps the current value and logs a warning without the body.
- [x] The persisted value is loaded before the first fetch. Done and proved in T04.
- [x] While no value is known, fetches are retried with a backoff from 1 s to 30 s.
- [x] Added: a save that failed is retried at the next refresh, so a restart during an outage reloads the last valid value.
- [x] New reservations get `503 limit_unavailable` with `Retry-After: 5`, only while no value has ever been obtained. Proved end to end in part 2.

## User acceptance criteria

The user is a client application, and the operator.

- Given the service uses 5, when env.hivenet.com goes down or starts answering `"5"` or `0`, then the service keeps using 5 and logs warnings.
- Given the service restarts during that outage, then it still uses 5.
- Given the first start happens during an outage, when the app reserves, then it gets `503 limit_unavailable` with `Retry-After: 5`; its usage request gets `200`.
- Given env.hivenet.com is down, when the app releases a reservation, then the release works.
- Given the source comes back, then new reservations are accepted without restarting the service.

## Tests

Integration (`internal/limits`, `internal/handler`, `internal/app`):
- [x] `TestFetcher_RejectsInvalidResponses` — written in T04: status 500 and 404, body over 1 MiB, invalid value. The slow source is `TestFetcher_TimesOut`, and every kind of invalid payload is a row of `TestParseMaxParallelChunks`, both from T04.
- [x] `TestLimits_KeepsLastValidValueOnFailure` — table, against a real source that first gives 5 then fails: status 500, body over 1 MiB, the value as a string, 0, a missing key. 5 stays in use and a warning is logged each time.
- [x] `TestLimits_KeepsLastValidValueWhenSourceTimesOut` *(added: a slow source needs its own handler)* — 5 stays in use when the next fetch times out.
- [x] `TestLimits_ReloadsPersistedValue` — written in T04: a new instance with the source down starts with the stored value.
- [x] `TestLimits_UnknownUntilFirstSuccess` — three failures, then the source answers: unknown after `Init`, 5 as soon as the source answers, with attempts 1, 2 and 4 s apart, then 60 s.
- [x] `TestLimits_RetriesWithBackoffWhileUnknown` *(added)* — with `testing/synctest`, two minutes of outage give attempts 1, 2, 4, 8, 16, 30 and 30 s apart.
- [x] `TestLimits_NeverLogsBody` — at debug level, neither an accepted payload nor a rejected one puts any part of the payload in the log (token, key name, URL).
- [x] `TestLimits_RetriesSaveAfterFailure` *(added)* — a save that fails is retried once by the next refresh, then the value is not written again.
- [x] `TestAPI_ReserveWhenLimitUnknown` — `503 limit_unavailable`, `Retry-After: 5` and the api.md detail; nothing is reserved.
- [x] `TestAPI_ReleaseWhenLimitUnknown` — a reservation made while the limit was known is released with `200` once it is unknown, and usage is back to zero.
- [x] `TestAPI_ReplayAndReadWhenLimitUnknown` *(added)* — with the limit unknown, a retry with the same key is replayed (`200`, `Idempotent-Replayed: true`, the same body), the reservation is read as created, and usage answers `200` with `max_active_reservations: null`.
- [x] `TestRun_FirstStartDuringOutage` *(added)* — `app.Run` starts while the source fails: `/readyz` answers `200` with `"limits":"unavailable"`, usage `200` with `max_active_reservations: null`, a reservation `503 limit_unavailable` with `Retry-After: 5`. Once the source answers a retry of the refresher, the same request gets `201`, without a restart.
- [x] `TestRun_KeepsReservingWhenSourceGoesDown` *(added)* — the source answers at startup, then fails: after two failed refreshes, a new reservation still gets `201`, `/readyz` still says `"limits":"ok"`, and a release answers `200`.
- [x] `TestRun_RestartDuringOutageKeepsTheLastLimit` *(added)* — a first run saves 5; a second run on the same database with the source unreachable accepts a reservation and reports `max_active_reservations: 5`.

## Test results

### Part 1: inside `internal/limits` (2026-11-01)

- `gofmt -l .` prints nothing; `go vet ./...` passes.
- `go test ./...` (also with `-shuffle=on`): 66 tests and 89 subtests pass, none fail or skip; `limits` has 18 tests.
- `docker compose run --rm test`: passes with the race detector; coverage of `./internal/...` is 100.0%.
- Two tests failed first on an assertion: `TestLimits_RetriesWithBackoffWhileUnknown` (attempts 60 s apart before the backoff) and `TestLimits_RetriesSaveAfterFailure` (one save attempt, no retry).
- Four tests passed as soon as they were written, because T04 or the backoff already gave the behaviour they pin; coverage stayed at 100% of `limits`. Each was then run against a deliberately broken copy of the code, reverted afterwards, and failed as it should:
  - `TestLimits_UnknownUntilFirstSuccess`, with a backoff that keeps going after a value is known: attempts 8, 16 and 30 s apart instead of 60 s;
  - `TestLimits_KeepsLastValidValueOnFailure` and `TestLimits_KeepsLastValidValueWhenSourceTimesOut`, with a refresh that forgets the value on failure: `MaxActive` gives 0, unknown;
  - `TestLimits_NeverLogsBody`, with a parsing error that quotes the payload: the token shows in the log.
- Backoff only while no value is known: with a valid value in use, a failed refresh waits the normal 60 s. The ticket, architecture.md and consistency.md P8 now say so.
- Remaining for part 2, once reserve (T06), retries (T08) and release (T09) exist: `503 limit_unavailable` with `Retry-After: 5` for new reservations only while no value has ever been obtained, and release, usage, reads and replays working with the limit unknown.

### Part 2: end to end (2026-11-01)

- `gofmt -l .` prints nothing; `go vet ./...` passes; no Go comments.
- `go test ./...` (also with `-shuffle=on`): 121 tests and 163 subtests pass, none fail or skip. The three `app` tests also passed five runs in a row; every package is at 100% on its own.
- `docker compose run --rm test`: passes with the race detector; coverage of `./internal/...` is 100.0%.
- No production change: the behaviour existed (T03 maps the error, T06 refuses with an unknown limit, T08 answers a replay first, T09 never asks for the limit, T04 and part 1 keep and reload the value). So the six tests passed as soon as they were written. Each was then run against a deliberately broken copy of the code, reverted afterwards, and failed as it should:
  - `TestAPI_ReserveWhenLimitUnknown`: without the unknown-limit check in `account.Reserve`, the request got `409 too_many_active_reservations` with a maximum of 0; with `RetryAfterSeconds: 0` for `limit_unavailable`, the `Retry-After` header was empty.
  - `TestAPI_ReleaseWhenLimitUnknown`: with a service that refuses a release while the limit is unknown, the release got `503`.
  - `TestAPI_ReplayAndReadWhenLimitUnknown`: with the limit checked before the stored reservation in `account.Reserve`, the replay got `503 limit_unavailable`; with a usage body that always sets the maximum, usage showed `max_active_reservations: 0` instead of `null`.
  - `TestRun_FirstStartDuringOutage`: with no backoff while the limit is unknown, the refresher waited the normal 60 s and the test gave up after 5 s waiting for the retry.
  - `TestRun_KeepsReservingWhenSourceGoesDown`: with a refresh that forgets the value on failure, the reservation got `503 limit_unavailable` and `/readyz` said `"limits":"unavailable"`.
  - `TestRun_RestartDuringOutageKeepsTheLastLimit`: with an `Init` that does not load the saved value, the reservation got `503` and usage `max_active_reservations: null`.
- The `app` tests never call the real endpoint: a switchable `httptest` source answers or fails, and signals on channels when it answered and when it failed, so the tests wait on events rather than on time. The only wait on a clock is `waitUntilLimitKnown`, which polls `/readyz` for the few microseconds between the source's answer and the cache storing it.
