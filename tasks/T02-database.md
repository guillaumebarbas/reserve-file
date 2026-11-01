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

- [x] `sqlite.Open` opens the file with `journal_mode=WAL`, `synchronous=FULL`, `busy_timeout=5000` and `foreign_keys=ON`, applied to every pooled connection. `_txlock=immediate` comes in T07, driven by a failing concurrency test.
- [x] `schema.sql` is embedded and applied when `PRAGMA user_version` is 0, which then becomes 1. Opening an up-to-date database changes nothing.
- [x] Tables `accounts`, `reservations` and `settings` exactly as in consistency.md: `STRICT`, `CHECK`, `UNIQUE` and the foreign key.
- [x] `SEED_ACCOUNTS` (`id:limit_bytes,…`) is parsed with clear errors. Each account is inserted, or its `storage_limit_bytes` is updated; counters are never touched.
- [x] `app.Run` opens the database at `DB_PATH` (default `/data/reserve.db`) and seeds it before serving. The database is closed after the HTTP server has stopped.
- [x] A failure to open, migrate or seed stops the startup with a message naming the step.

## User acceptance criteria

The user is the operator of the service. These checks become visible through the usage endpoint once T05 is done.

- Given a fresh volume, when the service starts, then `acc_demo` (10 GiB) and `acc_small` (100 MiB) exist with nothing reserved.
- Given `acc_small` is changed to 50 MiB in `SEED_ACCOUNTS`, when the service restarts, then its limit is 50 MiB and its reservations are unchanged.
- Given reservations exist, when the container is recreated with the same volume, then they are still there.

## Tests

Unit (`internal/app`): reading `SEED_ACCOUNTS` is configuration, so it lives in `loadConfig` rather than in `repository/sqlite`.
- [x] `TestLoadConfig_Defaults` — also checks `DB_PATH` (`/data/reserve.db`) and the default accounts (`acc_demo` 10 GiB, `acc_small` 100 MiB).
- [x] `TestLoadConfig_SeedAccounts` — `acc_one:100, acc_two:200` gives two accounts; spaces around entries are ignored.
- [x] `TestLoadConfig_InvalidValues` — six rows added: missing colon, non-numeric limit, negative limit, duplicate ID, empty entry, empty ID. Each error names `SEED_ACCOUNTS`.

Integration (`internal/repository/sqlite`):
- [x] `TestOpen_CreatesSchema` — the tables exist and `user_version` is 1.
- [x] `TestOpen_AppliesPragmas` — reads back `journal_mode`, `synchronous`, `busy_timeout` and `foreign_keys` on two different connections.
- [x] `TestSchema_EnforcesConstraints` — table: `bytes = 0`, `bytes` as text, unknown status, `released` without `released_at`, `active` with `released_at`, duplicate `(account_id, idempotency_key)`, unknown account, negative limit, negative counters. Each statement is rejected.
- [x] `TestOpen_IsIdempotent` — opening the same file twice keeps the data and the schema.
- [x] `TestOpen_FailsOnUnusablePath` — a directory given as the file path returns an error naming the step.
- [x] `TestOpen_FailsOnForeignDatabase` — a database the service did not create (version 0, tables already there) is refused.
- [x] `TestOpen_FailsOnIncompatibleSchema` — a version 1 database without the expected columns fails at the seeding step.
- [x] `TestSeed_CreatesAccounts`
- [x] `TestSeed_UpsertsLimitOnly` — reseeding with a lower limit keeps the counters.

Integration (`internal/app`):
- [x] `TestRun_FailsWhenDatabaseCannotOpen` — `Run` refuses to start, and the error names the database. The other `Run` tests get a temporary `DB_PATH`.

## Test results

Run on 2026-11-01.

- `gofmt -l .` prints nothing; `go vet ./...` passes.
- `go test ./...` (also with `-shuffle=on`): 19 tests and 22 subtests pass, none fail or skip (`app` 9, `handler` 1, `repository/sqlite` 9).
- `docker compose run --rm test`: passes with the race detector; coverage of `./internal/...` is 100.0%.
- Three tests passed as soon as they were written, because Go had already forced the error branch they cover. Coverage measured before and after each proves they exercise it: `TestOpen_FailsOnUnusablePath` (`Open` 60% to 100%), `TestOpen_FailsOnForeignDatabase` (`ensureSchema` 75% to 100%), `TestOpen_FailsOnIncompatibleSchema` (`seedAccounts` 80% to 100%).
- Container, checked by hand on a separate Compose project and volume, removed afterwards; the database was read with a throwaway program running as the container's user:
  - fresh volume: `acc_demo` 10737418240 and `acc_small` 104857600 bytes, counters at 0, `user_version` 1, `journal_mode` `wal`;
  - one reservation of 30 bytes added by hand to `acc_small`, then the container recreated with `SEED_ACCOUNTS=acc_demo:10737418240,acc_small:52428800`: `acc_small` limit 52428800, counters 30 and 1, the reservation still there;
  - `SEED_ACCOUNTS=acc_demo`: exit 1, `load configuration: SEED_ACCOUNTS: entry "acc_demo": want <account id>:<limit in bytes>`;
  - `DB_PATH=/data`, a directory: exit 1, `open database /data: read schema version: unable to open database file (14)`;
  - `docker compose stop`: `shutting down`, then `server stopped`, exit 0, and no `-wal` or `-shm` file left: the database is closed after the server.
