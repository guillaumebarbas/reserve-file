# reserve-file

A Go service that reserves storage space for uploads: a client reserves bytes before it uploads, releases them when it no longer needs them, and reads how much space its account has left. Written for the Hivenet backend homework.

**Status: complete.** Every ticket of the plan is done, from the core API to the edge cases, container tests and an OpenAPI description. What a production release would still need is listed under [Gaps](#gaps-that-would-block-a-production-release).

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

The examples below use port 8080; with `API_PORT` set, use that port instead.

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
| `CORS_ALLOWED_ORIGIN` | empty: CORS off | The one browser origin allowed to call the API, such as Swagger UI's `http://localhost:8081` |

Compose passes `SEED_ACCOUNTS`, `ENV_CONFIG_URL` and `CORS_ALLOWED_ORIGIN` from your shell or `.env`. For example, to lower `small_account` to 50 MiB at the next start, or to read the limit from another environment:

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

A refused attempt stores nothing, so the same key can succeed once there is room. The same key with another size is a client bug: it is refused with `422 idempotency_key_reused` and the original size in `original_bytes`, and nothing is reserved. A second request with the same key while the first is still being processed gets `409 request_in_progress` with `Retry-After: 1`; its retry then gets the replay.

Read the reservation back with the `id` from the response, then release it; the space comes back at once:

```sh
curl -s http://localhost:8080/accounts/small_account/reservations/01a1989c-9235-7ec5-a7f2-00e4ce5e968d
curl -s -X POST http://localhost:8080/accounts/small_account/reservations/01a1989c-9235-7ec5-a7f2-00e4ce5e968d/release
```

```json
{"id":"01a1989c-9235-7ec5-a7f2-00e4ce5e968d","account_id":"small_account","bytes":31457280,"status":"released","created_at":"2026-11-01T19:36:00.309Z","released_at":"2026-11-01T19:36:04.649Z"}
```

Releasing it again answers `200` with the same body, `released_at` included, and never gives the space back twice. An unknown reservation, or one that belongs to another account, gets `404 reservation_not_found`.

Every response carries an `X-Request-Id`; `docker compose logs api` shows one line per request with the same ID. Errors are `application/problem+json` with a stable `code`.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/accounts/{accountID}/usage` | Limit, reserved and available space, active reservations and `MAX_PARALLEL_CHUNKS` |
| `POST` | `/accounts/{accountID}/reservations` | Reserve space, with an `Idempotency-Key`; a retry with the same key is replayed |
| `GET` | `/accounts/{accountID}/reservations/{reservationID}` | Read a reservation |
| `POST` | `/accounts/{accountID}/reservations/{reservationID}/release` | Release a reservation; repeating it returns the same body |
| `GET` | `/healthz` | Liveness |
| `GET` | `/readyz` | Readiness: the database answers; `limits` tells whether `MAX_PARALLEL_CHUNKS` is known |

The contract, the error catalogue and the retry rules are in [docs/api.md](docs/api.md).

### API description

[api/openapi.yaml](api/openapi.yaml) describes every endpoint, header, body and error code in OpenAPI 3.1. To browse it and send calls from the page, start the stack with the `docs` profile and allow Swagger UI's origin:

```sh
CORS_ALLOWED_ORIGIN=http://localhost:8081 docker compose --profile docs up --build
```

Swagger UI is then at http://localhost:8081: "Try it out" sends the calls to the API on port 8080. If you changed the ports, set `DOCS_PORT` and `API_PORT` in `.env`, use `http://localhost:<DOCS_PORT>` as the allowed origin, and pick `<API_PORT>` in the server list of the page. `docker compose run --rm openapi-lint` checks the file with Redocly.

## Verify

```sh
docker compose run --rm test
```

It runs `go vet`, then the unit and integration tests with the race detector, and fails below 100% statement coverage of `internal/`. Each database test uses its own SQLite file. What each kind of test covers, and how failures are simulated: [docs/testing.md](docs/testing.md).

Concurrent reservations are proved by tests that start 20 to 50 reservations at the same instant, through the repository and through the HTTP API: only those that fit succeed, and the counters always equal the active reservations (`TestReserve_ConcurrentRequestsNeverExceedLimit`, `TestReserve_ConcurrentRequestsNeverExceedSlots`, `TestAPI_ConcurrentReservations`).

### Demo

With the stack built from the current code and running, run the demo against it:

```sh
docker compose up -d --build
docker compose run --rm demo
```

Keep `--build`: without it, Compose reuses an image built earlier from older code, and the demo fails on the endpoints that image lacks.

It uses `small_account` (100 MiB) and prints, for each step, the expected and the actual answer:
1. usage before anything: nothing reserved;
2. a reservation of 30 MiB: `201`;
3. the same request with the same key, as after a lost response: `200`, `Idempotent-Replayed: true`, the same ID, usage unchanged;
4. 80 MiB while 70 MiB are free: `409 insufficient_storage`;
5. 20 reservations of 20 MiB sent at the same time: exactly 3 get `201` and 17 get `409`, and the counters match;
6. the first key reused with 31 MiB: `422 idempotency_key_reused`;
7. the first reservation released twice: `200` both times, and the space comes back once;
8. the other reservations released, and usage back to zero.

```text
5. Send 20 reservations of 20 MiB at the same time, with distinct keys
   201 Created: expected 3, got 3
   409 Conflict: expected 17, got 17
   usage status: expected 200, got 200
   reserved bytes: expected 94371840, got 94371840
   active reservations: expected 4, got 4
```

It stops with a non-zero exit code at the first unexpected answer. Even then it releases what it created, so it can run again. The script is [scripts/demo.sh](scripts/demo.sh): POSIX shell and curl, run in a `curlimages/curl` container.

### Restart

All the state is in the database. Reserve something, then restart the service: usage is unchanged, and a retry with the same key is still replayed.

```sh
docker compose restart api
```

On `docker compose stop` (SIGTERM), the server stops accepting connections, lets the requests in progress finish, then closes the database; the log ends with `server stopped`.

### Container tests

They test the packaged service the way a reviewer runs it, so they need Go and Docker on the host:

```sh
go test -tags e2e ./e2e/...
```

They build the image and start it with `compose.yaml` plus [compose.e2e.yaml](compose.e2e.yaml), as Compose project `reserve-e2e` on port 18090, so a stack you already run is untouched. An nginx container stands in for env.hivenet.com with a fixed `MAX_PARALLEL_CHUNKS` of 5. Each test starts from a fresh volume, waits for `/readyz`, and removes its containers, volume and image at the end, even after a failure. They prove what in-process tests cannot see:
- the process runs as the non-root user 65532 and writes its database on the volume;
- the main flow, and 20 concurrent requests that never exceed the limit, over the network;
- reservations, counters and idempotency keys survive `docker compose restart` and `docker compose kill -s SIGKILL`;
- SIGTERM stops the service in under 10 s with exit code 0, and the log ends with `server stopped`;
- with the limit source down, a restart reloads the last value from the database, and a first start without it refuses new reservations with `503 limit_unavailable` while usage still answers.

A run takes about two minutes, most of it the first image build. The build tag keeps them out of `go test ./...` and of the coverage check.

## Design

How reservations stay consistent:

- Each reserve or release is one SQLite transaction that takes the write lock at `BEGIN` (`BEGIN IMMEDIATE`), so concurrent requests queue and each one sees the state the previous one left. The account row carries its counters, so admission reads one row, whatever the number of reservations.
- The client sends an `Idempotency-Key`. It is stored with the reservation and looked up before every admission rule, so a retry gets the same reservation, even after a release or while the account is full. The same key still in progress gets `409`; the same key with another size gets `422`.
- A release moves a reservation from active to released once; repeating it changes nothing.
- `MAX_PARALLEL_CHUNKS` is fetched from `https://env.hivenet.com/dev` at startup and in the background, cached and saved; no request waits for it. When the source is down, the last value is kept. Before any value is known, new reservations get `503 limit_unavailable`, while release, usage, reads and replays keep working.
- Lowering a limit below current usage revokes nothing: new reservations are refused until releases bring usage back under it.
- A database failure answers `503 service_unavailable` with `Retry-After: 1` and leaves nothing half-written, so a retry with the same key is safe.

The reasons, the invariants and the failure handling are in [docs/decisions.md](docs/decisions.md), [docs/consistency.md](docs/consistency.md) and [docs/architecture.md](docs/architecture.md).

## Status

Each step is a ticket in [tasks/](tasks/README.md) and an issue on the [project board](https://github.com/users/guillaumebarbas/projects/2), with its acceptance criteria, its tests and their results. All 23 are done: the core API and its persistence (T01 to T11), the edge cases (T13 to T21), the container tests (T22), this README and the demo (T12), and the OpenAPI description (T23).

### Time spent

- Before the first ticket, the design and the tooling: docs, decisions, tickets, project rules, GitHub setup.
- T01 to T03, one ticket at a time: about 1 h 35 min.
- T04 to T23, built in parallel by agents and reviewed one at a time: about 1 h 45 min of wall-clock time.

### Gaps that would block a production release

The single-instance MVP the brief asks for is complete; a production release would still need:
- reservation expiry, so a client that crashes does not hold its space and slot forever;
- a retention window for released reservations and idempotency keys, which are kept forever today;
- several instances: PostgreSQL instead of a local SQLite file, since the in-flight guard is per process;
- authentication, so the account comes from the caller's identity instead of the path;
- rate limiting, metrics and tracing;
- backups of the database.

Each one, with its impact and the direction to take: [docs/consistency.md](docs/consistency.md#gaps-that-would-block-a-production-release).

## How AI was used

- Claude Code wrote the design docs, the tickets, the project rules and the code, test first.
- I directed the work: I made the decisions, validated the plans and asked for changes, such as the rich `Account` aggregate, descriptive names and no code comments.
- Every ticket goes through a pull request, and the review is published on it.
- From the limit source on, independent tickets were built in parallel by Claude Code agents in separate git worktrees; a Claude Code session reviewed their pull requests on GitHub and merged them.
- I can explain and modify every part of the code.

## Repository

- [docs/](docs/): API contract, architecture, consistency and failure handling, decisions, testing strategy.
- [tasks/](tasks/README.md): one ticket per step, with its acceptance criteria, tests and results.
- `cmd/api` and `internal/`: the service, organised as described in [docs/architecture.md](docs/architecture.md).
