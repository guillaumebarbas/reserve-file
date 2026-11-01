BEGIN;

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

PRAGMA user_version = 1;

COMMIT;
