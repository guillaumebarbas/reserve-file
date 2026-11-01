# Design decisions

> Written before implementation and kept up to date as the code lands.

Each decision states what was chosen, why, and what was rejected. **Accepted** decisions are settled; **Proposed** ones are recommended and still open.

How they are implemented: [architecture.md](architecture.md) · [consistency.md](consistency.md) · [api.md](api.md) · [testing.md](testing.md).

## Summary

| ID | Decision | Status |
|---|---|---|
| D1 | Go standard library for HTTP, logs and configuration; no web framework | Accepted |
| D2 | SQLite through the pure-Go `modernc.org/sqlite` driver | Accepted |
| D3 | `database/sql` with handwritten SQL; no ORM | Accepted |
| D4 | Layered (clean) architecture; admission rules run inside the transaction | Accepted |
| D5 | Usage tracked by counters on the account row, updated in the same transaction | Accepted |
| D6 | Writes serialized by SQLite's write lock (`BEGIN IMMEDIATE`) | Accepted |
| D7 | Client-generated `Idempotency-Key`, stored on the reservation | Accepted |
| D8 | `Idempotency-Key` required on reserve | Accepted |
| D9 | A duplicate still in progress gets `409 request_in_progress` | Accepted |
| D10 | Release is `POST …/release`, idempotent, and keeps the row | Accepted |
| D11 | Lowered limits never revoke existing reservations | Accepted |
| D12 | `MAX_PARALLEL_CHUNKS` refreshed in the background; last valid value persisted | Accepted |
| D13 | No valid `MAX_PARALLEL_CHUNKS` yet: new reservations fail closed (`503`) | Accepted |
| D14 | Errors as RFC 9457 problem details with a stable `code` | Accepted |
| D15 | WAL journal with `synchronous=FULL` | Accepted |
| D16 | UUIDv7 reservation identifiers | Accepted |
| D17 | Accounts seeded from configuration; the configured limit is applied at every start | Accepted |
| D18 | Delivered with Docker Compose; tests and demo run in containers | Accepted |
| D19 | Unit, integration and container tests; 100% coverage of the internal packages | Accepted |
| D20 | A rich `Account` aggregate with private counters holds the business rules | Accepted |
| D21 | List only the active reservations, oldest first, without pagination | Accepted |

## D1 — Standard library for HTTP, logs and configuration

**Status:** Accepted

**Decision.** `net/http` with Go 1.22+ routing patterns (`"POST /accounts/{accountID}/reservations"`), `log/slog` with JSON output, configuration from environment variables.

**Why.** Four endpoints. The standard library already provides method and path matching, path parameters, automatic `405` responses and graceful shutdown.

**Rejected.** chi, gin, echo, fiber: nothing needed here is missing from the standard library. Viper: a handful of environment variables does not need it.

## D2 — SQLite with a pure-Go driver

**Status:** Accepted

**Decision.** One SQLite file on a Docker volume, accessed through `modernc.org/sqlite`.

**Why.**
- Allowed by the brief, and there is no database container to run.
- Tests run against a real database file in milliseconds, so transactional and concurrent behaviour is tested for real.
- No cgo: static binary, distroless image, identical build on every host.

