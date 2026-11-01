# Consistency and failure handling

> Written before implementation and kept up to date as the code lands.

What can go wrong, and how the design prevents it. Each behaviour names the test that proves it ([testing.md](testing.md)).

## In short

- All state lives in SQLite. Every operation is **one** short write transaction started with `BEGIN IMMEDIATE`, so writes run one at a time and each one sees the state left by the previous one.
- The usage counters on the account row change in the same transaction as the reservation, and only when a reservation actually changes state.
- Retries are made safe by a client-generated `Idempotency-Key`, checked **before** any other rule.
- The external `MAX_PARALLEL_CHUNKS` is never fetched on the request path; the last valid value is cached and persisted.

## Assumptions

| ID | Assumption |
|---|---|
| A1 | One service instance; one process owns the SQLite file. |
| A2 | No network I/O happens inside a database transaction. |
| A3 | Clients send the same `Idempotency-Key` on every retry of one logical request, and a new key for a new request. |
| A4 | Clients retry with the same key after no response, `409 request_in_progress` or `503`. |

## Invariants

These hold after every committed transaction.

| ID | Invariant | Enforced by |
|---|---|---|
| I1 | `reserved_bytes` = sum of `bytes` of the account's active reservations | Counters private to the `Account` aggregate, changed only by `Reserve` and `Release`; saved in the same transaction as the reservation; decremented only on a real `active → released` transition |
| I2 | `active_reservations` = number of the account's active reservations | Same as I1 |
| I3 | A reservation moves from `active` to `released` at most once, and never back | `Account.Release` changes nothing on a released reservation; `UPDATE … WHERE status = 'active'` as a second guard; a `CHECK` tying `status` to `released_at` |
| I4 | At most one reservation per `(account_id, idempotency_key)` | `UNIQUE` constraint |
| I5 | A reservation is admitted only if, at that moment, `active_reservations < MAX_PARALLEL_CHUNKS` and `bytes ≤ limit − reserved_bytes` | `Account.Reserve`, called inside a `BEGIN IMMEDIATE` transaction |
| I6 | Counters are never negative; `bytes > 0` | `CHECK` constraints |

