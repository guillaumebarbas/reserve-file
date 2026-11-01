# Architecture rules

Full description: `docs/architecture.md`.

| Package | Role | May import |
|---|---|---|
| `cmd/api` | Signal handling; calls `app.Run`, nothing else | `app` |
| `internal/app` | Configuration, wiring, startup, graceful shutdown | Every internal package |
| `internal/domain` | The `Account` aggregate with its rules as methods, entities, errors | Standard library only |
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
| A business rule (admission, release, available space) | `domain`, as a method of the type that owns the data, mostly the `Account` aggregate, with a table test |
| In-flight guard, current limit, clock, ID generation | `service` |
| SQL statements and transactions | `repository/sqlite` |
| JSON, status codes, headers, error messages | `handler` |
| Call to env.hivenet.com and validation of its payload | `limits` |
| Configuration, construction of the components, server lifecycle | `app` |

## Domain model: objects, the Go way

Decision [D20](../../docs/decisions.md#d20--a-rich-account-aggregate-with-private-counters).

- Data and the rules that protect it live together. A business rule is a method of the domain type that owns the data (`account.Reserve`, `account.Release`, `account.AvailableBytes`), not a free function, a service or a handler acting on someone else's fields.
- `Account` is the aggregate root: reservations are made and released through it, so its counters always match its active reservations. One repository loads and saves the aggregate, in one transaction.
- Encapsulate only where an invariant needs it. Fields that only the aggregate may change are unexported; adapters rebuild the aggregate with a `Restore…` constructor and read it through accessors named after the field, without `Get`. No setters.
- Plain data (`Reservation`, DTOs, rows) stays a struct with exported fields: no accessor without an invariant to protect.
- Methods are pure: everything they need comes in as arguments, including the time and the IDs. A refusal is a returned typed error carrying its facts, never a panic.
- Objects the Go way, not the Java way: no inheritance through embedding, no interface for a domain type, no constructor overloading, no `Manager` or `Helper` types.

## Transaction boundary

- A rule that depends on stored state runs inside the repository transaction: the repository loads the `Account` aggregate, calls its method (`Reserve`, `Release`) and saves the result before committing. Never read in one call and write in another.
- Domain methods receive everything as arguments: no clock, no ID generation, no I/O, no global state.

## Errors

- `domain` defines every error the handler maps: typed errors carrying facts (`InsufficientStorageError`) and sentinels (`ErrAccountNotFound`).
- The repository translates driver errors before returning them.
- The handler is the only place that turns an error into a status, a `code` and a message.

## Avoid

- Interfaces with a single implementation and no test fake; `Interface`/`Impl` pairs.
- Generic repositories, mapper packages, `utils`, `common` or `helpers` packages.
- Dependency-injection frameworks, global variables, `init()` functions.
- Business rules in handlers, services or SQL; HTTP or SQL types in `domain` or `service`.
- Passive domain types whose rules live elsewhere, and code that changes an aggregate's state without going through its methods.
