---
paths:
  - "internal/repository/**"
  - "**/*.sql"
---

# Persistence rules (SQLite)

Reference: `docs/consistency.md` (data model, transactions, SQLite configuration).

- `database/sql` with handwritten SQL. No ORM, no query builder.
- Values always go through placeholders (`?`), never through string concatenation.
- Every operation that reads then writes, or writes rows that must change together (reserve, release), runs in one transaction opened with `BeginTx` (the DSN sets `_txlock=immediate`, so it is `BEGIN IMMEDIATE`). `defer transaction.Rollback()` right after opening it; `transaction.Commit()` at the end. The deferred Rollback's error is ignored on purpose: after a successful Commit it is always `sql.ErrTxDone`. It is the only error ignored without `_ =`.
- A lone statement needs no explicit transaction: SQLite runs it in its own. The startup seed upserts each account that way, since no invariant spans two accounts and a failed seed stops the startup.
- Scan rows into local variables and build domain values from them: nullable columns go through `sql.NullInt64`, timestamps are converted to UTC `time.Time`. No `db` tags on domain types.
- Read the state and write it in the same transaction. No network call and no sleep inside a transaction.
- Use affected-row counts to know what a statement did; never re-read to guess.
- Account counters change only in the transaction that changes the reservation row, and only when that row actually changed state.
- Always use the context variants: `ExecContext`, `QueryContext`, `QueryRowContext`.
- `defer rows.Close()` after every `QueryContext`, and check `rows.Err()` after the loop.
- Translate errors before returning them:
  - `sql.ErrNoRows` → the matching domain not-found error;
  - busy, locked, closed database, I/O error → `fmt.Errorf("%w: <action>: %v", domain.ErrServiceUnavailable, err)`: the sentinel with `%w`, the driver cause with `%v` only, so no caller can reach the SQLite error with `errors.As`;
  - unique violation on `(account_id, idempotency_key)` → `domain.ErrRequestInProgress`.
- Timestamps are stored as INTEGER Unix milliseconds (UTC); IDs as TEXT.
- Schema changes: edit `schema.sql`, bump `PRAGMA user_version`, and keep the `STRICT`, `CHECK` and `UNIQUE` constraints in line with `docs/consistency.md`.
