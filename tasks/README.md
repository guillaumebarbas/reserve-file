# Tasks

One ticket per step: the foundations, each feature, and each edge case or small group of cases. Every ticket holds an overview, technical and user acceptance criteria, the names of its tests, and their results once run.

**Tracking.** Ticket TNN is GitHub issue #NN. Its state is the issue's label (`status: to do`, `status: in progress`, `status: validation`, `status: validated` or `status: refused`), mirrored by its column on the [project board](https://github.com/users/guillaumebarbas/projects/2).

**Workflow.**
1. `/implement-task <ID>` on the first open ticket: a plan to validate, the ticket branch, then the TDD cycle with a validation after each step. The issue moves to `status: in progress`.
2. `/push-code` after each validated cycle: checks, conventional commit, push.
3. `/push-pr` when the ticket is complete: a short pull request that closes the issue, which moves to `status: validation`.
4. `/review-task <ID>`: review and tests, then the user merges (`status: validated`) or asks for changes (`status: refused`).

| ID | Ticket | Kind | Priority | Estimate | Depends on | Issue | Branch | Actual |
|---|---|---|---|---|---|---|---|---|
| [T01](T01-scaffold.md) | Scaffold and walking skeleton | Foundation | P0 | 30 min | — | [#1](https://github.com/guillaumebarbas/reserve-file/issues/1) | `chore/t01-scaffold` | 35 min |
| [T02](T02-database.md) | Database | Foundation | P0 | 25 min | T01 | [#2](https://github.com/guillaumebarbas/reserve-file/issues/2) | `feat/t02-database` | 30 min |
| [T03](T03-http-foundation.md) | HTTP foundation and error format | Foundation | P0 | 30 min | T01 | [#3](https://github.com/guillaumebarbas/reserve-file/issues/3) | `feat/t03-http-foundation` | 30 min |
| [T04](T04-external-limit.md) | External limit: fetch, cache and refresh | Foundation | P0 | 25 min | T02, T03 | [#4](https://github.com/guillaumebarbas/reserve-file/issues/4) | `feat/t04-external-limit` | 20 min |
| [T05](T05-usage.md) | Account usage | Feature | P0 | 15 min | T04 | [#5](https://github.com/guillaumebarbas/reserve-file/issues/5) | `feat/t05-usage` | 20 min |
| [T06](T06-reserve.md) | Reserve storage | Feature | P0 | 30 min | T05 | [#6](https://github.com/guillaumebarbas/reserve-file/issues/6) | `feat/t06-reserve` | 30 min |
| [T07](T07-concurrent-reservations.md) | Concurrent reservations never exceed the limits | Case | P0 | 20 min | T06 | [#7](https://github.com/guillaumebarbas/reserve-file/issues/7) | `feat/t07-concurrent-reservations` | 7 min |
| [T08](T08-retry-after-lost-response.md) | Retry after a lost response | Case | P0 | 20 min | T06 | [#8](https://github.com/guillaumebarbas/reserve-file/issues/8) | `feat/t08-retry-after-lost-response` | 20 min |
| [T09](T09-release.md) | Release and read a reservation | Feature | P0 | 20 min | T06 | [#9](https://github.com/guillaumebarbas/reserve-file/issues/9) | `feat/t09-release` | 25 min |
| [T10](T10-double-release.md) | Double and concurrent release | Case | P0 | 15 min | T09 | [#10](https://github.com/guillaumebarbas/reserve-file/issues/10) | `feat/t10-double-release` | 15 min |
| [T11](T11-restart-and-shutdown.md) | Restart and graceful shutdown | Case | P0 | 15 min | T09 | [#11](https://github.com/guillaumebarbas/reserve-file/issues/11) | `feat/t11-restart-and-shutdown` | 20 min |
| [T12](T12-demo-and-readme.md) | Demo script and README | Delivery | P0 | 25 min | T01 to T11 | [#12](https://github.com/guillaumebarbas/reserve-file/issues/12) | `chore/t12-demo-and-readme` | |
| [T13](T13-invalid-input.md) | Invalid input | Case | P1 | 15 min | T06 | [#13](https://github.com/guillaumebarbas/reserve-file/issues/13) | `feat/t13-invalid-input` | 15 min |
| [T14](T14-key-reuse.md) | Same key, different size | Case | P1 | 10 min | T08 | [#14](https://github.com/guillaumebarbas/reserve-file/issues/14) | `feat/t14-key-reuse` | 10 min |
| [T15](T15-duplicate-in-flight.md) | Same request twice at the same time | Case | P1 | 25 min | T08 | [#15](https://github.com/guillaumebarbas/reserve-file/issues/15) | `feat/t15-duplicate-in-flight` | |
| [T16](T16-retry-after-change.md) | Retry after the situation changed | Case | P1 | 15 min | T08, T09 | [#16](https://github.com/guillaumebarbas/reserve-file/issues/16) | `feat/t16-retry-after-change` | |
| [T17](T17-release-not-found.md) | Release of an unknown or foreign reservation | Case | P1 | 10 min | T09 | [#17](https://github.com/guillaumebarbas/reserve-file/issues/17) | `feat/t17-release-not-found` | |
| [T18](T18-limit-source-down.md) | Limit source down or invalid | Case | P1 | 25 min | T04, T06, T09 | [#18](https://github.com/guillaumebarbas/reserve-file/issues/18) | `feat/t18-limit-source-down` | |
| [T19](T19-limits-lowered.md) | Limits lowered below current usage | Case | P1 | 15 min | T06, T09 | [#19](https://github.com/guillaumebarbas/reserve-file/issues/19) | `feat/t19-limits-lowered` | |
| [T20](T20-database-unavailable.md) | Database unavailable | Case | P1 | 20 min | T06, T09 | [#20](https://github.com/guillaumebarbas/reserve-file/issues/20) | `feat/t20-database-unavailable` | |
| [T21](T21-no-partial-state.md) | No partial state after a failure mid-transaction | Case | P1 | 15 min | T06, T09 | [#21](https://github.com/guillaumebarbas/reserve-file/issues/21) | `feat/t21-no-partial-state` | |
| [T22](T22-container-tests.md) | Container tests | Delivery | P1 | 40 min | T11 | [#22](https://github.com/guillaumebarbas/reserve-file/issues/22) | `chore/t22-container-tests` | |
| [T23](T23-openapi.md) | OpenAPI description and Swagger UI | Delivery | P2 | 30 min | T12 | [#23](https://github.com/guillaumebarbas/reserve-file/issues/23) | `feat/t23-openapi` | |

**Actual time.** From T04 on, independent tickets are built in parallel by agents in separate worktrees; their actual time is the wall-clock time from launch to merge, so parallel tickets overlap.

**Branches** follow [Conventional Branch](https://conventional-branch.github.io/): `chore/` for tooling and delivery, `feat/` for behaviour.

**Priorities.**
- P0: what the brief requires, including the cases it names (retry, double release, concurrency, restart), and the README.
- P1: the other edge cases, and the container tests.
- P2: optional.

**Timebox.** The estimates add up to about 4 h 30 for P0, 3 h 10 for P1 and 30 min for P2. The brief asks for 2 to 4 hours and to stop at 4 hours. When the limit gets close, stop and finish T12: the README lists what remains. After any later ticket, update the README's list of what is complete and unfinished.

## Case coverage

Every edge case from the design, and the ticket that proves it.

| Group | Case | Expected behaviour | Ticket |
|---|---|---|---|
| Retries | Commit succeeded, response lost, client retries | Replay of the same reservation: no extra bytes, no extra slot | T08 |
| Retries | Two identical requests at the same time (double click) | One reservation. The second request gets `409 request_in_progress`, and its retry gets the replay. Before T15, the second request waits for the lock and gets the replay | T15 |
| Retries | Same key, different `bytes` | `422 idempotency_key_reused` | T14 |
| Retries | Retry after the reservation was released | `200` with the released reservation; nothing is created again | T16 |
| Retries | Retry when the account is full or all slots are used | The replay succeeds: the key is checked first | T16 |
| Retries | Retry while env.hivenet.com is down | The replay succeeds: it does not need the limit | T16 |
| Retries | First attempt refused (account full), retried later | Evaluated again: refusals are not stored | T08 |
| Retries | Client generating a new key for each retry | A new reservation each time: client contract, documented in api.md | T08 |
| Release | Released twice, or twice at the same time | One transition from `active` to `released`; the other calls get `200` and change nothing | T10 |
| Release | Unknown ID, or ID of another account | `404`, not `403`: existence is not revealed | T17 |
| Release | Release while env.hivenet.com is down | Works: release never uses the limit | T18 |
| Concurrency | 50 parallel reservations asking for more than the limit | Only those that fit succeed; counters equal the active rows; reserved ≤ limit | T07 |
| Input | `bytes` zero, negative, decimal, string, missing or above 2⁶³−1 | `400 invalid_request` | T13 (zero, negative, missing: T06) |
| Input | Body too large, or unknown fields | `413 request_too_large`, `400 invalid_request` | T13 |
| Input | `Idempotency-Key` missing, empty, or over 255 characters | `400 invalid_idempotency_key` | T13 (missing, empty: T06) |
| Database | Locked too long, disk full, read-only | `503` with `Retry-After`, rollback, no partial state; the retry is safe | T20 |
| Database | Crash during the transaction | Nothing applied | T21, T22 |
| Database | Crash after the commit, before the response | The client retries and gets the replay | T08, T22 |
| Database | Client disconnects mid-request | Commit or rollback, never partial; the retry is safe | T21 |
| Database | Restart, or SIGTERM | State reloaded from the database; in-flight requests finish | T11, T22 |
| Limits | env.hivenet.com down, slow or invalid; first start without it | Last valid value kept; `503` for new reservations only while no value has ever been obtained | T18 |
| Limits | Storage limit or `MAX_PARALLEL_CHUNKS` lowered below current usage | Nothing revoked; new reservations refused until usage is back under the limit | T19 |
