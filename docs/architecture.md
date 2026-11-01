# Architecture

> Written before implementation and kept up to date as the code lands.

How the code is organized and how a request travels through it. The reasons behind each choice are in [decisions.md](decisions.md).

## Layers

```
HTTP ──► handler ──────────► service ──────────► domain
         JSON and DTOs,       use cases,          entities, pure admission
         validation,          in-flight guard     rules, typed errors
         problem+json
                                │         │
              ReservationRepository     LimitProvider      ports, declared in service
                                │         │
                                ▼         ▼
                   repository/sqlite     limits ──► https://env.hivenet.com/dev
                                │                   (background refresh only)
                                ▼
                       SQLite file (/data)
```

Adapters implement the ports declared in `service` without importing it, because Go interfaces are satisfied implicitly. `internal/app` is the only package that knows every other one: it builds the adapters and injects them. `cmd/api/main.go` only handles signals and calls `app.Run`, so all the wiring can be tested.

| Package | Imports | Never imports |
|---|---|---|
| `domain` | Standard library only | `database/sql`, `net/http`, other internal packages |
| `service` | `domain` | `database/sql`, `net/http`, adapters |
| `repository/sqlite` | `domain`, `database/sql`, the driver | `service`, `handler` |
| `limits` | Standard library | `service`, `handler` |
| `handler` | `service`, `domain`, `net/http` | `repository`, `database/sql` |
| `app` | Every internal package | — |
| `cmd/api` | `app` | Every other internal package |

## Package layout

```
cmd/api/main.go               signal handling; calls app.Run, nothing else
internal/
├── app/                      configuration, wiring, startup, graceful shutdown, healthcheck subcommand
├── domain/
│   ├── reservation.go        Reservation, Status, AccountState, available bytes
│   ├── reserve.go            DecideReserve: admission rules, pure
│   └── errors.go             typed errors carrying facts, sentinel errors
├── service/
│   ├── ports.go              ReservationRepository, LimitProvider
│   ├── reservation.go        Reserve, Release, Get, Usage
│   └── inflight.go           in-flight Idempotency-Key guard
├── repository/sqlite/
│   ├── schema.sql            embedded, versioned with PRAGMA user_version
│   ├── db.go                 Open: DSN, pragmas, migration, account seeding
│   └── reservations.go       transactions, queries, driver error translation
├── limits/                   MAX_PARALLEL_CHUNKS: fetch, validate, cache, refresh, persist
└── handler/                  routes, middleware, request/response DTOs, problem+json
scripts/demo.sh               end-to-end demo: retry, concurrency, release
```

`limits` persists its last valid value through a small `Store` interface that it declares itself and that `repository/sqlite` implements (table `settings`).

## The transaction boundary

The rule that decides whether a reservation is admitted must run inside the transaction that writes it. The repository owns the transaction; the domain owns the rule.

```go
// domain: pure, no I/O, no clock
type ReserveState struct {
    Account  AccountState // Exists, LimitBytes, ReservedBytes, ActiveReservations
    Existing *Reservation // reservation already created with this Idempotency-Key, if any
}

type MaxActive struct {
    Value int
    Known bool // false until MAX_PARALLEL_CHUNKS has been obtained once
}

// DecideReserve applies every admission rule, in order (see consistency.md).
// It returns Create or Replay, or a typed error.
func DecideReserve(st ReserveState, bytes int64, max MaxActive) (Decision, error)

// service/ports.go
type ReservationRepository interface {
    // Reserve loads the state, calls decide and persists the outcome, all in one write transaction.
    Reserve(ctx context.Context, accountID, key string, candidate domain.Reservation,
        decide func(domain.ReserveState) (domain.Decision, error)) (domain.Reservation, domain.Decision, error)
    Release(ctx context.Context, accountID, reservationID string) (domain.Reservation, error)
    Get(ctx context.Context, accountID, reservationID string) (domain.Reservation, error)
    Account(ctx context.Context, accountID string) (domain.AccountState, error)
}

type LimitProvider interface {
    MaxActive() (value int, known bool) // service turns the pair into a domain.MaxActive
}
```

The signatures are indicative and will be settled during implementation. What will not change: no code path reads the state in one statement and writes in another without a transaction around both.

## Request flows

### Reserve

1. **handler**: reads `{accountID}`, validates the `Idempotency-Key` header and decodes the body strictly (at most 4 KiB, unknown fields rejected, `bytes` a positive 64-bit integer). Failure gives `400` or `413`.
2. **service**: takes the in-flight slot for `(accountID, key)`. If it is already taken: `409 request_in_progress`. The slot is released with `defer`.
3. **service**: reads `MAX_PARALLEL_CHUNKS` from `LimitProvider` (possibly unknown) and builds the candidate reservation (UUIDv7 ID, `created_at` from an injected clock).
4. **repository**: `BEGIN IMMEDIATE`, then loads the reservation for `(account, key)` and the account row.
5. **domain**: `DecideReserve` returns `Create`, `Replay` or an error.
6. **repository**: on `Create`, inserts the reservation, increments the counters and commits. On `Replay`, returns the stored reservation. On error, rolls back.
7. **handler**: `201` with `Location`, `200` with `Idempotent-Replayed: true`, or problem details.

### Release

