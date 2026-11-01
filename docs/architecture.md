# Architecture

> Written before implementation and kept up to date as the code lands.

How the code is organized and how a request travels through it. The reasons behind each choice are in [decisions.md](decisions.md).

## Layers

```
HTTP ──► handler ──────────► service ──────────► domain
         JSON and DTOs,       use cases,          Account aggregate:
         validation,          in-flight guard     rules as methods,
         problem+json                             typed errors
                                │         │
                AccountRepository       LimitProvider      ports, declared in service
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

The table is about production code. An integration test may use another adapter: `handler`'s `TestAPI_Readyz` opens a real `sqlite.Store`. `go list -f '{{.Imports}}' ./internal/...` shows the production imports only.

## Package layout

```
cmd/api/main.go               signal handling; calls app.Run, nothing else
internal/
├── app/                      configuration, wiring, startup, graceful shutdown, healthcheck subcommand
├── domain/
│   ├── account.go            Account aggregate: Reserve, Release, AvailableBytes; RestoreAccount
│   ├── reservation.go        Reservation, Status, Outcome, MaxActive
│   └── errors.go             typed errors carrying facts, sentinel errors
├── service/
│   ├── ports.go              AccountRepository, LimitProvider
│   ├── service.go            Service, New
│   ├── usage.go              Usage
│   ├── reservation.go        Reserve, Release, Reservation
│   └── inflight.go           in-flight Idempotency-Key guard
├── repository/sqlite/
│   ├── schema.sql            embedded, versioned with PRAGMA user_version
│   ├── db.go                 Open: DSN, pragmas, migration, account seeding
│   ├── accounts.go           AccountRepository: loading and saving the Account aggregate
│   └── reservations.go       the reserve and release transactions, driver error translation
├── limits/                   MAX_PARALLEL_CHUNKS: fetch, validate, cache, refresh, persist
└── handler/                  routes, middleware, request/response DTOs, problem+json
scripts/demo.sh               end-to-end demo: retry, concurrency, release
```

`limits` persists its last valid value through a small `Store` interface that it declares itself and that `repository/sqlite` implements (table `settings`).

## The transaction boundary

The rules live on the `Account` aggregate; the repository runs them inside the transaction that writes their result ([D4](decisions.md#d4--layered-architecture-rules-inside-the-transaction), [D20](decisions.md#d20--a-rich-account-aggregate-with-private-counters)).

```go
// domain: pure methods, no I/O, no clock, no ID generation
type Account struct {
    ID                 string
    LimitBytes         int64
    reservedBytes      int64 // unexported: only Reserve and Release change the counters
    activeReservations int
}

// RestoreAccount rebuilds an account from storage.
func RestoreAccount(id string, limitBytes, reservedBytes int64, activeReservations int) Account

// Reserve answers a reservation request. existing is the reservation already made
// with the same Idempotency-Key, if any, and is answered first: Replayed, or
// IdempotencyKeyReusedError. Otherwise candidate is admitted only if a slot and
// enough space are free, and the counters grow: Created.
func (account *Account) Reserve(candidate Reservation, existing *Reservation, maxActive MaxActive) (Reservation, Outcome, error)

// Release marks the reservation released and gives its space back. On a
// reservation already released, it changes nothing and returns false.
func (account *Account) Release(reservation *Reservation, releasedAt time.Time) bool

func (account Account) AvailableBytes() int64 // max(0, limit − reserved)
func (account Account) ReservedBytes() int64
func (account Account) ActiveReservations() int

type MaxActive struct {
    Value int
    Known bool // false until MAX_PARALLEL_CHUNKS has been obtained once
}

// service/ports.go: one repository for the aggregate
type AccountRepository interface {
    // Reserve loads the account and the reservation already made with the
    // candidate's key, calls account.Reserve, then inserts the reservation and
    // saves the counters: one write transaction.
    Reserve(ctx context.Context, candidate domain.Reservation, maxActive domain.MaxActive) (domain.Reservation, domain.Outcome, error)
    // Release loads the account and the reservation, calls account.Release and
    // saves what changed: one write transaction.
    Release(ctx context.Context, accountID, reservationID string, at time.Time) (domain.Reservation, error)
    Reservation(ctx context.Context, accountID, reservationID string) (domain.Reservation, error)
    Account(ctx context.Context, accountID string) (domain.Account, error)
}

