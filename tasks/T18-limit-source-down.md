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

- [ ] A failed fetch (error status, timeout, body over 1 MiB, invalid payload) keeps the current value and logs a warning without the body.
- [ ] The persisted value is loaded before the first fetch.
- [ ] While no value is known, fetches are retried with a backoff from 1 s to 30 s.
- [ ] New reservations get `503 limit_unavailable` with `Retry-After: 5`, only while no value has ever been obtained.

## User acceptance criteria

The user is a client application, and the operator.

- Given the service uses 5, when env.hivenet.com goes down or starts answering `"5"` or `0`, then the service keeps using 5 and logs warnings.
- Given the service restarts during that outage, then it still uses 5.
- Given the first start happens during an outage, when the app reserves, then it gets `503 limit_unavailable` with `Retry-After: 5`; its usage request gets `200`.
- Given env.hivenet.com is down, when the app releases a reservation, then the release works.
- Given the source comes back, then new reservations are accepted without restarting the service.

## Tests

Integration (`internal/limits`, `internal/handler`):
- [ ] `TestFetcher_RejectsInvalidResponses` — table: status 500, response slower than the timeout, body over 1 MiB, invalid payloads.
- [ ] `TestLimits_KeepsLastValidValueOnFailure`
- [ ] `TestLimits_ReloadsPersistedValue` — a new instance with the source down starts with the stored value.
- [ ] `TestLimits_UnknownUntilFirstSuccess` — and known as soon as the source recovers.
- [ ] `TestLimits_NeverLogsBody` — the captured logs contain no token from the payload.
- [ ] `TestAPI_ReserveWhenLimitUnknown` — `503 limit_unavailable`, `Retry-After: 5`.
- [ ] `TestAPI_ReleaseWhenLimitUnknown` — `200`.

## Test results

_Not run yet._
