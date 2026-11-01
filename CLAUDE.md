# reserve-file

Take-home exercise for Hivenet: a Go service that reserves storage space for uploads. Brief: `Backend Developer Homework - Reserve Space for an Upload.md`. Timebox 2 to 4 hours: reliable, explainable core behaviour matters more than features.

## Read before changing behaviour

- `tasks/`: one ticket per step, in order, each with its acceptance criteria and the names of its tests. Ticket TNN is GitHub issue #NN; its state is the issue's `status:` label.
- `docs/decisions.md`: decisions D1–D19 and their reasons. Do not contradict an Accepted decision without asking the user.
- `docs/consistency.md`: invariants, reserve and release transactions, failure handling.
- `docs/api.md`: the HTTP contract and the error catalogue.
- `docs/architecture.md`: packages and the dependency rule.
- `docs/testing.md`: test types, the coverage rule, how failures are simulated.

## Working with the user

- The user does not know Go yet and must explain and modify every line during a live interview. Readability beats cleverness.
- Tickets are implemented with `/implement-task <ID>`: a plan first, then the `tdd` cycle with a validation from the user after every step. Never generate code ahead of the current step.
- After each step, explain in French, in the chat: what changed, why, and each Go construct used for the first time, compared to Java when that helps. Code comments are written for reviewers: no Go tutorials in the code.
- Ask before adding a dependency, or before changing an Accepted decision, the API contract or the schema.
- Chat in French. Code, comments, commit messages, docs and tickets in English.

## Workflow

| Step | Command | Ticket state afterwards |
|---|---|---|
| Plan, branch linked to the issue, TDD with a stop after every step | `/implement-task T06` | `status: in progress` |
| Checks, conventional commit, push | `/push-code` | unchanged |
| Pull request that closes the issue | `/push-pr` | `status: validation` |
| Review, tests, then merge or send back | `/review-task T06` | `status: validated` or `status: refused` |

- Branch names follow Conventional Branch (`feat/t06-reserve`, listed in `tasks/README.md`); commit messages follow Conventional Commits (skill `conventional-commit`).
- Never commit on `main`. Commit and push only through `/push-code`.
- GitHub operations (issues, labels, pull requests, project) go through the GitHub MCP server defined in `.mcp.json`: official image, authenticated with a personal access token read from the `GITHUB_MCP_PAT` environment variable (scopes `repo` and `project`), toolsets limited to `context,repos,issues,labels,pull_requests,projects`. Without the variable the server falls back to OAuth, but Claude Code cannot display its login URL, so every call fails. `gh` is the fallback.

## Ticket state

A ticket's state lives in two places, always changed together:
- the `status:` label of its issue (`status: to do`, `status: in progress`, `status: validation`, `status: validated`, `status: refused`), which the skills read;
- the Status column of its card in the GitHub Project [reserve-file](https://github.com/users/guillaumebarbas/projects/2) (`To do`, `In progress`, `Validation`, `Validated`, `Refused`), which shows the board.

"Set the ticket state to X" in a skill means: replace the issue's `status:` label, then set the card's Status to the matching column. If the two disagree, the label wins and the card is fixed.
- Skill `go-best-practices`: Go practices and review checklist, used when planning, refactoring and reviewing.

## Commands

| Task | Command |
|---|---|
| Tests | `go test ./...` |
| One test | `go test ./internal/<pkg>/ -run '^TestName$' -v` |
| Quick coverage per package | `go test -cover ./...` |
| Format check (must print nothing) | `gofmt -l .` |
| Static checks | `go vet ./...` |
| Race detector and 100% coverage check (in Docker: the race detector needs cgo) | `docker compose run --rm test` |
| Container tests | `go test -tags e2e ./e2e/...` |
| Run the stack | `docker compose up --build` |
| End-to-end demo | `docker compose run --rm demo` |

The Docker commands work once T01 is done; the demo once T12 is done; the container tests once T22 is done.

## Definition of done

For every TDD cycle:
1. A test failed first and now passes.
2. `gofmt -l .` prints nothing; `go vet ./...` and `go test ./...` pass.
3. The test is ticked in its ticket. Docs are updated when behaviour, API, schema or a decision changed.
4. The user got the French explanation.

For every ticket, in addition:
5. `docker compose run --rm test` passes: race detector, and 100% coverage of `./internal/...`.
6. Acceptance criteria checked, **Test results** filled in, actual time recorded in `tasks/README.md`; then `/push-code` and `/push-pr`.

## Non-negotiables

- Each reserve or release is one `BEGIN IMMEDIATE` transaction, with no network I/O inside.
- The idempotency lookup runs before every other admission check.
- Account counters change only together with the reservation row, in the same transaction.
- `MAX_PARALLEL_CHUNKS` is never fetched on the request path.
- No SQLite error leaves `internal/repository/sqlite`; no internal error text reaches an HTTP response.
- The env.hivenet.com payload contains tokens: never log it.
