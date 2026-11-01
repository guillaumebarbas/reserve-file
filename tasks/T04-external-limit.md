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

- [x] `GET` with a 3 s timeout. Any status other than 200 is a failure.
- [x] At most 1 MiB of body is read. The body must be a JSON object, and `MAX_PARALLEL_CHUNKS` a JSON integer ≥ 1; anything else is a parsing error.
- [x] `MaxActive()` returns `(value, known)` from an atomic value, without I/O or locking.
- [x] One fetch is attempted at startup, then the value is refreshed every `ENV_CONFIG_REFRESH` (60 s).
- [x] Each new valid value is saved in `settings` through the `limits.Store` interface, implemented by `repository/sqlite`.
- [x] The refresher stops when its context is cancelled, and `app.Run` waits for it before closing the database.
- [x] `/readyz` reports `"limits": "ok"` or `"unavailable"`, without failing because of it.
- [x] Configuration: `ENV_CONFIG_URL` (default `https://env.hivenet.com/dev`), `ENV_CONFIG_REFRESH` (60 s), `ENV_CONFIG_TIMEOUT` (3 s). An invalid value (not an absolute http or https URL, not a duration) stops the startup with a message naming the variable.
- [x] `compose.yaml` passes `ENV_CONFIG_URL` through, with the dev URL as default: `ENV_CONFIG_URL=<url> docker compose up` points the service at another environment.
- [x] The response body is never logged: it contains analytics tokens.

## User acceptance criteria

The user is the operator; the effect on clients shows in T05 and T06.

- Given env.hivenet.com returns `MAX_PARALLEL_CHUNKS: 5`, when the service starts, then `/readyz` shows `"limits": "ok"` and the service uses 5.
- Given the value changes to 8 at the source, when the next refresh has run, then the service uses 8.
- Given `ENV_CONFIG_URL` is set to another environment's URL, when the service starts, then it reads `MAX_PARALLEL_CHUNKS` from that URL.

## Tests

Unit (`internal/limits`, `internal/app`):
- [x] `TestParseMaxParallelChunks` — table: `5`, `"5"`, `5.5`, `0`, `-1`, `null`, missing key, JSON array, malformed JSON, and a `null` body.
- [x] `TestLoadConfig_EnvConfig` — defaults and overrides. The invalid URLs (relative, `ftp`, unparsable) and durations (not a duration, `0s`, no unit, negative) are rows of `TestLoadConfig_InvalidValues`, rejected with the variable's name.

Integration (`internal/limits`, `internal/repository/sqlite`, `internal/handler`, `internal/app`):
- [x] `TestFetcher_ParsesValue` — against an `httptest` server returning a payload shaped like the real one: an object with strings, booleans, numbers and nested objects.
- [x] `TestFetcher_RejectsInvalidResponses` *(added)* — `500`, `404`, a body over 1 MiB, an invalid value.
- [x] `TestFetcher_TimesOut` *(added)* — a source that never answers.
- [x] `TestFetcher_FailsOnTruncatedBody` *(added)* — the connection closes before the announced length.
- [x] `TestLimits_UsesFetchedValue`
- [x] `TestLimits_RefreshPicksUpNewValue` — with `testing/synctest`, so the minute between two refreshes is virtual.
- [x] `TestLimits_PersistsNewValue` — each new value is saved once; an unchanged value is not written again.
- [x] `TestLimits_StopsWhenContextCancelled`
- [x] `TestLimits_ReloadsPersistedValue` *(added, listed for T18 in consistency.md)* — `Init` loads the saved value before fetching, so a start while the source is down keeps it.
- [x] `TestLimits_FetchesWhenSavedValueCannotBeLoaded` *(added)* — a store that fails to load does not stop the fetch.
- [x] `TestLimits_KeepsFetchedValueWhenSaveFails` *(added)* — a store that fails to save keeps the value in memory and logs a warning.
- [x] `TestSettingsStore_SaveAndLoad`
- [x] `TestSettingsStore_FailsWhenDatabaseClosed` *(added)* — both methods wrap `domain.ErrServiceUnavailable` and nothing else.
- [x] `TestSettingsStore_RejectsCorruptedValue` *(added)* — a stored value that is not a number is an error.
- [x] `TestAPI_ReadyzReportsLimits` — `200` with `"limits":"unavailable"` while the limit is unknown. `TestAPI_Readyz` now expects `"limits"` in both of its bodies.
- [x] `TestRun_FetchesLimitAtStartup` *(added)* — `app.Run` fetches the limit before serving, and `/readyz` reports it. The other `Run` tests point `ENV_CONFIG_URL` at an unused local port, so no test calls the real endpoint.

## Test results

Run on 2026-11-01.

- `gofmt -l .` prints nothing; `go vet ./...` passes.
- `go test ./...` (also with `-shuffle=on`): 49 tests and 78 subtests pass, none fail or skip (`app` 11, `domain` 1, `handler` 12, `limits` 12, `repository/sqlite` 13).
- `docker compose run --rm test`: passes with the race detector; coverage of `./internal/...` is 100.0%.
- Every test failed first on an assertion, except five that passed as soon as they were written, because Go had already forced the error branch they cover. Coverage before and after each proves they exercise it: `TestFetcher_TimesOut` and `TestFetcher_FailsOnTruncatedBody` (`limits` 90.5% to 100%), `TestLimits_FetchesWhenSavedValueCannotBeLoaded` (97.1% to 100%), `TestLimits_KeepsFetchedValueWhenSaveFails` (97.4% to 100%), and the two settings failure tests (`repository/sqlite` 92.5% to 100%). `TestLimits_StopsWhenContextCancelled` also passed at once: the previous test already needed `Run` to stop on cancellation to clean up.
- Container, checked by hand on a separate Compose project, removed afterwards:
  - default source: the log shows `limit updated` with `max_parallel_chunks` 5 and no payload, and `/readyz` answers `{"database":"ok","limits":"ok"}`;
  - recreated with `ENV_CONFIG_URL=http://127.0.0.1:9/dev`: the saved 5 is reloaded, `limits` stays `ok`, and the warning names only the connection error;
  - fresh volume with that URL: `/readyz` answers `200` with `{"database":"ok","limits":"unavailable"}`;
  - `ENV_CONFIG_URL=env.hivenet.com/dev`: exit 1, `load configuration: ENV_CONFIG_URL: "env.hivenet.com/dev" is not an absolute http or https URL`;
  - `docker compose stop`: `shutting down`, `server stopped`, exit 0.
