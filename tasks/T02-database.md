# T02 — Database

| | |
|---|---|
| Issue | [#2](https://github.com/guillaumebarbas/reserve-file/issues/2) |
| Branch | `feat/t02-database` |
| Priority | P0 |
| Estimate | 25 min |
| Depends on | T01 |
| References | [consistency.md](../docs/consistency.md#data-model) (data model, SQLite configuration), D2, D3, D15, D17 |

## Overview

Open the SQLite database with the settings that make it safe, create the schema whose constraints back the invariants, and seed the accounts from configuration. There is no business operation yet: this ticket gives the next ones a database they can trust.

## Technical acceptance criteria

- [ ] `sqlite.Open` opens the file with `journal_mode=WAL`, `synchronous=FULL`, `busy_timeout=5000` and `foreign_keys=ON`, applied to every pooled connection. `_txlock=immediate` comes in T07, driven by a failing concurrency test.
- [ ] `schema.sql` is embedded and applied when `PRAGMA user_version` is 0, which then becomes 1. Opening an up-to-date database changes nothing.
- [ ] Tables `accounts`, `reservations` and `settings` exactly as in consistency.md: `STRICT`, `CHECK`, `UNIQUE` and the foreign key.
- [ ] `SEED_ACCOUNTS` (`id:limit_bytes,…`) is parsed with clear errors. Each account is inserted, or its `storage_limit_bytes` is updated; counters are never touched.
- [ ] `app.Run` opens the database at `DB_PATH` (default `/data/reserve.db`) and seeds it before serving. The database is closed after the HTTP server has stopped.
- [ ] A failure to open, migrate or seed stops the startup with a message naming the step.

## User acceptance criteria

The user is the operator of the service. These checks become visible through the usage endpoint once T05 is done.

- Given a fresh volume, when the service starts, then `acc_demo` (10 GiB) and `acc_small` (100 MiB) exist with nothing reserved.
- Given `acc_small` is changed to 50 MiB in `SEED_ACCOUNTS`, when the service restarts, then its limit is 50 MiB and its reservations are unchanged.
- Given reservations exist, when the container is recreated with the same volume, then they are still there.

## Tests

Unit (`internal/repository/sqlite`):
- [ ] `TestParseSeedAccounts` — table: valid list, empty string, missing colon, non-numeric limit, negative limit, duplicate ID.

Integration (`internal/repository/sqlite`):
- [ ] `TestOpen_AppliesPragmas` — reads back `journal_mode`, `synchronous`, `busy_timeout` and `foreign_keys` on two different connections.
- [ ] `TestOpen_CreatesSchema` — the tables exist and `user_version` is 1.
- [ ] `TestOpen_IsIdempotent` — opening the same file twice keeps the data and the schema.
- [ ] `TestOpen_FailsOnUnusablePath` — a directory given as the file path returns an error.
- [ ] `TestSchema_EnforcesConstraints` — table: `bytes = 0`, unknown status, `released` without `released_at`, negative counter, duplicate `(account_id, idempotency_key)`, unknown account. Each insert is rejected.
- [ ] `TestSeed_CreatesAccounts`
- [ ] `TestSeed_UpsertsLimitOnly` — reseeding with a lower limit keeps the counters.

## Test results

_Not run yet._
