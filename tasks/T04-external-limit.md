# T04 — External limit: fetch, cache and refresh

| | |
|---|---|
| Issue | [#4](https://github.com/guillaumebarbas/reserve-file/issues/4) |
| Branch | `feat/t04-external-limit` |
| Priority | P0 |
| Estimate | 25 min |
| Depends on | T02, T03 |
| References | [architecture.md](../docs/architecture.md#external-limit-component), [D12](../docs/decisions.md#d12--external-limit-refreshed-in-the-background) |

## Overview

Load `MAX_PARALLEL_CHUNKS` from `https://env.hivenet.com/dev` without ever putting that call on a request path. The `limits` package fetches and validates the value at startup, keeps it in memory and in the `settings` table, and refreshes it in the background. Requests only read the cached value. What happens when the source fails is T18.

## Technical acceptance criteria

- [ ] `GET` with a 3 s timeout. Any status other than 200 is a failure.
- [ ] At most 1 MiB of body is read. The body must be a JSON object, and `MAX_PARALLEL_CHUNKS` a JSON integer ≥ 1; anything else is a parsing error.
- [ ] `MaxActive()` returns `(value, known)` from an atomic value, without I/O or locking.
- [ ] One fetch is attempted at startup, then the value is refreshed every `ENV_CONFIG_REFRESH` (60 s).
- [ ] Each new valid value is saved in `settings` through the `limits.Store` interface, implemented by `repository/sqlite`.
- [ ] The refresher stops when its context is cancelled, and `app.Run` waits for it before closing the database.
- [ ] `/readyz` reports `"limits": "ok"` or `"unavailable"`, without failing because of it.
- [ ] Configuration: `ENV_CONFIG_URL` (default `https://env.hivenet.com/dev`), `ENV_CONFIG_REFRESH` (60 s), `ENV_CONFIG_TIMEOUT` (3 s). An invalid value (not an absolute http or https URL, not a duration) stops the startup with a message naming the variable.
- [ ] `compose.yaml` passes `ENV_CONFIG_URL` through, with the dev URL as default: `ENV_CONFIG_URL=<url> docker compose up` points the service at another environment.
- [ ] The response body is never logged: it contains analytics tokens.

## User acceptance criteria

The user is the operator; the effect on clients shows in T05 and T06.

- Given env.hivenet.com returns `MAX_PARALLEL_CHUNKS: 5`, when the service starts, then `/readyz` shows `"limits": "ok"` and the service uses 5.
- Given the value changes to 8 at the source, when the next refresh has run, then the service uses 8.
- Given `ENV_CONFIG_URL` is set to another environment's URL, when the service starts, then it reads `MAX_PARALLEL_CHUNKS` from that URL.

## Tests

Unit (`internal/limits`, `internal/app`):
- [ ] `TestParseMaxParallelChunks` — table: `5`, `"5"`, `5.5`, `0`, `-1`, `null`, missing key, JSON array, malformed JSON.
- [ ] `TestLoadConfig_EnvConfig` — defaults, overrides, and an invalid URL or duration rejected with the variable's name.

Integration (`internal/limits`, `internal/repository/sqlite`, `internal/handler`):
- [ ] `TestFetcher_ParsesValue` — against an `httptest` server returning a payload shaped like the real one.
- [ ] `TestLimits_UsesFetchedValue`
- [ ] `TestLimits_RefreshPicksUpNewValue`
- [ ] `TestLimits_PersistsNewValue`
- [ ] `TestLimits_StopsWhenContextCancelled`
- [ ] `TestSettingsStore_SaveAndLoad`
- [ ] `TestAPI_ReadyzReportsLimits`

## Test results

_Not run yet._
