# T14 — Same key, different size

| | |
|---|---|
| Issue | [#14](https://github.com/guillaumebarbas/reserve-file/issues/14) |
| Branch | `feat/t14-key-reuse` |
| Priority | P1 |
| Estimate | 10 min |
| Depends on | T08 |
| References | [consistency.md, P4](../docs/consistency.md#p4--a-key-is-reused-for-a-different-request), D7 |

## Overview

Case: a client bug reuses a known `Idempotency-Key` with another size. Returning the stored reservation would silently hand back the wrong amount of space, so the request is refused.

## Technical acceptance criteria

- [x] Same key, different `bytes`: `422 idempotency_key_reused` with `original_bytes`. Nothing is written.

## User acceptance criteria

The user is a client application. Account `small_account`.

- Given K was used to reserve 30 MiB, when the app sends K with 31 MiB, then it gets `422` saying "This Idempotency-Key was already used to reserve 31457280 bytes. Use a new key for a new request.", and usage is unchanged.

## Tests

Unit (`internal/domain`):
- [x] `TestAccount_Reserve` — adds: an existing key with different bytes gives `IdempotencyKeyReusedError`, and the key reuse is reported before every admission rule (limit unknown, slots full, no space).

Integration (`internal/repository/sqlite`, `internal/handler`):
- [x] `TestReserve_KeyReuseWithDifferentBytes` — the error carries the original 31457280 bytes; one row, counters unchanged, invariants hold.
- [x] `TestAPI_KeyReuseWithDifferentBytes` — `422`, `original_bytes`, message; usage unchanged.

## Test results

Run on 2026-11-01.

- `gofmt -l .` prints nothing; `go vet ./...` passes.
- `go test ./...` (also with `-shuffle=on`): 108 tests and 163 subtests pass, none fail or skip; every package is at 100% on its own.
- `docker compose run --rm test`: passes with the race detector; coverage of `./internal/...` is 100.0%.
- Test first:
  - the two new rows of `TestAccount_Reserve` failed first: the existing reservation was replayed (outcome 2) whatever its size;
  - `TestReserve_KeyReuseWithDifferentBytes` passed at once, because the domain rule already existed. It failed with the rule removed ("got error <nil>, want idempotency key reused"), then passed again once restored;
  - `TestAPI_KeyReuseWithDifferentBytes` was written while the rule was removed and failed: the second request got `200` with the 30 MiB reservation. It passes with the rule.
- Container, checked by hand on a separate Compose project, removed afterwards: `small_account` reserves 30 MiB with a key (`201`), then the same key with 31 MiB gets `422 idempotency_key_reused` in `application/problem+json`, with `original_bytes: 31457280` and the api.md detail; usage stays at 31457280 bytes and 1 active reservation.
