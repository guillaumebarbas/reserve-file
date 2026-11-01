---
name: push-pr
description: "Open the pull request of the current ticket branch, with a short and digestible English description, and move the ticket's issue to validation. Run it with /push-pr."
disable-model-invocation: true
---

# Push a pull request

GitHub operations go through the GitHub MCP server (`github`, configured in `.mcp.json`). If it is not connected, say so and use `gh` only with the user's agreement.

## 1. Preconditions

1. The current branch is not `main`, and everything is committed and pushed (`git status`, `git log @{u}..`). Otherwise suggest `/push-code` and stop.
2. Find the ticket and its issue from the branch name. If the branch already has an open pull request, this is a rework after a refusal: do not create another one. Prepare a reply to each unresolved review thread (the commit that addresses it, or the reason it is deferred and where it is tracked) and a short comment summarizing what changed since the review. Once the user validates them, post the replies, resolve the threads, and post the comment. Then set the ticket state to `validation` (see Ticket state in `CLAUDE.md`), give the pull request URL and stop.
   A branch without a ticket ID (tooling, docs) has no issue: the body has no `Closes` line, no ticket state changes, and step 3 is skipped.
3. Check the ticket: every test ticked, **Test results** filled in, `docker compose run --rm test` run for this ticket. List what is missing; the user decides whether to open the pull request anyway, and if so it goes in Notes.

## 2. Title and description

4. Title: a conventional commit header ending with the ticket ID, at most 72 characters, for example `feat(reserve): reserve storage within the account limits (T06)`. It becomes the squash commit on `main`.
5. Body in English, readable in 30 seconds:

   ```
   Closes #<issue>

   ## What
   - 2 to 4 bullets: the behaviour added, as a client or a reviewer sees it.

   ## How to test
   - The commands: `go test ./...`, `docker compose run --rm test`, or one curl call.

   ## Notes
   - Only if useful: a decision, a trade-off, a follow-up. Otherwise drop the section.
   ```

   - No file-by-file list, no copy of the ticket, no account of how the code was written. Link the ticket file when more context helps.
   - End with the attribution line Claude Code requires for pull requests in this session, if any.
6. **Stop**: show the title and the body, and wait for validation or edits.

## 3. Creation

7. Create the pull request: base `main`, head the ticket branch, with the validated title and body.
8. Set the ticket state to `validation` (see Ticket state in `CLAUDE.md`).
9. Give the pull request URL and suggest `/review-task <ticket>`.
