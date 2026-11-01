---
name: implement-task
description: "Implement one ticket from tasks/ together with the user, step by step: read the ticket, analyse the existing code, propose a plan, create the ticket branch, then run the tdd cycle with a validation after every step. Run it with a ticket ID, for example /implement-task T06."
argument-hint: "<ticket ID, e.g. T06>"
disable-model-invocation: true
---

# Implement a ticket

Ticket: $ARGUMENTS

The user must understand and control every line. Never generate code ahead of the current step.

Ticket TNN is GitHub issue #NN, and its branch is listed in `tasks/README.md`. The ticket's state is the `status:` label of its issue. GitHub operations go through the GitHub MCP server (`github`, configured in `.mcp.json`); if it is not connected, say so and use `gh` only with the user's agreement.

## 0. Check the input

- No ticket given: reply "Please choose a ticket." followed by the open tickets in order (ID, title and state, from the GitHub issues), then stop.
- Unknown ticket: reply "Ticket <ID> not found. Please choose a ticket." followed by the same list, then stop.
- Issue closed or labelled `status: validated`: say the ticket is done and stop, unless the user asks to reopen it.
- Labelled `status: validation`: a pull request is waiting for review; suggest `/review-task <ID>` and stop.
- Labelled `status: refused`: read the review comments on its pull request; the plan starts from them.
- A ticket it depends on is not validated: name it and ask whether to continue anyway.

## 1. Understand

No file is created or modified in this phase.

1. Read the ticket and the docs it references.
2. Read the code the ticket will touch: packages, types, functions, existing tests. Note what exists and what is missing.
3. Load the `go-best-practices` skill.

## 2. Propose a plan

Write it in French, under these headings:
- **Need**: the ticket in two or three sentences, in your own words.
- **Existing code**: what is already there and will be reused.
- **Files**: each file to create or modify, with one line on why.
- **Tests in order**: the ticket's tests in the order they will be written, one TDD cycle each, simplest first, with the code each test will force into existence.
- **Watch points**: the decisions and Go practices that apply, the risks, the Go constructs that will appear for the first time.
- **Questions**: anything unclear in the ticket.

Ask the user to validate the plan and wait. Revise it until they approve it. If they decline, stop.

## 3. Start the ticket

1. The working tree must be clean; otherwise show `git status` and ask what to do.
2. `git switch main` and `git pull --ff-only`.
3. Branch: if the ticket's branch already exists (rework after a refusal), switch to it and pull. Otherwise create it from `main` with `git switch -c <branch>`. The pull request will link it to the issue through `Closes #<issue>`.
4. Set the ticket state to `in progress` (see Ticket state in `CLAUDE.md`), and assign the issue to the user.

## 4. Implement, one validated step at a time

1. **Test file**: create the test file with the first test of the plan only, and the helpers it needs. No production code. Show it, explain it in French, and wait for validation. The Red step of this first cycle then only adds the compile stub and runs the test.
2. For each test, follow the `tdd` skill. Each of its steps (Red, Green, Refactor, close of the cycle) ends with a stop: show what was done, explain it in French, and wait for the user's validation. At the Refactor step, also check the code against the `go-best-practices` checklist.
3. Write one test at a time. Never write production code that the current test does not require.
4. At the end of a cycle, remind the user that `/push-code` commits it.
5. If the user edits the code or changes direction, adapt and say what changes in the plan.
6. Skip a stop only when the user explicitly asks to, and only for what they asked.

## 5. Finish

Follow "Closing a ticket" in the `tdd` skill. Then summarize in French what the ticket added, and tell the user the next steps: `/push-code` for the remaining changes, then `/push-pr`. The issue state is not changed here.
