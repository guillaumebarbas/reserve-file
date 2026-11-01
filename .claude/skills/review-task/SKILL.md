---
name: review-task
description: "Review the implementation of a ticket: read the ticket or issue and the pull request, run the tests, list the problems and possible improvements, then let the user validate or refuse it. Run it with /review-task followed by a ticket ID, an issue number or a pull request."
argument-hint: "<T06 | issue number | pull request number or URL>"
disable-model-invocation: true
---

# Review a ticket

Target: $ARGUMENTS

GitHub operations go through the GitHub MCP server (`github`, configured in `.mcp.json`). If it is not connected, say so and use `gh` only with the user's agreement.

If no target is given, reply "Please choose a ticket or a pull request." followed by the open pull requests, then stop.

## 1. Gather

1. Resolve the ticket, its issue (issue #NN is ticket TNN), its branch (listed in `tasks/README.md`) and its pull request. Without a pull request, review the branch against `main`.
2. Read the ticket (acceptance criteria, tests) and the pull request: description, diff, commits.
3. Get the branch locally (`git fetch origin`, then `git switch <branch>`), after making sure the working tree is clean.

## 2. Verify

4. Run `gofmt -l .`, `go vet ./...`, `go test ./...`, then `docker compose run --rm test` (race detector and 100% coverage).
5. Check, with evidence (test names, `file:line`):
   - every technical and user acceptance criterion of the ticket is met;
   - every test listed in the ticket exists, tests what its name says, and passes;
   - the architecture and Go style rules, and the checklist of the `go-best-practices` skill;
   - the non-negotiables of `CLAUDE.md`: one transaction per operation, idempotency checked first, counters changed with the row, no SQLite error leaking, nothing secret logged;
   - the ticket's **Test results** and the docs are up to date;
   - nothing outside the ticket's scope.

## 3. Report, in French

- **Proposed verdict**: validate, or changes requested.
- **Problems**: blocking issues only, each with `file:line`, why it matters and a suggested fix.
- **Improvements**: optional and non-blocking (simplification, naming, performance, a missed edge case).
- **Remarks**: only if useful. When there is nothing to say, say so in one line rather than inventing remarks.

Do not modify code during the review.

## 4. Decision

Ask the user for the decision, then:
- **Validate**: squash-merge the pull request, then delete its branch (`git push origin --delete <branch>`). The issue closes through `Closes #<issue>`; replace its `status:` label with `status: validated`. Finally `git switch main` and `git pull --ff-only`.
- **Changes requested**: replace the issue's `status:` label with `status: refused`. Post the problems as a comment on the pull request only if the user agrees. The next `/implement-task` on this ticket resumes on the same branch.
- **Abandon**: close the pull request, close the issue as not planned, and label it `status: refused`.

GitHub does not let authors approve their own pull requests: validating means merging.
