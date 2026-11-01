# T13 — Invalid input

| | |
|---|---|
| Issue | [#13](https://github.com/guillaumebarbas/reserve-file/issues/13) |
| Branch | `feat/t13-invalid-input` |
| Priority | P1 |
| Estimate | 15 min |
| Depends on | T06 |
| References | [consistency.md, P13](../docs/consistency.md#p13--invalid-input), [api.md, Errors](../docs/api.md#errors) |

## Overview

Cases:
- `bytes` is `0`, negative, a decimal, a string, missing, or above 2⁶³−1.
- The body is too large, or has unknown fields.
- `Idempotency-Key` is missing, empty, longer than 255 characters, or not visible ASCII.

Each request is refused with a precise error, and nothing is written. T03 and T06 already provide most of the mechanisms; this ticket adds the format rules of the key and proves every case.

## Technical acceptance criteria

- [ ] `Idempotency-Key`: 1 to 255 visible ASCII characters (`0x21` to `0x7E`); otherwise `400 invalid_idempotency_key`.
- [ ] `bytes`: decimals, exponents, strings and values above 2⁶³−1 give `400 invalid_request`, with a `detail` naming the problem.
- [ ] A body over 4 KiB gives `413 request_too_large`; an unknown field or trailing data gives `400 invalid_request`.
- [ ] No invalid request reaches the database.

## User acceptance criteria

The user is a client application.

- Given `bytes` is `5.5`, `"5"`, `-1` or `9223372036854775808`, then the app gets `400 invalid_request` with a `detail` saying what is wrong.
- Given a body larger than 4 KiB, then the app gets `413 request_too_large`.
- Given an unknown field, as in `{"bytes": 5, "size": 5}`, then the app gets `400 invalid_request`.
- Given an `Idempotency-Key` of 256 characters, or containing a space, then the app gets `400 invalid_idempotency_key`.

## Tests

Unit (`internal/handler`):
- [ ] `TestValidateIdempotencyKey` — table: missing, empty, 1, 255 and 256 characters, space, non-ASCII.
- [ ] `TestDecodeReserveRequest` — table of bodies.

Integration (`internal/handler`):
- [ ] `TestAPI_ReserveValidation` — table of invalid headers and bodies, each giving `400` or `413` with the right `code`, and nothing written.

## Test results

_Not run yet._
