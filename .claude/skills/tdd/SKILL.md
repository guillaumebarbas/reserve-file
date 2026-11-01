---
name: tdd
description: Red-green-refactor loop for this Go service, driven by the tickets in tasks/, with a validation from the user after every step. Use when implementing, changing or fixing any behaviour, or when the user asks for TDD or the next test.
argument-hint: "[ticket ID such as T06, or a test name]"
---

# TDD cycle

Target: $ARGUMENTS, a ticket ID or a test name. If empty: the ticket being implemented, otherwise the first ticket in `tasks/README.md` that is not Done, and its first unticked test.

One test per cycle. Every step ends with a **stop**: show what was done, explain it in French, and wait for the user's validation before going on. Skip a stop only when the user explicitly asks to.

## 1. Red

1. State the behaviour in one French sentence.
2. Write the test, with its name from the ticket, and only the stub production code needed to compile (types, a function returning zero values).
3. Run it: `go test ./internal/<pkg>/ -run '^TestName$' -v`. It must fail on an assertion, not on a compilation error.
4. **Stop**: the test, the stub, the failure output, and why it fails.

## 2. Green

5. Write the simplest code that makes the test pass. Nothing for future tests.
6. Run the package tests, then `go test ./...`.
7. **Stop**: the code, the test output, how the code works, and each Go construct used for the first time (compared to Java when that helps).

## 3. Refactor

8. Propose improvements (names, duplication, Go style rules), or say there is nothing to refactor and why.
9. **Stop** if there are proposals; apply only the approved ones. Tests stay green after each change.
10. `gofmt -l .` prints nothing, `go vet ./...` passes, and `go test -cover ./internal/<pkg>/` shows no new uncovered line. An uncovered line gets a test that reproduces the failure it handles, or is deleted.

## 4. Close the cycle

11. Tick the test in the ticket. Update the docs if behaviour, API, schema or a decision changed.
12. **Stop**: a short recap in French, and the next test.

## Closing a ticket

When all its tests are ticked:
1. Run `docker compose run --rm test` (race detector and 100% coverage), and the container tests if the ticket touches the container.
2. Check each acceptance criterion and tick the technical ones.
3. Fill in **Test results**: date, commands, pass and fail counts, coverage, anything notable.
4. Ask the user for the actual time spent and record it in `tasks/README.md`. The ticket's state then moves on GitHub through `/push-pr` and `/review-task`.

## Rules

- No production code without a failing test that requires it.
- Never change a test just to make it pass. If the test itself was wrong, say so explicitly before changing it.
- If a new test passes before any production change, the behaviour already exists or the test is wrong: find out which before going on.
