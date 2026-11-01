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

- [ ] Same key, different `bytes`: `422 idempotency_key_reused` with `original_bytes`. Nothing is written.

## User acceptance criteria

The user is a client application. Account `small_account`.

- Given K was used to reserve 30 MiB, when the app sends K with 31 MiB, then it gets `422` saying "This Idempotency-Key was already used to reserve 31457280 bytes. Use a new key for a new request.", and usage is unchanged.

## Tests

Unit (`internal/domain`):
- [ ] `TestAccount_Reserve` — adds: an existing key with different bytes gives `IdempotencyKeyReusedError`.

Integration (`internal/repository/sqlite`, `internal/handler`):
- [ ] `TestReserve_KeyReuseWithDifferentBytes`
- [ ] `TestAPI_KeyReuseWithDifferentBytes` — `422`, `original_bytes`, message.

## Test results

_Not run yet._