I5 constrains admissions, not the stored state: after a limit is lowered, usage may legitimately exceed it ([P7](#p7--a-limit-drops-below-current-usage)). I1 and I2 are checked with SQL after every database test.

## Data model

```sql
CREATE TABLE accounts (
  id                  TEXT    PRIMARY KEY,
  storage_limit_bytes INTEGER NOT NULL CHECK (storage_limit_bytes >= 0),
  reserved_bytes      INTEGER NOT NULL DEFAULT 0 CHECK (reserved_bytes >= 0),
  active_reservations INTEGER NOT NULL DEFAULT 0 CHECK (active_reservations >= 0)
) STRICT;

CREATE TABLE reservations (
  id              TEXT    PRIMARY KEY,                    -- UUIDv7
  account_id      TEXT    NOT NULL REFERENCES accounts(id),
  idempotency_key TEXT    NOT NULL,
  bytes           INTEGER NOT NULL CHECK (bytes > 0),
  status          TEXT    NOT NULL CHECK (status IN ('active', 'released')),
  created_at      INTEGER NOT NULL,                       -- Unix milliseconds, UTC
  released_at     INTEGER,
  CHECK ((status = 'released') = (released_at IS NOT NULL)),
  UNIQUE (account_id, idempotency_key)
) STRICT;

CREATE TABLE settings (
  key        TEXT    PRIMARY KEY,                         -- 'max_parallel_chunks'
  value      TEXT    NOT NULL,
  updated_at INTEGER NOT NULL
) STRICT;
```

- **Counters on `accounts`**: admission reads one row, whatever the number of active reservations ([D5](decisions.md#d5--usage-counters-on-the-account-row)). In Go, they are private fields of the `Account` aggregate ([D20](decisions.md#d20--a-rich-account-aggregate-with-private-counters)).
- **No `CHECK (reserved_bytes <= storage_limit_bytes)`**: that state is legitimate after a limit is lowered ([D11](decisions.md#d11--lowered-limits-never-revoke-reservations)).
- **The idempotency key lives on the reservation**: only one operation needs it, and `bytes` serves as the request fingerprint.
- **Released rows are kept**: they answer replays and `GET`. Their retention is gap [G2](#gaps-that-would-block-a-production-release).
- **`settings`** holds the last valid `MAX_PARALLEL_CHUNKS`.

## SQLite configuration

| Setting | Value | Why |
|---|---|---|
| `_txlock` | `immediate` | The write lock is taken at `BEGIN`, so read-check-write sequences cannot interleave, and a transaction never fails while upgrading from a read lock to a write lock |
| `journal_mode` | `WAL` | Reads do not wait for the writer |
| `synchronous` | `FULL` | A committed transaction survives a power loss; with `NORMAL` in WAL mode, the last commits can be lost |
| `busy_timeout` | `5000` ms | A writer waits for the lock instead of failing at once; past the timeout, the request gets `503` |
| `foreign_keys` | `ON` | Off by default in SQLite |

```
/data/reserve.db?_txlock=immediate&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=foreign_keys(1)
```

With `modernc.org/sqlite`, `_pragma` values are applied to every connection the pool opens. `_txlock` is a driver option, not a pragma: every `BeginTx` then sends `BEGIN IMMEDIATE`, while a read-only transaction still sends a plain `BEGIN`.

## Reserve transaction

The idempotency lookup comes before every admission rule. A retry of an accepted request therefore succeeds even if the account has since filled up, all slots are used, or the limit source is down.

| Step | Where | Check | Result |
|---|---|---|---|
| 0 | service | Same `(account, key)` already in flight | `409 request_in_progress`, no database access |
| 1 | repository | The account does not exist | `404 account_not_found` |
| 2 | `Account.Reserve` | A reservation exists for `(account, key)` with the same `bytes` | Replay: `200` with its current state |
| 3 | `Account.Reserve` | … with different `bytes` | `422 idempotency_key_reused` |
| 4 | `Account.Reserve` | `MAX_PARALLEL_CHUNKS` never obtained | `503 limit_unavailable` |
| 5 | `Account.Reserve` | `active_reservations ≥ MAX_PARALLEL_CHUNKS` | `409 too_many_active_reservations` |
| 6 | `Account.Reserve` | `bytes > storage_limit_bytes − reserved_bytes` | `409 insufficient_storage` |
| 7 | `Account.Reserve`, then repository | None | The counters grow; the repository inserts the reservation, saves the counters and commits: `201` |

Steps 1 to 7 run between `BEGIN IMMEDIATE` and `COMMIT`; steps 2 to 7 are `Account.Reserve`, a pure method.
- Step 1 comes before the idempotency lookup but never hides a replay: the foreign key guarantees that no reservation exists for an unknown account.
- Step 6 subtracts instead of adding, so it cannot overflow. It also refuses every request when usage is above a lowered limit, because the difference is then negative.
- The repository saves the counters as absolute values taken from the aggregate. It is safe because `BEGIN IMMEDIATE` holds the write lock from the load to the commit: no other write can come in between.

## Release transaction

In one `BEGIN IMMEDIATE` transaction:

1. Load the account, as reserve does. Unknown: `404 account_not_found`.
2. Load the reservation by `id` and `account_id`. Not found, or owned by another account: `404 reservation_not_found`.
3. Call `account.Release(&reservation, now)`. For an active reservation, it marks it released and gives its `bytes` and its slot back. For a released one, it changes nothing and returns `false`: the transaction ends without writing, and the call answers `200` with the unchanged reservation, `released_at` included.
4. When it changed something, save it:

```sql
UPDATE reservations
   SET status = 'released', released_at = :now
 WHERE id = :id AND account_id = :account AND status = 'active';
-- must change exactly one row: a second guard behind Account.Release

UPDATE accounts
   SET reserved_bytes = :reserved_bytes, active_reservations = :active_reservations
 WHERE id = :account;
```

The space comes back only on a real `active → released` transition, so a second release has nothing to give back.

## Problems and solutions

### P1 — Concurrent reservations exceed the limit

**Problem.** Two requests both read "60 MiB available" and both reserve 40 MiB: 80 MiB end up reserved on a 60 MiB budget. This is the classic check-then-act race.

**Solution.** The check and the write are in the same `BEGIN IMMEDIATE` transaction. SQLite admits one writer at a time, so the second request reads the state left by the first.

Concurrent requests with different keys are not refused for being concurrent: parallel chunks and multiple devices are the normal case. They are processed one after the other, each against the real state.

Measured before `_txlock=immediate` (T07): in WAL mode the limit was never exceeded, because a transaction that read an older snapshot cannot write. But 7 to 19 of 50 concurrent requests failed to upgrade their read lock to a write lock, with `SQLITE_BUSY`, and got `503` instead of their real answer: `busy_timeout` cannot help a transaction whose snapshot is stale. With `BEGIN IMMEDIATE`, each request waits for the lock, then reads the state left by the previous one.

**Proved by.** `TestReserve_ConcurrentRequestsNeverExceedLimit`, `TestReserve_ConcurrentRequestsNeverExceedSlots`, `TestAPI_ConcurrentReservations`, `TestReserveRelease_ConcurrentMix`.

### P2 — The response is lost and the client retries

**Problem.** The reservation was committed but the client never got the response (timeout, network cut, crash after commit). A naive retry reserves the space twice and takes a second slot.

**Solution.** The retry carries the same `Idempotency-Key`. The key lookup (step 2) finds the reservation and returns it in its current state, with `200` and `Idempotent-Replayed: true`: no extra bytes, no extra slot. A refused or failed attempt stores nothing, so its retry is evaluated again.

**Proved by.** `TestAccount_Reserve`, `TestReserve_RetryReturnsSameReservation`, `TestReserve_RefusedAttemptIsNotRemembered`, `TestAPI_ReserveRetry`.

### P3 — The same request arrives twice at the same time

**Problem.** A double click, or a client retrying while the original request is still being processed.

**Solution.** The in-flight guard answers the second request with `409 request_in_progress` and `Retry-After: 1`, without touching the database. The client's next retry is a replay.

The guard is a convenience. The `UNIQUE (account_id, idempotency_key)` constraint is what makes a second row impossible. If the constraint ever fires (it cannot with one instance), the request also gets `409 request_in_progress`.

**Proved by.** `TestService_DuplicateInFlightReturnsInProgress`, `TestReserve_ConcurrentSameKeyCreatesOneRow`, `TestReserve_UniqueViolationMapsToInProgress`, `TestAPI_ConcurrentSameKey`.

### P4 — A key is reused for a different request

**Problem.** A client bug sends a known key with another size. Returning the stored reservation would silently hand back the wrong amount of space.

**Solution.** `bytes` is stored with the key. `account.Reserve` compares it right after the key lookup, before every admission rule, so the answer never depends on the account's state. A mismatch returns `422 idempotency_key_reused`, with the original `bytes` in the response, and writes nothing.

**Proved by.** `TestAccount_Reserve`, `TestReserve_KeyReuseWithDifferentBytes`, `TestAPI_KeyReuseWithDifferentBytes`.

### P5 — A retry arrives after the situation changed

**Problem.** When the retry arrives, the account may be full, all slots may be taken (possibly by this very reservation), the limit source may be down, or the reservation may already be released.

**Solution.** The idempotency lookup comes before every other check, so all these retries return the original reservation. A released reservation is returned as released, with its `released_at`, and never created again. The reserve transaction still reads the account row, which must exist, and the service still reads the cached `MAX_PARALLEL_CHUNKS`, but the answer to a known key is decided before any limit, slot or space rule uses them.

**Proved by.** `TestAccount_Reserve` (replay cases), `TestReserve_ReplayOfReleasedReservation`, `TestReserve_ReplayWhenAccountFull`, `TestReserve_ReplayWhenEverySlotIsUsed`, `TestReserve_ReplayWhenLimitUnknown`, `TestAPI_ReplayOfReleasedReservation`, `TestAPI_ReplayWhenAccountFull`, `TestAPI_ReplayWhenLimitUnknown`.

### P6 — Double release and concurrent releases

**Problem.** Releasing twice, whether from a retry or from two devices, must not give the space back twice.

**Solution.** `Account.Release` gives the space back only on a real `active → released` transition, and changes nothing on a reservation already released. The `UPDATE … WHERE status = 'active'` that saves it is a second guard. Later calls return `200` with the same body.

An unknown reservation, or one owned by another account, gets `404` rather than `403`, so the service does not reveal that it exists.

With `BEGIN IMMEDIATE`, the reservation is read under the write lock, so the second guard can only refuse a row that something else changed in between; `TestRelease_RollsBackWhenTheRowIsNotUpdated` simulates that with a trigger, and the whole release rolls back.

**Proved by.** `TestAccount_ReleaseTwice`, `TestRelease_Twice`, `TestRelease_ConcurrentReleasesCreditOnce`, `TestReserveRelease_ConcurrentMix`, `TestRelease_RollsBackWhenTheRowIsNotUpdated`, `TestRelease_OtherAccountIsNotFound`, `TestAPI_ReleaseIdempotent`.

### P7 — A limit drops below current usage

**Problem.** The account's storage limit is lowered below `reserved_bytes` (a plan change), or `MAX_PARALLEL_CHUNKS` is lowered below `active_reservations`.

**Solution.** Nothing is revoked ([D11](decisions.md#d11--lowered-limits-never-revoke-reservations)). New reservations are refused until usage is back under the limit; release, usage and replays keep working. `available_bytes` is reported as `max(0, limit − reserved)`.

**Proved by.** `TestReserve_AfterStorageLimitLowered`, `TestReserve_AfterMaxActiveLowered`, `TestAccount_AvailableBytesNeverNegative`.

### P8 — env.hivenet.com is down, slow or returns garbage

The value is never fetched on the request path ([D12](decisions.md#d12--external-limit-refreshed-in-the-background)).

| Situation | Behaviour |
|---|---|
| Down, `5xx` or slower than 3 s, after a value was obtained | The last valid value is kept (memory and database), a warning is logged without the payload, and the next fetch comes after the normal 60 s: a valid value is in use, so there is no hurry |
| The service restarts during the outage | The last valid value is reloaded from `settings`. A save that failed is retried at the next refresh, so the stored value does not lag behind |
| First start during an outage, nothing stored | New reservations get `503 limit_unavailable` with `Retry-After`; release, usage, `GET` and replays are unaffected ([D13](decisions.md#d13--fail-closed-until-the-limit-is-known)). The fetch is retried after 1 s, then 2, 4, 8, 16 and every 30 s, until a value is obtained; from then on the normal 60 s applies |
| Key missing, `"5"`, `5.5`, `0`, a negative number, invalid JSON, body over 1 MiB | The value is rejected and the previous one kept |
| The value increases | It applies from the next refresh (within 60 s) |
| The value decreases | See P7 |

The payload contains analytics tokens and is never logged.

**Proved by.** `TestFetcher_RejectsInvalidResponses`, `TestLimits_KeepsLastValidValueOnFailure`, `TestLimits_KeepsLastValidValueWhenSourceTimesOut`, `TestLimits_NeverLogsBody`, `TestLimits_ReloadsPersistedValue`, `TestLimits_RetriesSaveAfterFailure`, `TestLimits_RetriesWithBackoffWhileUnknown`, `TestLimits_UnknownUntilFirstSuccess`, `TestAPI_ReserveWhenLimitUnknown`.

### P9 — The database is unavailable

**Problem.** The lock is held longer than `busy_timeout`, the disk is full, an I/O error occurs, or the database is closed.

**Solution.** Each operation is a single transaction: on error it rolls back, so nothing is partially applied. The repository maps these errors to `ErrServiceUnavailable`, which the API returns as `503 service_unavailable` with `Retry-After: 1`. Retrying with the same key is safe. `/readyz` reports the database as down.

**Proved by.** `TestReserve_DatabaseLockedReturnsUnavailable`, `TestRepository_ClosedDatabaseReturnsUnavailable`, `TestRepository_ReadOnlyDatabaseReturnsUnavailable`, `TestRepository_DiskFullReturnsUnavailable`, `TestReserve_CounterUpdateFailureRollsBack`, `TestRelease_CounterUpdateFailureRollsBack`, `TestAPI_DatabaseClosedReturns503`.

### P10 — Crashes

| When | Outcome |
|---|---|
| During a transaction | SQLite discards it when the file is next opened |
| After `COMMIT`, before the response | The client retries with the same key and gets a replay (P2) |
| Power loss after a `201` | `synchronous=FULL`: the commit is on disk |

**Proved by.** `TestRepository_ReopenKeepsState`, `TestContainer_SigkillKeepsCommittedReservations` and the P2 tests. Atomic commit is SQLite's own guarantee and is not re-tested.

### P11 — The client disconnects mid-request

The request context is cancelled. The transaction either commits or rolls back; it never half-applies. The client cannot tell which, so it retries with the same key and gets either a replay or a fresh execution. There is no duplicate in either case.

**Proved by.** `TestReserve_CancelledContextRollsBack`.

### P12 — Restart and shutdown

Everything needed for correctness is in the database: the accounts and their counters, the reservations with their idempotency keys and states, and the last valid limit.
- After a restart, usage is the same, a retry with a known key is replayed, and a reservation released before the restart is replayed as released.
- The in-flight guard starts empty after a restart, which is correct since no request survives one.
- The limit is reloaded from `settings`.

On SIGTERM, the application context is cancelled. The HTTP server stops accepting connections and lets in-flight requests finish (up to 10 s), while the limits refresher, which runs on the same context, stops. `app` waits for both, then closes the database last, so an in-flight request still reads and writes an open database. If the server stops on its own, `app` stops the refresher once the server has stopped, and the database is still closed last.

**Proved by.** `TestRepository_ReopenKeepsState`, `TestRun_DrainsInFlightRequestsOnShutdown`, `TestLimits_ReloadsPersistedValue`, `TestContainer_RestartKeepsReservations`, `TestContainer_SigtermStopsGracefully`.

### P13 — Invalid input

| Input | Response |
|---|---|
| `bytes` missing, `null`, `0`, negative, decimal, an exponent, a string, or above 2⁶³−1 | `400 invalid_request`, with a `detail` showing the value received |
| Unknown JSON field, or trailing data after the object | `400 invalid_request` |
| Body over 4 KiB | `413 request_too_large` |
| `Idempotency-Key` missing, empty, over 255 characters, or with a character outside visible ASCII (`0x21` to `0x7E`: no space, no tab, no accented letter) | `400 invalid_idempotency_key`, with a `detail` naming the rule |
| Unknown account or reservation | `404` |
| `bytes` above the account's entire limit | `409 insufficient_storage` |

Every check runs in the handler, before the service is called, so an invalid request never reaches the database: its key stays unused.

**Proved by.** `TestAPI_ReserveValidation`, `TestAPI_ReserveStoresNothingForAnInvalidRequest`, `TestValidateIdempotencyKey_RejectsInvalidKeys`, `TestReadReserveRequest_RejectsInvalidBytes`, `TestAccount_Reserve`.

### P14 — MAX_PARALLEL_CHUNKS becomes very large

**Problem.** If reserved space were computed with `SUM(bytes)`, each reservation would read every active row while holding the write lock. At 5 rows that costs nothing; at 1,000,000, every write in the service slows down.

**Solution.** Counters on the account row ([D5](decisions.md#d5--usage-counters-on-the-account-row)): admission reads one row, and the idempotency lookup uses the `UNIQUE` index. The cost does not depend on the number of active reservations.

### P15 — The client generates a new key for each retry

The service cannot prevent this, because it cannot tell such a retry from a new request of the same size. The API contract tells clients to generate the key once per logical request ([api.md](api.md#idempotency)).

**Proved by.** `TestReserve_DifferentKeysCreateDifferentReservations`: two keys give two reservations, as the contract says.

## Gaps that would block a production release

| ID | Gap | Impact | Direction |
|---|---|---|---|
| G1 | No reservation expiry (out of scope for this exercise) | A client that crashes keeps its reservation forever; `MAX_PARALLEL_CHUNKS` such crashes block the account | An `expires_at` per reservation; expired reservations released inside the admission transaction and by a periodic sweep; an endpoint to extend long uploads |
| G2 | Idempotency keys and released rows are kept forever | The table grows without bound | A retention window (for example 24 h) after which released rows and their keys are purged |
| G3 | Single instance | The in-flight guard is per process, and the SQLite file is local | PostgreSQL, with admission through `SELECT … FOR UPDATE` on the account row or a conditional `UPDATE`; the `UNIQUE` constraint keeps idempotency correct across instances |
| G4 | No authentication | Anyone can reserve or release on any account | The account comes from the authenticated identity (Zitadel, according to the environment configuration), not from the path |
| G5 | No rate limiting, metrics or tracing | Abuse and incidents go unnoticed | A per-account rate limit; metrics for admissions, refusals by code, lock wait time and the age of the limit value |
| G6 | No backup of the SQLite file | Losing the volume loses every reservation | Backups or replication, or a managed PostgreSQL |
