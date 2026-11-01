---
paths:
  - "internal/repository/**"
  - "**/*.sql"
---

# Persistence rules (SQLite)

Reference: `docs/consistency.md` (data model, transactions, SQLite configuration).

- `database/sql` with handwritten SQL. No ORM, no query builder.
- Values always go through placeholders (`?`), never through string concatenation.
- Every write runs in one transaction opened with `BeginTx` (the DSN sets `_txlock=immediate`, so it is `BEGIN IMMEDIATE`). `defer tx.Rollback()` right after opening it; `tx.Commit()` at the end.
- Read the state and write it in the same transaction. No network call and no sleep inside a transaction.
- Use `RETURNING` and affected-row counts to know what a statement did; never re-read to guess.
- Account counters change only in the transaction that changes the reservation row, and only when that row actually changed state.
- Always use the context variants: `ExecContext`, `QueryContext`, `QueryRowContext`.
- `defer rows.Close()` after every `QueryContext`, and check `rows.Err()` after the loop.
- Translate errors before returning them:
  - `sql.ErrNoRows` → the matching domain not-found error;
  - busy, locked, closed database, I/O error → `domain.ErrUnavailable`, wrapping the cause with `%w`;
  - unique violation on `(account_id, idempotency_key)` → `domain.ErrRequestInProgress`.
- Timestamps are stored as INTEGER Unix milliseconds (UTC); IDs as TEXT.
- Schema changes: edit `schema.sql`, bump `PRAGMA user_version`, and keep the `STRICT`, `CHECK` and `UNIQUE` constraints in line with `docs/consistency.md`.