One write transaction: a conditional `UPDATE … WHERE status = 'active' RETURNING bytes`, then the counters are decremented only if a row changed. Otherwise the reservation is looked up: if it is already released, it is returned unchanged; if it is missing, `404`. SQL in [consistency.md](consistency.md#release-transaction).

Release involves neither the in-flight guard nor `LimitProvider`, so it works as long as the database does.

### Usage

Reads the account row (a single-row read is consistent without an explicit transaction) and the current `MAX_PARALLEL_CHUNKS` (`null` if unknown). `available_bytes = max(0, limit − reserved)`.

## Errors across layers

| Layer | Responsibility |
|---|---|
| `domain` | Defines the errors. Business refusals carry their facts: `InsufficientStorageError{Requested, Available, Limit}`, `TooManyActiveError{Active, Max}`, `KeyReusedError{OriginalBytes}`. Sentinels: `ErrAccountNotFound`, `ErrReservationNotFound`, `ErrRequestInProgress`, `ErrLimitUnavailable`, `ErrUnavailable`. |
| `repository/sqlite` | Translates driver errors. No rows becomes a not-found sentinel; busy, locked, I/O error or closed database becomes `ErrUnavailable`: `fmt.Errorf("%w: begin reserve: %v", domain.ErrUnavailable, err)`. The sentinel is wrapped with `%w`; the driver cause only with `%v`, so its text reaches the logs but `errors.As` cannot reach the SQLite error. No SQLite error leaves this package. |
| `handler` | Maps errors to status, `code` and `detail` with `errors.Is` / `errors.As` (catalogue in [api.md](api.md#errors)). Anything unrecognized becomes `500 internal_error`, logged with its `request_id`. |

## External limit component

The `limits` package:
- **`Init(ctx)`**: loads the persisted value, then attempts one fetch (bounded by the 3 s timeout). Synchronous.
- **`MaxActive()`**: an atomic read. It never blocks and never does I/O.
- **`Run(ctx)`**: blocks and refreshes every 60 s until `ctx` is cancelled. On failure it keeps the current value and logs a warning. While no value is known, it retries with backoff from 1 s up to 30 s. `app` starts it in a goroutine it owns, on the application context (never a request's), and waits for it at shutdown. The package itself starts no goroutine.
- **Validation**: status 200, body at most 1 MiB, a JSON object, `MAX_PARALLEL_CHUNKS` an integer ≥ 1. The body is never logged, because it contains analytics tokens.
- **HTTP client**: its own `http.Client` with a timeout, never `http.Get` or `http.DefaultClient`; the response body is always closed.

## Process lifecycle

**Startup (`app.Run`):** read the configuration, open the database (pragmas, migration, account seeding), start `limits`, start the HTTP server.

**Shutdown (SIGINT or SIGTERM):** stop accepting connections and let in-flight requests finish (`http.Server.Shutdown`, 10 s), stop the refresher, close the database.

**Middleware:**
- request ID, generated per request, returned in `X-Request-Id` and attached to every log line;
- access log;
- panic recovery, which answers `500 internal_error`.

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `HTTP_ADDR` | `:8080` | Listen address |
| `DB_PATH` | `/data/reserve.db` | SQLite file |
| `SEED_ACCOUNTS` | `acc_demo:10737418240,acc_small:104857600` | Accounts applied at startup, as `id:limit_bytes` (10 GiB and 100 MiB) |
| `ENV_CONFIG_URL` | `https://env.hivenet.com/dev` | Source of `MAX_PARALLEL_CHUNKS` |
| `ENV_CONFIG_REFRESH` | `60s` | Refresh interval |
| `ENV_CONFIG_TIMEOUT` | `3s` | Fetch timeout |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |

## Container

- **Dockerfile**: stage 1 is `golang:1.27` running `CGO_ENABLED=0 go build -trimpath`; stage 2 is `gcr.io/distroless/static:nonroot` (no shell, runs as UID 65532).
- **Volume ownership**: `/data` is created in the image and owned by `65532:65532`. Docker copies that ownership into an empty named volume on first mount; without it, the non-root process cannot write the database.
- **Signals**: `ENTRYPOINT ["/app/api"]` in exec form, so the process is PID 1 and receives SIGTERM directly.
- **Health check**: distroless has no curl, so the binary's `healthcheck` subcommand calls `/healthz`.
- **Compose**:
  - `api`: host port `${API_PORT:-8080}` (Windows can reserve port ranges, so a local `.env` may override it), named volume `data`;
  - `test`: `golang:1.27` with the source mounted, running `go vet ./... && go test -race ./...`;
  - `demo`: a curl image running `scripts/demo.sh` against `api`.

  `test` and `demo` sit behind profiles and only run when invoked.

## Go conventions

- Small interfaces, declared by the package that uses them.
- No `Interface` / `Impl` pairs; no `utils`, `models` or `common` packages.
- Wiring by hand in `main.go`; no dependency-injection framework.
- Errors are values: wrapped with `%w` inside the module, inspected with `errors.Is` / `errors.As`. At the boundary with a dependency, the dependency's error is formatted with `%v`. No panics for control flow.
- `context.Context` is the first parameter of every call that does I/O.
- `gofmt`, `go vet` and `golangci-lint` report nothing.
