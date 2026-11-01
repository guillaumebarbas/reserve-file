# T17 — Release of an unknown or foreign reservation

| | |
|---|---|
| Issue | [#17](https://github.com/guillaumebarbas/reserve-file/issues/17) |
| Branch | `feat/t17-release-not-found` |
| Priority | P1 |
| Estimate | 10 min |
| Depends on | T09 |
| References | [consistency.md, P6](../docs/consistency.md#p6--double-release-and-concurrent-releases), [api.md, Errors](../docs/api.md#errors) |

## Overview

Case: the app releases or reads a reservation ID that does not exist, or that belongs to another account. Both get the same `404`, so the service never reveals that a reservation exists in another account.

## Technical acceptance criteria

- [ ] An unknown ID, a malformed ID, or the ID of another account's reservation gives `404 reservation_not_found`, with the same body shape in every case.
- [ ] Nothing is written.

## User acceptance criteria

The user is a client application.

- Given a reservation of `large_account`, when the app releases or reads it through `small_account`, then it gets `404`, exactly as for an ID that does not exist.

## Tests

Integration (`internal/repository/sqlite`, `internal/handler`):
- [ ] `TestRelease_UnknownReservation`
- [ ] `TestRelease_OtherAccountIsNotFound`
- [ ] `TestGet_OtherAccountIsNotFound`
- [ ] `TestAPI_ReleaseUnknownAndForeignLookTheSame` — same status and `code` for both.

## Test results

_Not run yet._
