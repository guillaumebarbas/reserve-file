# reserve-file

A Go service that reserves storage space for uploads: a client reserves bytes before it uploads, releases them when it no longer needs them, and reads how much space its account has left. Written for the Hivenet backend homework.

**Status: in progress.** The foundations are done; the reservation endpoints are being built. See [Status](#status).

## Run it

You need Docker with Compose. No Go toolchain and no database to install.

```sh
docker compose up --build
```

The API listens on `http://localhost:8080`. `docker compose ps` shows the container as `healthy` once `/healthz` answers.

If port 8080 is taken, or reserved as Windows does for Hyper-V and WSL, pick another host port in a local `.env` file:

```sh
echo API_PORT=18080 > .env
```

### Database

SQLite runs inside the service container. At startup the service:
- creates the schema if the database is new;
- applies `SEED_ACCOUNTS`: a missing account is created, an existing one only gets its new limit, and its reservations and counters are never touched.

The file lives on the `data` volume, so it survives restarts and rebuilds. To start again from an empty database:

```sh
docker compose down -v
```

### Configuration

| Variable | Default | Purpose |
|---|---|---|
| `HTTP_ADDR` | `:8080` | Listen address inside the container |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `DB_PATH` | `/data/reserve.db` | SQLite file |
| `SEED_ACCOUNTS` | `large_account:10737418240,small_account:104857600` | Accounts applied at startup, as `id:limit_bytes` (10 GiB and 100 MiB) |
| `ENV_CONFIG_URL` | `https://env.hivenet.com/dev` | Source of `MAX_PARALLEL_CHUNKS`, an absolute http or https URL |
| `ENV_CONFIG_REFRESH` | `60s` | How often the limit is fetched again |
| `ENV_CONFIG_TIMEOUT` | `3s` | Timeout of one fetch |

Compose passes `SEED_ACCOUNTS` and `ENV_CONFIG_URL` from your shell or `.env`. For example, to lower `small_account` to 50 MiB at the next start, or to read the limit from another environment:

```sh
SEED_ACCOUNTS=large_account:10737418240,small_account:52428800 docker compose up --build
ENV_CONFIG_URL=https://env.example.com/staging docker compose up --build
```

Any other variable goes under `services.api.environment` in a `compose.override.yaml`, which Compose merges automatically. An invalid value stops the startup with an error that names the variable.

## Try it

```sh
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/readyz
```

```http
HTTP/1.1 200 OK
Content-Type: application/json
X-Request-Id: b749a20a74cad954

{"database":"ok","limits":"ok"}
```

```sh
curl -s http://localhost:8080/accounts/large_account/usage
```

```json
{"account_id":"large_account","limit_bytes":10737418240,"reserved_bytes":0,"available_bytes":10737418240,"active_reservations":0,"max_active_reservations":5}
```

`max_active_reservations` is the current `MAX_PARALLEL_CHUNKS`, or `null` while it has never been obtained. An unknown account gives `404 account_not_found`.

Reserve 30 MiB on `small_account` (100 MiB). The `Idempotency-Key` header is required: generate one per logical request and send it again on every retry.

```sh
curl -i -X POST http://localhost:8080/accounts/small_account/reservations \
  -H 'Idempotency-Key: 9b2f4c1e-5a43-4f7e-9d0a-2f7c1b8e4d21' \
  -d '{"bytes": 31457280}'
```

```http
HTTP/1.1 201 Created
Content-Type: application/json
Location: /accounts/small_account/reservations/01a1988c-8e0a-77f2-8afe-1c0d35876fe2

{"id":"01a1988c-8e0a-77f2-8afe-1c0d35876fe2","account_id":"small_account","bytes":31457280,"status":"active","created_at":"2026-11-01T19:18:30.666Z","released_at":null}
```

A request that does not fit is refused with its numbers, and nothing is reserved:

```json
{"type":"https://github.com/guillaumebarbas/reserve-file/blob/main/docs/api.md#insufficient_storage","title":"Insufficient storage","status":409,"code":"insufficient_storage","detail":"Requested 80.00 MiB but only 70.00 MiB is available (limit 100.00 MiB). Release a reservation or request less space.","requested_bytes":83886080,"available_bytes":73400320,"limit_bytes":104857600,"request_id":"4fd5a3fcc5666c6f"}
```

A retry with the same key and size gets the same reservation back, with `200` and `Idempotent-Replayed: true`; nothing is reserved twice and no second slot is used:

```http
HTTP/1.1 200 OK
Content-Type: application/json
Idempotent-Replayed: true

{"id":"01a1988c-8e0a-77f2-8afe-1c0d35876fe2","account_id":"small_account","bytes":31457280,"status":"active","created_at":"2026-11-01T19:18:30.666Z","released_at":null}
```

A refused attempt stores nothing, so the same key can succeed once there is room. The same key with another size is a client bug: it is refused with `422 idempotency_key_reused` and the original size in `original_bytes`, and nothing is reserved.

Read the reservation back with the `id` from the response, then release it; the space comes back at once:

```sh
curl -s http://localhost:8080/accounts/small_account/reservations/01a1989c-9235-7ec5-a7f2-00e4ce5e968d
curl -s -X POST http://localhost:8080/accounts/small_account/reservations/01a1989c-9235-7ec5-a7f2-00e4ce5e968d/release
```

```json
{"id":"01a1989c-9235-7ec5-a7f2-00e4ce5e968d","account_id":"small_account","bytes":31457280,"status":"released","created_at":"2026-11-01T19:36:00.309Z","released_at":"2026-11-01T19:36:04.649Z"}
```

A second release of the same reservation never gives the space back twice; until T10, it answers `500` instead of the same body.

Every response carries an `X-Request-Id`; `docker compose logs api` shows one line per request with the same ID. Errors are `application/problem+json` with a stable `code`.

| Method | Path | Purpose | Available |
|---|---|---|---|
| `GET` | `/accounts/{accountID}/usage` | Limit, reserved space and available space | Yes |
| `POST` | `/accounts/{accountID}/reservations` | Reserve space, with an `Idempotency-Key` | Yes; a retry with the same key is replayed |
| `GET` | `/accounts/{accountID}/reservations/{reservationID}` | Read a reservation | Yes |
| `POST` | `/accounts/{accountID}/reservations/{reservationID}/release` | Release a reservation | Yes; a second release with T10 |
| `GET` | `/healthz` | Liveness | Yes |
| `GET` | `/readyz` | Readiness: the database answers; `limits` tells whether `MAX_PARALLEL_CHUNKS` is known | Yes |

The contract, the error catalogue and the retry rules are in [docs/api.md](docs/api.md).

## Verify

```sh
docker compose run --rm test
```

It runs `go vet`, then the unit and integration tests with the race detector, and fails below 100% statement coverage of `internal/`. Each database test uses its own SQLite file. What each kind of test covers, and how failures are simulated: [docs/testing.md](docs/testing.md).

Concurrent reservations are proved by tests that start 20 to 50 reservations at the same instant, through the repository and through the HTTP API: only those that fit succeed, and the counters always equal the active reservations (`TestReserve_ConcurrentRequestsNeverExceedLimit`, `TestReserve_ConcurrentRequestsNeverExceedSlots`, `TestAPI_ConcurrentReservations`).

A demo script that shows a reservation, a retry, concurrent requests and a release against the running stack comes with [T12](tasks/T12-demo-and-readme.md).

## Design

How reservations work; the endpoints are being built in T05 to T09.

- Each reserve or release is one SQLite write transaction. The account row carries its counters, so admission reads one row, whatever the number of reservations.
- The client sends an `Idempotency-Key`. It is stored with the reservation, so a retry gets the same reservation instead of a second one.
- `MAX_PARALLEL_CHUNKS` is fetched from `https://env.hivenet.com/dev` in the background and cached; no request waits for it.

The reasons, the invariants and the failure handling are in [docs/decisions.md](docs/decisions.md), [docs/consistency.md](docs/consistency.md) and [docs/architecture.md](docs/architecture.md).

## Status

Each step is a ticket in [tasks/](tasks/README.md) and an issue on the [project board](https://github.com/users/guillaumebarbas/projects/2).

- **Done:** service skeleton, Docker image and test pipeline (T01); database, schema and seeded accounts (T02); HTTP foundation: request IDs, access logs, panic recovery, error format, readiness (T03); `MAX_PARALLEL_CHUNKS` fetched at startup, cached, saved and refreshed every minute (T04); account usage, through every layer (T05); reserving space within the storage limit and the slots (T06); concurrent reservations serialized by `BEGIN IMMEDIATE`, so they never exceed the limit or the slots (T07); a retry with the same key replayed instead of reserved twice (T08); a known key reused with another size refused with `422` (T14); reading and releasing a reservation (T09); invalid keys and sizes refused with a precise detail (T13); the limit source retried with a backoff while the limit is unknown (T18, part 1).
- **In progress:** this README and the demo (T12).
- **Not started:** double release and restart (T10, T11), the other edge cases (T15 to T17, T19 to T21, and the end-to-end part of T18), container tests (T22), OpenAPI (T23).

Time spent so far: T01 35 min, T02 30 min, T03 30 min, then T04 to T09, T13 and part of T18 built in parallel by agents in about 1 h of wall-clock time, after the design and tooling work (docs, tickets, rules, GitHub setup).

## How AI was used

- Claude Code wrote the design docs, the tickets, the project rules and the code, test first.
- I directed the work: I made the decisions, validated the plans and asked for changes, such as the rich `Account` aggregate, descriptive names and no code comments.
- Every ticket goes through a pull request, and the review is published on it.
- From the limit source on, independent tickets are built in parallel by Claude Code agents in separate git worktrees; a Claude Code session reviews their pull requests on GitHub and merges them.
- I can explain and modify every part of the code.

## Repository

- [docs/](docs/): API contract, architecture, consistency and failure handling, decisions, testing strategy.
- [tasks/](tasks/README.md): one ticket per step, with its acceptance criteria, tests and results.
- `cmd/api` and `internal/`: the service, organised as described in [docs/architecture.md](docs/architecture.md).
