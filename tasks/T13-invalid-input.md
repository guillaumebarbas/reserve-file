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

- [x] `Idempotency-Key`: 1 to 255 visible ASCII characters (`0x21` to `0x7E`); otherwise `400 invalid_idempotency_key`.
- [x] `bytes`: decimals, exponents, strings and values above 2⁶³−1 give `400 invalid_request`, with a `detail` naming the problem.
- [x] A body over 4 KiB gives `413 request_too_large`; an unknown field or trailing data gives `400 invalid_request`.
- [x] No invalid request reaches the database.

## User acceptance criteria

The user is a client application.

- Given `bytes` is `5.5`, `"5"`, `-1` or `9223372036854775808`, then the app gets `400 invalid_request` with a `detail` saying what is wrong.
- Given a body larger than 4 KiB, then the app gets `413 request_too_large`.
- Given an unknown field, as in `{"bytes": 5, "size": 5}`, then the app gets `400 invalid_request`.
- Given an `Idempotency-Key` of 256 characters, or containing a space, then the app gets `400 invalid_idempotency_key`.

## Tests

Unit (`internal/handler`), split so that each table holds data only:
- [x] `TestValidateIdempotencyKey_RejectsInvalidKeys` — table: missing or empty, 256 characters, space, tab, `DEL` (`0x7F`), non-ASCII; each with the `detail` naming the rule.
- [x] `TestValidateIdempotencyKey_AcceptsVisibleASCII` — table: 1 character, 255 characters, every character from `!` to `~`, a UUID, a key derived from the upload.
- [x] `TestReadReserveRequest_RejectsInvalidBytes` *(was `TestDecodeReserveRequest`)* — table of bodies: `null`, `5.5`, `5.0`, `1e3`, `"5"`, `true`, 2⁶³ and −2⁶³−1; each with its `detail`. Missing, `0` and negative are T06's `TestAPI_ReserveRejectsInvalidBytes`.
- [x] `TestReadReserveRequest_AcceptsPositiveIntegers` — table: 1, 2⁶³−1, spaces around the value.

Integration (`internal/handler`):
- [x] `TestAPI_ReserveValidation` — table of invalid headers and bodies (key too long, with a space, non-ASCII; `bytes` decimal, exponent, string, above 2⁶³−1; unknown field; trailing data; body over 4 KiB), each giving `400` or `413` with the right `code`, and the account's counters unchanged.
- [x] `TestAPI_ReserveStoresNothingForAnInvalidRequest` *(added)* — after an invalid request with `key_1`, a valid one with the same key gets `201`: nothing was stored under that key.

## Test results

Run on 2026-11-01.

- `gofmt -l .` prints nothing; `go vet ./...` passes.
- `go test ./...` (also with `-shuffle=on`): 85 tests and 150 subtests pass, none fail or skip; `handler` has 25 tests.
- `docker compose run --rm test`: passes with the race detector; coverage of `./internal/...` is 100.0%, and every package is at 100% on its own.
- Failed first, on an assertion:
  - `TestValidateIdempotencyKey_RejectsInvalidKeys`, against a stub that accepted every key;
  - `TestReadReserveRequest_RejectsInvalidBytes`: 7 of its 8 rows got the generic detail "The request body must be a single JSON object, without unknown fields."; `null` already gave "bytes is required";
  - `TestAPI_ReserveValidation`: its 3 key rows got `201`, and the counters moved. A key of 256 characters, with a space or with an accented letter was stored, since only an empty key was refused.
- Passed at once, because the rules they pin were already written by the test before them; each was checked against a deliberately broken copy of the code, reverted afterwards:
  - `TestValidateIdempotencyKey_AcceptsVisibleASCII` fails when the length check becomes `>=` (the 255 characters row) and when the range becomes `"` to `}` (the every-character row);
  - `TestReadReserveRequest_AcceptsPositiveIntegers` fails when the positivity check becomes `<= 1` (the 1 byte row) and when `ParseInt` uses 63 bits (the 2⁶³−1 row);
  - `TestAPI_ReserveStoresNothingForAnInvalidRequest`: the handler has validated before calling the service since T06. Once T08 replays a stored key with `200`, this test also tells a stored row from an unused key.
- Container, checked by hand on a separate Compose project, removed afterwards, through the real HTTP server:
  - `Idempotency-Key: upload 42` and `Idempotency-Key: clé-42` give `400 invalid_idempotency_key`;
  - a key of 256 characters gives `400`, one of 255 gives `201`;
  - `{"bytes": 5.5}` gives `400` with "bytes must be a positive integer, got 5.5.", and 2⁶³ gives "bytes must be a positive integer of at most 9223372036854775807, got 9223372036854775808.";
  - the counters stayed at 0 after the invalid requests, and a valid request with `key_1` afterwards gives `201`.
