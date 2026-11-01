# Testing

> Written before implementation and kept up to date as the code lands.

How the service is tested. The tests themselves are listed in the tickets ([tasks/](../tasks/README.md)): each ticket names its tests and records their results.

## Approach

- **Test first.** Each behaviour starts as a failing test (red, green, refactor).
- **Real database.** Anything involving a transaction runs against a real SQLite file in `t.TempDir()`. Never `:memory:`: with the `database/sql` connection pool, each connection would open its own empty database.
- **No mocking framework.** Ports are replaced by small hand-written fakes.
- **Invariants everywhere.** Every repository and API test ends with `assertInvariants(t, db)`, which checks I1 and I2 from [consistency.md](consistency.md#invariants).
- **Race detector.** The suite runs with `-race` inside the `golang` image, because the race detector needs a C toolchain that the host may not have.

## Test types

| Type | What it exercises | Tools | Run with | Counted in coverage |
|---|---|---|---|---|
| Unit | One package without I/O: domain rules, services with fakes, pure helpers (validation, error mapping, payload parsing) | `testing` | `go test ./...` | Yes |
| Integration | A real SQLite file, `httptest` servers standing in for env.hivenet.com, the full HTTP stack in-process, `app.Run` | `testing`, `net/http/httptest` | `go test ./...` | Yes |
| Container | The Docker image started through `compose.yaml`: real network, named volume, non-root user, restart, SIGTERM, SIGKILL | `testing`, `os/exec` driving `docker compose`, build tag `e2e` | `go test -tags e2e ./e2e/...` | No |

Unit and integration tests live next to the code they test and run together. Why three types, and why not Testcontainers: [D19](decisions.md#d19--unit-integration-and-container-tests).

## Coverage

- Unit and integration tests together cover **100% of the statements in `./internal/...`**. `docker compose run --rm test` runs `go test -race -coverpkg=./internal/... -coverprofile=coverage.out ./...`, then `scripts/check-coverage.sh coverage.out`, which fails below 100%.
- `cmd/api/main.go` only handles signals and calls `app.Run`, so it stays outside the metric.
- An error branch is covered by reproducing the failure it handles (next section). A branch that no meaningful test can reach is dead code and is removed.
- Coverage shows which lines ran, not that they are right: every test asserts outcomes, and database tests check the invariants.

## How failures are simulated

| Failure | Simulation |
|---|---|
| Response lost | Call, discard the result, call again with the same key |
| Request still in progress | Fake repository blocked on a channel |
| Lock contention | A second connection holding `BEGIN IMMEDIATE`; the repository opened with a 100 ms busy timeout |
| Database gone | `db.Close()` before the call |
| Read-only database | The file opened with `mode=ro` |
| Disk full | `PRAGMA max_page_count` lowered by the test, so the next write fails with `SQLITE_FULL` |
| A statement fails mid-transaction | A trigger created by the test: `CREATE TRIGGER … BEGIN SELECT RAISE(ABORT, 'injected'); END` |
| Client disconnects | The context cancelled during the transaction |
| Crash or restart | Close and reopen the store on the same file; in container tests, `docker compose restart` and `docker compose kill -s SIGKILL` |
| Limit source down, slow or wrong | `httptest.Server` returning 500, sleeping past the timeout, or serving invalid payloads |
| Client gone while the response is written | A `ResponseWriter` whose `Write` fails |

## Concurrency tests

They start every goroutine at the same instant and count every outcome:

```go
start := make(chan struct{})
var wg sync.WaitGroup
errs := make([]error, n)
for i := range n {
	wg.Go(func() {
		<-start // all goroutines are released together
		_, _, errs[i] = repo.Reserve(ctx, "acc", fmt.Sprintf("key-%d", i), candidate(i), decide)
	})
}
close(start)
wg.Wait()
// count successes and each error type, then:
assertInvariants(t, db)
```

The invariant query must return no rows:

```sql
SELECT a.id
FROM accounts a
LEFT JOIN reservations r ON r.account_id = a.id AND r.status = 'active'
GROUP BY a.id
HAVING a.reserved_bytes <> COALESCE(SUM(r.bytes), 0)
    OR a.active_reservations <> COUNT(r.id);
```

## Running the tests

```sh
go test ./...                    # unit and integration tests, on the host
go test -cover ./...             # quick coverage per package
docker compose run --rm test     # go vet, race detector, 100% coverage check
go test -tags e2e ./e2e/...      # container tests (needs Go and Docker)
```

## Demo script

`scripts/demo.sh` runs against the Compose stack (`docker compose up -d`, then `docker compose run --rm demo`) and prints each step. It uses account `acc_small`: 100 MiB, with `MAX_PARALLEL_CHUNKS` = 5. It checks that nothing is reserved before it starts and releases what it created at the end, so it can be run again.

1. Show usage before anything.
2. Reserve 30 MiB: `201`.
3. Send the same request with the same key: `200`, `Idempotent-Replayed: true`, same ID, usage unchanged.
4. Send the same key with 31 MiB: `422`.
5. Send 20 parallel reservations of 20 MiB with distinct keys: exactly three `201` and seventeen `409 insufficient_storage`; usage shows 90 MiB reserved and 4 active reservations.
6. Release the first reservation twice: `200` both times; reserved space drops by 30 MiB once.
7. Release the remaining reservations: usage is back to zero.
