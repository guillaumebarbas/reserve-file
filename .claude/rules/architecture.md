# Architecture rules

Full description: `docs/architecture.md`.

| Package | Role | May import |
|---|---|---|
| `cmd/api` | Signal handling; calls `app.Run`, nothing else | `app` |
| `internal/app` | Configuration, wiring, startup, graceful shutdown | Every internal package |
| `internal/domain` | Entities, pure rules, errors | Standard library only |
| `internal/service` | Use cases; declares the ports it needs in `ports.go` | `domain` |
| `internal/repository/sqlite` | SQL, transactions, driver error translation | `domain`, `database/sql`, the driver |
| `internal/limits` | `MAX_PARALLEL_CHUNKS`: fetch, validate, cache, refresh | Standard library |
| `internal/handler` | HTTP: decode, validate, call the service, encode, map errors | `service`, `domain` |

## Dependency rule

- Dependencies point inward: `handler → service → domain ← repository/sqlite`.
- Adapters never import each other. Only `internal/app` knows every package, and it wires them by hand.
- `cmd/api/main.go` stays trivial so that all the wiring is testable through `app.Run`.
- An interface is declared by the package that uses it (`service/ports.go`, `limits.Store`), never next to its implementation.

## Where code goes

| Code | Package |
|---|---|
| An admission rule or a calculation on reservations | `domain`, as a pure function with a table test |
| In-flight guard, current limit, clock, ID generation | `service` |
| SQL statements and transactions | `repository/sqlite` |
| JSON, status codes, headers, error messages | `handler` |
| Call to env.hivenet.com and validation of its payload | `limits` |
| Configuration, construction of the components, server lifecycle | `app` |

## Transaction boundary

- A rule that depends on stored state runs inside the repository transaction, through a decide function supplied by `domain` (update-func pattern). Never read in one call and write in another.
- Domain functions receive everything as arguments: no clock, no I/O, no global state.

## Errors

- `domain` defines every error the handler maps: typed errors carrying facts (`InsufficientStorageError`) and sentinels (`ErrAccountNotFound`).
- The repository translates driver errors before returning them.
- The handler is the only place that turns an error into a status, a `code` and a message.

## Avoid

- Interfaces with a single implementation and no test fake; `Interface`/`Impl` pairs.
- Generic repositories, mapper packages, `utils`, `common` or `helpers` packages.
- Dependency-injection frameworks, global variables, `init()` functions.
- Business rules in handlers or in SQL; HTTP or SQL types in `domain` or `service`.
