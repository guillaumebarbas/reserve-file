---
name: push-code
description: "Check, commit and push the current work on the ticket branch. Shows the files and a conventional commit message for validation, runs the checks, commits, then pushes. Run it with /push-code."
argument-hint: "[optional: what this commit is about]"
disable-model-invocation: true
---

# Push code

Context from the user: $ARGUMENTS

## 1. Where we are

1. `git branch --show-current`. On `main`, stop: work happens on a ticket branch, which `/implement-task` creates.
2. `git status --short` and `git diff --stat`. If there is nothing to commit and nothing to push, say so and stop.
3. Find the ticket from the branch name: `feat/t06-reserve` is T06, issue #6. A branch without a ticket ID (tooling, docs) has no ticket, and its commits have no `Refs` footer.

## 2. Checks

4. `gofmt -l .` prints nothing; `go vet ./...` and `go test ./...` pass (once `go.mod` exists). If a check fails, show its output and stop: nothing is committed while checks fail.

## 3. Proposal

5. List the files to commit, grouped as added, modified and deleted. Leave out what must not be versioned: databases, `coverage.out`, binaries, local settings, secrets. Point out any file you are unsure about.
6. Write the message with the `conventional-commit` skill, ending with `Refs: #<issue>`.
7. If the changes mix unrelated concerns, propose several commits.
8. **Stop**: show the file list and the message, and wait for the user's validation or edits.

## 4. Commit and push

9. Stage the validated files by path (`git add <paths>`), never with `git add -A` or `git add .`.
10. Commit with the message passed through a heredoc: `git commit -F - <<'EOF' … EOF`. If a hook fails, fix the cause and commit again; never use `--no-verify`.
11. Push: `git push -u origin <branch>` the first time, `git push` afterwards. Never push to `main`; never force-push unless the user explicitly asks.
12. Report the commit hash and the branch. If the branch has no pull request yet (check with the GitHub MCP server), suggest `/push-pr` once the ticket is complete.