type LimitProvider interface {
    MaxActive() (value int, known bool) // service turns the pair into a domain.MaxActive
}
```

The signatures are indicative and will be settled during implementation. What will not change: the counters change only through the methods of `Account`, and no code path reads in one statement and writes in another without a transaction around both. Saving the counters as absolute values is safe because `BEGIN IMMEDIATE` holds the write lock from the load to the commit.

## Request flows

### Reserve

1. **handler**: reads `{accountID}`, validates the `Idempotency-Key` header and decodes the body strictly (at most 4 KiB, unknown fields rejected, `bytes` a positive 64-bit integer). Failure gives `400` or `413`.
2. **service**: takes the in-flight slot for `(accountID, key)`. If it is already taken: `409 request_in_progress`. The slot is released with `defer`.
3. **service**: reads `MAX_PARALLEL_CHUNKS` from `LimitProvider` (possibly unknown) and builds the candidate reservation (UUIDv7 ID, `created_at` from an injected clock).
4. **repository**: `BEGIN IMMEDIATE`, then loads the account (unknown: `404 account_not_found`) and the reservation already made with the key, if any. Checking the account first never hides a replay: the foreign key guarantees that no reservation exists for an unknown account.
5. **domain**: `account.Reserve(candidate, existing, maxActive)` returns `Created` or `Replayed`, or a typed error.
6. **repository**: on `Created`, inserts the reservation, saves the counters and commits. On `Replayed`, returns the stored reservation. On error, rolls back.
7. **handler**: `201` with `Location`, `200` with `Idempotent-Replayed: true`, or problem details.

### Release

One write transaction: load the reservation (unknown, or owned by another account: `404`) and the account, then `account.Release(&reservation, now)`. It marks the reservation released and gives its space back, or reports that it already was. Only when it changed, the repository writes the reservation and the counters. SQL in [consistency.md](consistency.md#release-transaction).

Release involves neither the in-flight guard nor `LimitProvider`, so it works as long as the database does.

### Usage

Reads the account (a single-row read is consistent without an explicit transaction) and the current `MAX_PARALLEL_CHUNKS` (`null` if unknown). `available_bytes` is `account.AvailableBytes()`, that is `max(0, limit − reserved)`.

## Errors across layers

| Layer | Responsibility |
|---|---|
| `domain` | Defines the errors. Business refusals carry their facts: `InsufficientStorageError{RequestedBytes, AvailableBytes, LimitBytes}`, `TooManyActiveReservationsError{ActiveReservations, MaxActiveReservations}`, `IdempotencyKeyReusedError{OriginalBytes}`. Sentinels: `ErrAccountNotFound`, `ErrReservationNotFound`, `ErrRequestInProgress`, `ErrLimitUnavailable`, `ErrServiceUnavailable`. |
| `repository/sqlite` | Translates driver errors. No rows becomes a not-found sentinel; busy, locked, I/O error or closed database becomes `ErrServiceUnavailable`: `fmt.Errorf("%w: begin reserve: %v", domain.ErrServiceUnavailable, err)`. The sentinel is wrapped with `%w`; the driver cause only with `%v`, so its text reaches the logs but `errors.As` cannot reach the SQLite error. No SQLite error leaves this package. |
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

**Shutdown (SIGINT or SIGTERM):** stop accepting connections and let in-flight requests finish (`http.Server.Shutdown`, 10 s), stop the refresher, close the database. If the server stops on its own, the same shutdown runs, as after a signal.

**HTTP server timeouts:** read header 5 s, read 10 s, write 15 s, idle 60 s. The write timeout is longer than SQLite's 5 s busy timeout, so a `503` sent after a lock wait still reaches the client.

**Middleware**, from the outside in:
- request ID: 16 random hexadecimal characters (`crypto/rand`), returned in `X-Request-Id`, stored in the request context and written on every log line of the request;
- access log: one line per request, with method, path, status and duration;
- panic recovery, which answers `500 internal_error` and logs the panic with its stack trace. It sits inside the access log, so the log records that `500`.

**Readiness:** `handler` declares a one-method interface, `DatabasePinger`, that `sqlite.Store` satisfies without knowing it; `app` passes the store to `handler.New`. `Store.Ping` translates the driver error into `domain.ErrServiceUnavailable`.

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `HTTP_ADDR` | `:8080` | Listen address |
| `DB_PATH` | `/data/reserve.db` | SQLite file |
| `SEED_ACCOUNTS` | `large_account:10737418240,small_account:104857600` | Accounts applied at startup, as `id:limit_bytes` (10 GiB and 100 MiB) |
| `ENV_CONFIG_URL` | `https://env.hivenet.com/dev` | Source of `MAX_PARALLEL_CHUNKS` |
| `ENV_CONFIG_REFRESH` | `60s` | Refresh interval |
| `ENV_CONFIG_TIMEOUT` | `3s` | Fetch timeout |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |

## Container

- **Dockerfile**: stage 1 is `golang:1.27` running `CGO_ENABLED=0 go build -trimpath`; stage 2 is `gcr.io/distroless/static:nonroot` (no shell, runs as UID 65532).
- **Volume ownership**: `/data` is created in the image and owned by `65532:65532`. Docker copies that ownership into an empty named volume on first mount; without it, the non-root process cannot write the database.
- **Signals**: `ENTRYPOINT ["/app/api"]` in exec form, so the process is PID 1 and receives SIGTERM directly. Compose waits 15 s (`stop_grace_period`) before killing it, longer than the 10 s shutdown timeout.
- **Health check**: distroless has no curl, so the binary's `healthcheck` subcommand calls `/healthz`. When `HTTP_ADDR` listens on every interface, it calls `127.0.0.1`: Docker runs the check inside the container, next to the server.
- **Compose**:
  - `api`: host port `${API_PORT:-8080}` (Windows can reserve port ranges, so a local `.env` may override it), named volume `data`, and `SEED_ACCOUNTS` passed through from the shell or `.env` (left empty, the service seeds its default accounts);
  - `test`: `golang:1.27` with the source mounted, running `go vet ./...`, `go test -race` with a coverage profile, then `scripts/check-coverage.sh`, which fails below 100%;
  - `demo`: a curl image running `scripts/demo.sh` against `api`.

  `test` and `demo` sit behind profiles and only run when invoked.

## Go conventions

- Small interfaces, declared by the package that uses them.
- No `Interface` / `Impl` pairs; no `utils`, `models` or `common` packages.
- Wiring by hand in `internal/app`; no dependency-injection framework.
- Errors are values: wrapped with `%w` inside the module, inspected with `errors.Is` / `errors.As`. At the boundary with a dependency, the dependency's error is formatted with `%v`. No panics for control flow.
- `context.Context` is the first parameter of every call that does I/O.
- `gofmt`, `go vet` and `golangci-lint` report nothing.