**Rejected.**
- PostgreSQL: closer to production, but adds a container and slower tests for no benefit with a single instance. Migration path: [consistency.md, G3](consistency.md#gaps-that-would-block-a-production-release).
- `mattn/go-sqlite3`: requires cgo and a C toolchain.

## D3 — Handwritten SQL, no ORM

**Status:** Accepted

**Decision.** About six handwritten statements through `database/sql`. Schema in an embedded `schema.sql`, versioned with `PRAGMA user_version`.

**Why.** Correctness depends on details an ORM hides: `BEGIN IMMEDIATE`, a guarded `UPDATE … WHERE status = 'active'` and its affected-row count, unique-constraint violations. They should be visible in review.

**Rejected.**
- GORM: these statements would be raw SQL anyway, and its default SQLite driver needs cgo.
- ent: code generation for two tables.
- sqlc: a good fit for a larger query set; here, one more build step.
- Migration tools (goose, migrate): there is one schema version.

## D4 — Layered architecture, rules inside the transaction

**Status:** Accepted

**Decision.** `domain` (the `Account` aggregate, entities, errors) ← `service` (use cases, ports) ← adapters (`repository/sqlite`, `limits`, `handler`). For reserve and release, the repository opens the transaction, loads the `Account` aggregate, calls its method (`Reserve` or `Release`) and saves the result, before committing (see [D20](#d20--a-rich-account-aggregate-with-private-counters)).

**Why.** The admission rule must run in the same transaction as the write. The rule stays in the domain and is tested without a database, while the repository owns atomicity. Ports keep SQLite and the external limit source replaceable, one package each.

**Rejected.**
- The service reads usage, checks it, then calls insert: two statements, so concurrent requests interleave between them and exceed the limit.
- The "update func" pattern of Three Dots Labs (*Wild Workouts*), where the service hands the repository a decision callback: with the rules on the aggregate, the callback would only forward the call, so it adds a function-typed parameter for nothing.
- A transaction manager carrying the transaction in `context.Context` (`@Transactional` style): implicit, and a call made outside it silently writes without a transaction.
- Generic repositories, mapper packages, interfaces with a single implementation, dependency-injection containers.

## D5 — Usage counters on the account row

**Status:** Accepted

**Decision.** `accounts.reserved_bytes` and `accounts.active_reservations` are updated in the same transaction as the reservation insert or release.

**Why.** Admission reads one row, whatever the number of active reservations. That number is bounded by `MAX_PARALLEL_CHUNKS`, a value we do not control (5 today), so the cost of admission must not grow with it.

**Rejected.** `SUM(bytes)` over active reservations on every request: a single source of truth, but its cost grows with the number of active reservations and is paid while holding the write lock, which slows down every write in the service.

**Guards against drift.**
- The counters are private to the `Account` aggregate: only its `Reserve` and `Release` methods change them ([D20](#d20--a-rich-account-aggregate-with-private-counters)).
- Counters are decremented only when the `active → released` transition really happened.
- `CHECK (… >= 0)` on both counters.
- After every database test, the counters are compared with `SUM` / `COUNT` of the active rows.

## D6 — Writes serialized by the SQLite write lock

**Status:** Accepted

**Decision.** Every write transaction starts with `BEGIN IMMEDIATE` (`_txlock=immediate` in the DSN) and waits up to `busy_timeout` (5 s) for the lock. No network I/O happens inside a transaction.

**Why.** `BEGIN IMMEDIATE` takes the write lock up front, so one request's read-check-write sequence cannot interleave with another's. In the default `DEFERRED` mode, two transactions can both read and then fail to upgrade to a write lock with `SQLITE_BUSY`, which `busy_timeout` cannot prevent.

**Rejected.** A Go mutex per account as the correctness mechanism: valid only inside one process, and it duplicates what the database already guarantees.

**Assumption.** One service instance owns the database file, as the brief allows.

## D7 — Client-generated idempotency key

**Status:** Accepted

**Decision.** Reserve requests carry an `Idempotency-Key` header ([draft-ietf-httpapi-idempotency-key-header](https://datatracker.ietf.org/doc/draft-ietf-httpapi-idempotency-key-header/)). Keys are scoped per account and stored on the reservation row under `UNIQUE (account_id, idempotency_key)`, with `bytes` as the request fingerprint.
- Same key, same `bytes`: the existing reservation is returned with `200` and `Idempotent-Replayed: true`.
- Same key, different `bytes`: `422 idempotency_key_reused`.

**Why.** Only the client knows whether two identical bodies are one request sent twice or two different requests: two 4 MiB chunks send exactly the same body. The client must also hold the key before sending: a key issued by the server would need an extra call, whose response could be lost too. Only one operation needs protection, so there is no separate idempotency table.

**Rejected.**
- A server-side hash of the payload: it would merge distinct chunks of the same size.
- A client-chosen reservation ID with `PUT`: it works, but the brief asks the service to return the identifier.

**Limit.** A client that generates a new key for each retry can still reserve twice. The API contract tells clients how to generate keys.

**Security.** The key is not a secret and grants no access; it only deduplicates requests within the caller's own account.
- Keys are looked up per account, so a key guessed or leaked from another account matches nothing. Once authentication exists ([G4](consistency.md#gaps-that-would-block-a-production-release)), the account, and therefore the scope of the key, comes from the token instead of the path.
- Replaying a captured request returns the existing reservation and creates nothing. TLS prevents the capture itself.
- A client that sends a new key with every request can only use up its own account's storage and slots. Table growth is covered by retention (G2) and rate limiting (G5).
- The header is validated (1 to 255 visible ASCII characters), SQL statements are parameterized, and logs are JSON-encoded.
- In-flight guard entries live only for the duration of a request, so keys cannot fill the service's memory.

## D8 — Idempotency key required

**Status:** Accepted

**Decision.** A reserve request without a valid key gets `400 invalid_idempotency_key`.

**Why.** Without a key, a lost response cannot be retried safely. Requiring it makes the safe path the only one.

**Rejected.** An optional key: convenient for manual calls, but retries become silently unsafe.

## D9 — In-flight duplicates get 409

**Status:** Accepted

**Decision.** The service keeps an in-memory set of in-flight `(account_id, idempotency_key)` pairs. A request whose pair is already in flight gets `409 request_in_progress` with `Retry-After: 1`, without touching the database. The pair is released with `defer`, including on error or panic.

**Why.** This is the behaviour the IETF draft specifies, and it sheds load during retry storms instead of queueing requests on the write lock.

**Rejected.**
- Waiting for the lock, then replaying: correct, but blocked requests pile up.
- A persisted `pending` state: a crash would leave keys stuck in `pending`, which then needs a recovery mechanism.

**Note.** The guard does not provide consistency; the `UNIQUE` constraint and the transaction do. If the guard were bypassed (a bug, a second instance), no duplicate could be created.

## D10 — Release endpoint

**Status:** Accepted

**Decision.** `POST /accounts/{accountID}/reservations/{reservationID}/release`. Repeating it returns `200` with the same body. The row stays, with `status = 'released'`.

**Why.** Release is a state transition, not a deletion: the row answers replays of the original reserve request, and `GET` keeps returning it.

**Rejected.** `DELETE`: idempotent by HTTP semantics and defensible, but a resource that stays readable after `DELETE` is surprising.

## D11 — Lowered limits never revoke reservations

**Status:** Accepted

**Decision.** If an account's storage limit or `MAX_PARALLEL_CHUNKS` drops below current usage, existing reservations stay active and new ones are refused until usage is back under the limit. `available_bytes` is reported as `max(0, limit − reserved)`. The schema has no `CHECK (reserved_bytes <= storage_limit_bytes)`, because this state is legitimate.

**Why.** A reservation is a promise to a client that may be uploading. Revoking it would break uploads in progress and would require notifying clients, which is out of scope. Limits gate admissions; they do not take back what was granted.

## D12 — External limit refreshed in the background

**Status:** Accepted

**Decision.** `MAX_PARALLEL_CHUNKS` is never fetched on the request path.
- At startup, the last valid value is loaded from the database, then `https://env.hivenet.com/dev` is fetched (3 s timeout, 1 MiB body cap).
- The value is refreshed every 60 s. While no value has ever been obtained, a failed fetch is retried sooner, with a backoff from 1 s doubling up to 30 s; once a value is known, a failure keeps it and the next fetch comes after the normal 60 s.
- A value is accepted only if it is a JSON integer ≥ 1. Anything else keeps the previous value and logs a warning.
- Each new value is persisted.

**Why.** The external service must add neither latency nor a failure mode to reserve, release or usage. A malformed payload must never set the limit to 0 or to "unlimited". A restart during an outage must keep a real value.

**Note.** The payload also contains analytics tokens. It is never logged.

## D13 — Fail closed until the limit is known

**Status:** Accepted

**Decision.** As soon as one valid value exists, in memory or in the `settings` table, it is used, even if the source goes down afterwards or the service restarts during the outage. Only when no valid value has ever been obtained (first start during an outage) do new reservations get `503 limit_unavailable` with `Retry-After`. Release, usage, `GET` and idempotent replays keep working.

**Why.** Admitting reservations without the limit can exceed it. Refusing new reservations during a first-start outage is the smaller harm.

**Rejected.**
- A configured default: it may disagree with the source.
- Refusing to start: it would also block release and usage.

## D14 — Problem details errors

**Status:** Accepted

**Decision.** Errors are `application/problem+json` ([RFC 9457](https://www.rfc-editor.org/rfc/rfc9457)) with:
- a stable machine-readable `code`;
- a `detail` sentence that gives the numbers and the next step;
- extension fields carrying the values (requested bytes, available bytes…).

Domain errors carry facts; the HTTP layer writes the text. `5xx` responses never expose internal errors; they carry a `request_id` that matches the server logs.

**Why.** Clients branch on `code`, humans read `detail`, and the wording can change without breaking clients. Catalogue: [api.md, Errors](api.md#errors).

## D15 — Durability settings

**Status:** Accepted

**Decision.** `journal_mode=WAL`, `synchronous=FULL`, `foreign_keys=ON`, `STRICT` tables.

**Why.**
- WAL lets reads proceed while a write is in progress.
- `synchronous=FULL` makes a committed transaction survive a power loss. With `NORMAL`, the service could answer `201` for a reservation that disappears after a crash.
- Foreign keys are off by default in SQLite.
- `STRICT` rejects values of the wrong type instead of storing them.

## D16 — UUIDv7 identifiers

**Status:** Accepted

**Decision.** Reservation IDs come from `github.com/google/uuid` (`NewV7`).

**Why.** Generated without coordination and ordered by creation time, which keeps index inserts local and listings naturally sorted.

## D17 — Accounts seeded from configuration

**Status:** Accepted

**Decision.** `SEED_ACCOUNTS` (for example `large_account:10737418240,small_account:104857600`) is applied at every startup. Missing accounts are created; `storage_limit_bytes` is updated if it changed; counters are never touched.

**Why.** No account-management API is in scope. Re-applying the configured limit on restart also makes the lowered-limit case (D11) easy to demonstrate.

## D18 — Docker Compose delivery

**Status:** Accepted

**Decision.** `docker compose up` starts the service.
- Multi-stage Dockerfile: `golang:1.27` builder with `CGO_ENABLED=0`, then a `gcr.io/distroless/static:nonroot` runtime.
- The SQLite file lives on a named volume.
- Compose services `test` (`go vet`, `go test -race` and the coverage check) and `demo` (curl script) run on demand, so a reviewer needs only Docker.

**Rejected.** A Dev Container: the brief asks for one of the two, and Compose starts everything with one command.

## D19 — Unit, integration and container tests

**Status:** Accepted

**Decision.**
- Unit tests for code without I/O: domain rules, services with hand-written fakes, pure helpers.
- Integration tests with a real SQLite file, `httptest` servers, the full HTTP stack in-process, and `app.Run`.
- Container tests against the Docker image started through `compose.yaml`, driven by `go test -tags e2e` and the `docker compose` CLI.
- Unit and integration tests together cover 100% of the statements in `./internal/...`, checked by `docker compose run --rm test`. `cmd/api/main.go` only calls `app.Run` and is outside the metric.

**Why.** Unit tests pin the rules cheaply. Integration tests prove transactions, constraints and concurrency against the real database. Container tests check what only the packaged service shows: the non-root user and its volume, restarts, SIGTERM and SIGKILL. The 100% target forces every error branch to be reproduced by a test, and error branches are where this service's failure handling lives.

**Rejected.**
- Testcontainers: its main use, starting a real database for the tests, does not apply to an embedded SQLite. Driving the same `compose.yaml` as the reviewer also tests the reviewer's path, without a new dependency.
- Mocking the database: it would prove neither locking, nor constraints, nor atomicity.

**Consequence.** Coverage shows which lines ran, not that they are right: assertions and invariant checks carry the proof. A branch that no meaningful test can reach is dead code and is removed.

## D20 — A rich Account aggregate with private counters

**Status:** Accepted

**Decision.** The business rules are methods of the domain types that own the data. `Account` is the aggregate root:
- `account.Reserve(candidate, existing, maxActive)` answers a reservation request. A retry of a known key is answered first: replay, or `IdempotencyKeyReusedError` if the size differs. Otherwise the candidate is admitted only if a slot and enough space are free, and the counters are updated.
- `account.Release(reservation, at)` marks an active reservation released and gives its space back. On a reservation already released, it changes nothing.
- `account.AvailableBytes()` is the space left, never negative.

Its counters are unexported fields, so only these methods change them. The repository rebuilds an account with `domain.RestoreAccount(…)` and reads its counters through the accessors `ReservedBytes()` and `ActiveReservations()`. `Reservation` stays a plain struct.

In memory, the aggregate is the account row and, when needed, the one reservation concerned. It never holds the account's whole list of reservations: the counters summarize them ([D5](#d5--usage-counters-on-the-account-row)).

**Why.**
- The code reads like the domain: an account reserves and releases.
- The compiler guarantees that the counters change only through `Reserve` and `Release`: no stray assignment elsewhere can break invariants I1 and I2.
- The methods are pure, so the rules are tested without a database.

**Rejected.**
- A free decision function over a passive state (`DecideReserve(state, …)`), the previous design: as testable, but the rules sit apart from the data they protect, and nothing stops other code from changing the counters.
- Encapsulating every type, `Reservation` included: an accessor for every field, Java-bean style, with no invariant to protect.
- Loading all the reservations of an account: unbounded memory ([P14](consistency.md#p14--max_parallel_chunks-becomes-very-large)).

## D21 — List only the active reservations, without pagination

**Status:** Accepted

**Decision.** `GET /accounts/{accountID}/reservations` answers `{"reservations": [...]}` with the account's active reservations, oldest first (`created_at`, then `id`), each with the body of a single read, and `[]` when none is active. The service checks the account first, so an unknown account gets `404 account_not_found`; the repository then lists in one `SELECT`. Nothing is written, so neither read runs in a transaction: accounts are never deleted, and one statement is consistent on its own. The list is all or nothing: SQLite sorts every matching row before it returns the first one, so an error on any row fails the query itself.

**Why.** Reservations do not expire ([G1](consistency.md#gaps-that-would-block-a-production-release)). A client that lost its reservation IDs, or an operator, needs to find the reservations that hold space and slots in order to release them. An account has at most `MAX_PARALLEL_CHUNKS` active reservations, a few more only after the limit was lowered, so one response holds them all. The brief does not ask for this endpoint: it was added at the end, as an extra.

**Rejected.**
- Listing released reservations too: they are history kept for replays, they grow without bound until a retention window exists ([G2](consistency.md#gaps-that-would-block-a-production-release)), and they would need pagination. They can come later as `?status=released` with a cursor, without changing this contract.
- A bare JSON array: the object leaves room for a cursor.
- Loading the list into the `Account` aggregate: its counters already summarize the reservations ([D5](#d5--usage-counters-on-the-account-row)), and no rule needs the list ([D20](#d20--a-rich-account-aggregate-with-private-counters)).

**Limit.** The account filter uses the index of `UNIQUE (account_id, idempotency_key)`, so the query also reads the account's released rows; a partial index on the active rows would remove that cost if it ever mattered. If `MAX_PARALLEL_CHUNKS` became very large ([P14](consistency.md#p14--max_parallel_chunks-becomes-very-large)), this list would need pagination too.

## Patterns considered and not used

| Pattern | Solves | Why not here | Becomes relevant if… |
|---|---|---|---|
| Saga | An operation spanning several services, undone by compensations | All state is in one database; each operation is one local transaction | Reserving must also allocate space on storage nodes or call billing |
| Transactional outbox | Writing to the database and publishing a message atomically | Nothing is published; message brokers are out of scope | Other services must be notified of reservations and releases |
| Persisted `pending` idempotency state | Operations spanning several transactions or external calls | Our operation is one short transaction | Reserve has to call another service before committing |
| Per-account lock in Go | Serializing access to an account | The database lock already serializes writes, across processes too | Never as the only mechanism |
| `429 Too Many Requests` for the slot limit | Rate limiting | The slot limit is not time-based; there is no meaningful `Retry-After` | — |
| `507 Insufficient Storage` | The server's own storage is exhausted (WebDAV) | Our refusal is a business rule on the account | — |

This service is one step of a larger flow: reserve, upload the chunks, then commit the upload (out of scope), with release as the compensation. It already provides what a saga step needs: an idempotent action and an idempotent compensation. What is missing is the timeout for when the client dies, that is, reservation expiry ([G1](consistency.md#gaps-that-would-block-a-production-release)).
