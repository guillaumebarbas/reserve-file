---
paths:
  - "**/*_test.go"
---

# Testing rules

Process: `/implement-task` and the `tdd` skill. Which tests to write: the current ticket in `tasks/`. Strategy: `docs/testing.md`.

## Test types

| Type | Scope | Where |
|---|---|---|
| Unit | One package, no I/O: domain rules, services with fakes, pure helpers | Next to the code |
| Integration | Real SQLite in `t.TempDir()`, `httptest` servers, the full HTTP stack in-process, `app.Run` | Next to the code |
| Container | The Docker image started through `compose.yaml` | `e2e/`, build tag `e2e` |

## Coverage

- Unit and integration tests together cover 100% of the statements in `./internal/...`. `docker compose run --rm test` enforces it; `go test -cover ./...` gives a quick view.
- `cmd/api/main.go` only calls `app.Run` and is outside the metric. Keep it that way.
- Cover an error branch by reproducing the failure it handles: a SQLite trigger raising an error, a closed database, a cancelled context, a failing `ResponseWriter`, an `httptest` server returning garbage.
- Never write a test only to execute a line, and never add a branch that only tests use (injecting a dependency such as a clock is fine). A branch that no meaningful test can reach is dead code: delete it.
- Coverage shows that lines ran, not that they are right: every test asserts outcomes.

## Structure

- Names: `Test<Unit>_<Behaviour>` (`TestRelease_Twice`). Table tests run each case with `t.Run(tc.name, …)` and a readable case name.
- Table-driven tests for rules with many cases (`DecideReserve`, input validation, the limits fetcher).
- Arrange, act, assert, separated by blank lines. One behaviour per test.
- Standard library only (`testing`, `net/http/httptest`): no assertion or mocking library.
- Failure messages say what was expected: `t.Errorf("available bytes: got %d, want %d", got, want)`. Use `t.Fatalf` when the test cannot continue.
- Helpers call `t.Helper()` first.

## Database

- A real SQLite file in `t.TempDir()`, one per test. Never `:memory:`: each pooled connection would get its own empty database.
- Every repository and API test ends with `assertInvariants(t, db)`.

## Fakes

- Hand-written fakes implementing the port interfaces, defined in the test files of the package that uses them.
- A fake does only what its tests need.

## Concurrency

- Start all goroutines together (close a shared channel), run them with `wg.Go(func() { … })`, then count every outcome exactly after `wg.Wait()`.
- Never call `t.Fatal`, `t.FailNow` or a helper that does from a goroutine the test started: record results, assert after `wg.Wait()`.
- Never use `time.Sleep` to order events: use channels. Time-based behaviour (refresh interval, backoff) is tested with `testing/synctest`, whose clock is virtual.
- These tests must pass under the race detector: `docker compose run --rm test`.

## External HTTP

- Use `httptest.NewServer` to stand in for env.hivenet.com. Unit and integration tests never call the real endpoint.

## Do not test

- The standard library or SQLite itself.
- Private helpers already covered by the behaviour that uses them.
