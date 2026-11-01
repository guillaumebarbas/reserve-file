# API

> Written before implementation and kept up to date as the code lands.

## Conventions

- Base URL: `http://localhost:8080`. Requests and responses are JSON; errors are `application/problem+json`.
- Sizes are integers in bytes. Timestamps are RFC 3339 in UTC. Reservation IDs are UUIDv7 strings.
- There is no authentication: the account is taken from the path ([gap G4](consistency.md#gaps-that-would-block-a-production-release)).
- Every response carries an `X-Request-Id` header.
- No CORS headers are sent. A browser client on another origin would need `Idempotency-Key` allowed (`Access-Control-Allow-Headers`) and `Idempotent-Replayed` exposed (`Access-Control-Expose-Headers`).

## Endpoints

| Method | Path | Purpose | Success |
|---|---|---|---|
| `POST` | `/accounts/{accountID}/reservations` | Reserve space | `201` new, `200` replay |
| `GET` | `/accounts/{accountID}/reservations/{reservationID}` | Read a reservation | `200` |
| `POST` | `/accounts/{accountID}/reservations/{reservationID}/release` | Release a reservation | `200` |
| `GET` | `/accounts/{accountID}/usage` | Limit, reserved space and available space | `200` |
| `GET` | `/healthz` | Liveness | `200` |
| `GET` | `/readyz` | Readiness | `200` or `503` |

## Reserve space

```sh
curl -i -X POST http://localhost:8080/accounts/large_account/reservations \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: 9b2f4c1e-5a43-4f7e-9d0a-2f7c1b8e4d21' \
  -d '{"bytes": 4194304}'
```

```http
HTTP/1.1 201 Created
Location: /accounts/large_account/reservations/01a196fb-1600-7c4e-9f1d-3a5b6c7d8e9f
Content-Type: application/json

{
  "id": "01a196fb-1600-7c4e-9f1d-3a5b6c7d8e9f",
  "account_id": "large_account",
  "bytes": 4194304,
  "status": "active",
  "created_at": "2026-11-01T12:00:00Z",
  "released_at": null
}
```

| Body field | Rule |
|---|---|
| `bytes` | Required. A JSON integer between 1 and 2⁶³−1. Decimals (`5.5`, `5.0`), exponents and strings are rejected. |

Unknown fields and trailing data are rejected. The body is limited to 4 KiB.

**Errors:** `400 invalid_request`, `400 invalid_idempotency_key`, `404 account_not_found`, `409 request_in_progress`, `409 too_many_active_reservations`, `409 insufficient_storage`, `413 request_too_large`, `422 idempotency_key_reused`, `503 limit_unavailable`, `503 service_unavailable`.

## Idempotency

The `Idempotency-Key` header is required on reserve.

- **Format:** 1 to 255 visible ASCII characters. A random UUID is recommended.
- **Scope:** per account.
- **Not a secret:** the key grants no access; it only deduplicates requests within the caller's account ([D7](decisions.md#d7--client-generated-idempotency-key)).
- **Client rule:** generate the key once per logical request, send it on every retry, and use a new key for a new request. Store it with the upload's state, not only in memory: a key lost on a page reload or an app restart turns the next retry into a second reservation. A key derived from the upload (for example `<file-id>:<chunk-index>`) also works, provided the file ID is unique within the account.

| Situation | Response |
|---|---|
| New key | Evaluated normally; `201` on success |
| Same key, same `bytes`, first request finished | `200` with `Idempotent-Replayed: true` and the reservation in its **current** state (it may be `released`) |
| Same key, different `bytes` | `422 idempotency_key_reused` |
| Same key while the first request is still being processed | `409 request_in_progress` with `Retry-After: 1` |
| Same key after the first attempt was refused (`4xx`) or failed (`5xx`) | Nothing was stored, so it is evaluated as a new request |

Keys are currently kept forever ([gap G2](consistency.md#gaps-that-would-block-a-production-release)).

## Read a reservation

`GET /accounts/{accountID}/reservations/{reservationID}` returns `200` with the reservation body, or `404 reservation_not_found` if it does not exist or belongs to another account.

## Release a reservation

```sh
curl -i -X POST http://localhost:8080/accounts/large_account/reservations/01a196fb-1600-7c4e-9f1d-3a5b6c7d8e9f/release
```

```json
{
  "id": "01a196fb-1600-7c4e-9f1d-3a5b6c7d8e9f",
  "account_id": "large_account",
  "bytes": 4194304,
  "status": "released",
  "created_at": "2026-11-01T12:00:00Z",
  "released_at": "2026-11-01T12:03:10Z"
}
```

- **Idempotent:** repeating the call returns `200` and the same body. The space is given back once.
- **Not found:** `404 reservation_not_found` for an unknown reservation or one owned by another account.
- **Independent of `MAX_PARALLEL_CHUNKS`:** release works even when the limit source is down.

## Usage

```sh
curl -s http://localhost:8080/accounts/large_account/usage
```

```json
{
  "account_id": "large_account",
  "limit_bytes": 10737418240,
  "reserved_bytes": 4194304,
  "available_bytes": 10733223936,
  "active_reservations": 1,
  "max_active_reservations": 5
}
```

- `available_bytes = max(0, limit_bytes − reserved_bytes)`.
- `max_active_reservations` is `null` while `MAX_PARALLEL_CHUNKS` has never been obtained.
- After a limit is lowered, `reserved_bytes` may exceed `limit_bytes` and `active_reservations` may exceed `max_active_reservations`. Nothing is revoked ([D11](decisions.md#d11--lowered-limits-never-revoke-reservations)).

**Errors:** `404 account_not_found`, `503 service_unavailable`.

## Health

| Endpoint | `200` when | Body |
|---|---|---|
| `/healthz` | The process serves HTTP | `{"status":"ok"}` |
| `/readyz` | The database answers | `{"database":"ok","limits":"ok"}`, where `limits` is `"unavailable"` while `MAX_PARALLEL_CHUNKS` is unknown |

An unknown limit does not fail readiness, because release, usage and replays still work. `/readyz` returns `503` only when the database does not answer.

## Errors

```http
HTTP/1.1 409 Conflict
Content-Type: application/problem+json
X-Request-Id: c0a8f3e2b71d4e5f

{
  "type": "https://github.com/guillaumebarbas/reserve-file/blob/main/docs/api.md#insufficient_storage",
  "title": "Insufficient storage",
  "status": 409,
  "code": "insufficient_storage",
  "detail": "Requested 5.00 GiB but only 1.20 GiB is available (limit 10.00 GiB). Release a reservation or request less space.",
  "requested_bytes": 5368709120,
  "available_bytes": 1288490189,
  "limit_bytes": 10737418240,
  "request_id": "c0a8f3e2b71d4e5f"
}
```

- `code` is stable, so clients branch on it. `detail` is written for humans and may change.
- `type` points to the code's entry in the table below.
- `5xx` responses never contain internal error text; `request_id` links them to the server logs.

| `code` | Status | When | `detail` example | Extra fields | `Retry-After` |
|---|---|---|---|---|---|
| <a id="invalid_request"></a>`invalid_request` | 400 | Malformed JSON, unknown field, trailing data, or `bytes` not a positive 64-bit integer | "bytes must be a positive integer, got -5." | — | — |
| <a id="invalid_idempotency_key"></a>`invalid_idempotency_key` | 400 | Header missing, empty, longer than 255 characters or not visible ASCII | "The Idempotency-Key header is required. Generate one key per logical request and reuse it on every retry." | — | — |
| <a id="account_not_found"></a>`account_not_found` | 404 | Unknown account | "Account 'unknown_account' does not exist." | — | — |
| <a id="reservation_not_found"></a>`reservation_not_found` | 404 | Unknown reservation, or one owned by another account | "Reservation '01a1…' does not exist for account 'large_account'." | — | — |
| <a id="request_in_progress"></a>`request_in_progress` | 409 | The same key is still being processed | "A request with this Idempotency-Key is still being processed. Retry in a moment to get its result." | — | 1 |
| <a id="too_many_active_reservations"></a>`too_many_active_reservations` | 409 | `active_reservations ≥ MAX_PARALLEL_CHUNKS` | "You already have 5 active reservations, the maximum allowed. Release a reservation before creating a new one." | `active_reservations`, `max_active_reservations` | — |
| <a id="insufficient_storage"></a>`insufficient_storage` | 409 | `bytes > limit − reserved` | "Requested 5.00 GiB but only 1.20 GiB is available (limit 10.00 GiB). Release a reservation or request less space." | `requested_bytes`, `available_bytes`, `limit_bytes` | — |
| <a id="request_too_large"></a>`request_too_large` | 413 | Body over 4 KiB | "The request body must not exceed 4096 bytes." | — | — |
| <a id="idempotency_key_reused"></a>`idempotency_key_reused` | 422 | Same key, different `bytes` | "This Idempotency-Key was already used to reserve 4194304 bytes. Use a new key for a new request." | `original_bytes` | — |
| <a id="limit_unavailable"></a>`limit_unavailable` | 503 | `MAX_PARALLEL_CHUNKS` never obtained | "Reservation limits are temporarily unavailable. Retry later; releases are not affected." | — | 5 |
| <a id="service_unavailable"></a>`service_unavailable` | 503 | Database busy or unavailable | "The service is temporarily unavailable. Retry with the same Idempotency-Key." | — | 1 |
| <a id="internal_error"></a>`internal_error` | 500 | Unexpected error | "An unexpected error occurred. Reference: c0a8f3e2b71d4e5f." | — | — |

**Status code policy:**
- `409`: refused because of the current state; the same request can succeed later.
- `422`: the key was reused for a different request; retrying will never succeed (as the IETF draft specifies).
- `503` with `Retry-After`: a temporary server-side condition; retry with the same key.
- Not used: `429` (the slot limit is not time-based) and `507` (WebDAV, about the server's own storage).

Unknown paths and wrong methods get the standard library's plain-text `404` and `405` (with an `Allow` header).
