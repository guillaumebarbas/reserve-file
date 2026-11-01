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

- [ ] An already released reservation gives `200` with the unchanged body, `released_at` included.
- [ ] The counters are decremented only by the call that moved the row from `active` to `released`.
- [ ] Concurrent releases and reservations keep invariants I1 and I2, also under the race detector.

## User acceptance criteria

The user is a client application. Account `acc_small`.

- Given an active 30 MiB reservation, when the app releases it twice, then both calls return `200` with the same body, and usage shows 30 MiB less, once.
- Given 20 releases of the same reservation arrive at the same time, then usage shows 30 MiB less, once.

## Tests

Integration (`internal/repository/sqlite`, `internal/handler`):
- [ ] `TestRelease_Twice` — the second call returns the same reservation; the counters changed once.
- [ ] `TestRelease_ConcurrentReleasesCreditOnce` — 20 goroutines release the same reservation.
- [ ] `TestReserveRelease_ConcurrentMix` — interleaved reservations and releases.
- [ ] `TestAPI_ReleaseIdempotent` — `200` twice, same body.

## Test results

_Not run yet._
