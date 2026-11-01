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

Compose passes `SEED_ACCOUNTS` from your shell or `.env`. For example, to lower `small_account` to 50 MiB at the next start:

```sh
SEED_ACCOUNTS=large_account:10737418240,small_account:52428800 docker compose up --build
```

Any other variable goes under `services.api.environment` in a `compose.override.yaml`, which Compose merges automatically. An invalid value stops the startup with an error that names the variable.

The settings of the limit source (`ENV_CONFIG_URL`, `ENV_CONFIG_REFRESH`, `ENV_CONFIG_TIMEOUT`) come with [T04](tasks/T04-external-limit.md).

## Try it

```sh
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/readyz
```

```http
HTTP/1.1 200 OK
Content-Type: application/json
X-Request-Id: b749a20a74cad954

{"database":"ok"}
```

Every response carries an `X-Request-Id`; `docker compose logs api` shows one line per request with the same ID. Errors are `application/problem+json` with a stable `code`.

| Method | Path | Purpose | Available |
|---|---|---|---|
| `GET` | `/accounts/{accountID}/usage` | Limit, reserved space and available space | With T05 |
| `POST` | `/accounts/{accountID}/reservations` | Reserve space, with an `Idempotency-Key` | With T06 |
| `GET` | `/accounts/{accountID}/reservations/{reservationID}` | Read a reservation | With T09 |
| `POST` | `/accounts/{accountID}/reservations/{reservationID}/release` | Release a reservation | With T09 |
| `GET` | `/healthz` | Liveness | Yes |
| `GET` | `/readyz` | Readiness: the database answers | Yes |

The contract, the error catalogue and the retry rules are in [docs/api.md](docs/api.md).

## Verify

```sh
docker compose run --rm test
```

It runs `go vet`, then the unit and integration tests with the race detector, and fails below 100% statement coverage of `internal/`. Each database test uses its own SQLite file. What each kind of test covers, and how failures are simulated: [docs/testing.md](docs/testing.md).

A demo script that shows a reservation, a retry, concurrent requests and a release against the running stack comes with [T12](tasks/T12-demo-and-readme.md).

## Design

How reservations work; the endpoints are being built in T05 to T09.

- Each reserve or release is one SQLite write transaction. The account row carries its counters, so admission reads one row, whatever the number of reservations.
- The client sends an `Idempotency-Key`. It is stored with the reservation, so a retry gets the same reservation instead of a second one.
- `MAX_PARALLEL_CHUNKS` is fetched from `https://env.hivenet.com/dev` in the background and cached; no request waits for it.

The reasons, the invariants and the failure handling are in [docs/decisions.md](docs/decisions.md), [docs/consistency.md](docs/consistency.md) and [docs/architecture.md](docs/architecture.md).

## Status

Each step is a ticket in [tasks/](tasks/README.md) and an issue on the [project board](https://github.com/users/guillaumebarbas/projects/2).

- **Done:** service skeleton, Docker image and test pipeline (T01); database, schema and seeded accounts (T02); HTTP foundation: request IDs, access logs, panic recovery, error format, readiness (T03).
- **In progress:** limit source (T04), usage (T05), this README and the demo (T12).
- **Not started:** reserve, release, retries and concurrency (T06 to T11), the other edge cases (T13 to T21), container tests (T22), OpenAPI (T23).

Time spent so far: T01 35 min, T02 30 min, T03 30 min, after the design and tooling work (docs, tickets, rules, GitHub setup).

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
