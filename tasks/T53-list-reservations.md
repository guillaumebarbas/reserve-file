# T53 — List active reservations

| | |
|---|---|
| Issue | [#53](https://github.com/guillaumebarbas/reserve-file/issues/53) |
| Branch | `feat/t53-list-reservations` |
| Priority | P2 |
| Estimate | 30 min |
| Depends on | T09 |
| References | [api.md](../docs/api.md#list-active-reservations), D21, gaps [G1 and G2](../docs/consistency.md#gaps-that-would-block-a-production-release) |

## Overview

`GET /accounts/{accountID}/reservations` lists the account's active reservations, oldest first. The brief does not ask for it. Reservations do not expire (G1), so a client that crashed and lost its reservation IDs holds their space and slots forever: this list lets it, or an operator, find them and release them.

Released reservations are not listed: they are history kept for idempotent replays, and listing them would need pagination and a retention window (G2). They can come later as `?status=released` with a cursor, without breaking this contract.

## Technical acceptance criteria

- [ ] `200` with `{"reservations": [...]}`: the active reservations only, oldest first (`created_at`, then `id`), each with the same body as `GET` of one reservation.
- [ ] No active reservation: `{"reservations": []}`, never `null`.
- [ ] Unknown account: `404 account_not_found`; database failure: `503 service_unavailable`, with nothing else to unwrap.
- [ ] Read-only: no transaction, no write, no call to `LimitProvider`. The `Account` aggregate is not loaded with its reservations (D5, D20).
- [ ] No schema change: the account filter uses the index of `UNIQUE (account_id, idempotency_key)`.
- [ ] `docs/api.md`, `api/openapi.yaml`, the README and decision D21 describe the endpoint.

## User acceptance criteria

The user is a client application. Account `small_account`.

- Given two active reservations and a released one, when the app lists the reservations, then it gets the two active ones, oldest first.
- Given no active reservation, when the app lists them, then it gets an empty list.
- Given an unknown account, when the app lists its reservations, then it gets `404 account_not_found`.

## Tests

Unit (`internal/service`):
- [x] `TestService_ListActiveReservationsChecksTheAccount` — an unknown account returns `ErrAccountNotFound` without reading the list; a known one returns what the repository listed.

Integration (`internal/repository/sqlite`, `internal/handler`):
- [x] `TestListActiveReservations_OnlyActiveOldestFirst` — released reservations and those of other accounts are left out; the order follows `created_at`, not the key, the ID or the insertion order.
- [x] `TestListActiveReservations_DatabaseUnavailable` — a closed database gives `ErrServiceUnavailable` and nothing else to unwrap.
- [x] `TestListActiveReservations_UnreadableRowFailsTheWholeList` — a row that cannot be read gives `ErrServiceUnavailable`, never a partial list.
- [ ] `TestAPI_ListActiveReservations` — `200` with the active reservations, oldest first, each with the body of a single read.
- [ ] `TestAPI_ListActiveReservationsEmpty` — `200` with `{"reservations":[]}`.
- [ ] `TestAPI_ListActiveReservationsUnknownAccount` — `404 account_not_found`.

## Test results

Not run yet.
