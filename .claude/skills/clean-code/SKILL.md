---
name: clean-code
description: Clean code for this Go service — readable, understandable, simple and minimal code that a Go newcomer can explain line by line. Use when planning, writing, refactoring or reviewing code; /implement-task, the tdd skill and /review-task load it.
---

# Clean code

The reader is new to Go and must explain every line in a live interview. Code is written for that reader first.

The specific rules are in `.claude/rules/go-style.md` and `.claude/rules/testing.md`. This skill gives the principles behind them, their priority, and the checklist used at the Refactor step and in reviews.

## The four rules of simple design

In priority order (Kent Beck):
1. **Passes the tests.** Nothing else counts until it does.
2. **Reveals intention.** Names and structure say what the code is for, without comments.
3. **No duplicated knowledge.** Each rule, limit or decision lives in one place.
4. **Fewest elements.** No function, type, field, parameter, interface or package that the first three rules do not require.

When two rules conflict, the earlier one wins. A helper that makes the intention clear is worth its lines; one that only saves two lines is not.

## Simple

- Clear is better than clever. Take the obvious construct, even if it is longer: an `if` rather than a clever expression, a loop rather than a chain of helpers.
- One level of abstraction per function: it either orchestrates steps (`Run`: load the configuration, build the logger, serve) or performs one step.
- Flat control flow: early returns, no `else` after `return`, at most two levels of nesting.
- Concrete first: a function and a concrete type. Add an interface, a type or a package only when a second real need appears (a second implementation, a test fake, a second caller).

## Minimal

- Write only what the current test or a ticket criterion requires. No parameter, option, field, configuration variable, export or branch "for later".
- Unexported by default: export only what another package uses.
- Delete rather than comment out. Code that no meaningful test can reach is dead: delete it.
- Standard library first; a new dependency needs the user's agreement.
- No generic helpers (`utils`, `common`, `helpers`) and no wrappers that add nothing to the standard library.

## Readable

- Names carry the meaning: the domain words of `docs/` (reservation, release, usage, idempotency key), units in quantities (`limitBytes`), booleans that read as statements (`replayed`).
- If a name needs a comment to be understood, rename it: `const deadlineIfRunHangs = 5 * time.Second` instead of a comment above each `5*time.Second`.
- No comments in Go code, doc comments included: Go requires none, so only toolchain directives (`//go:embed`) stay. A reason that no name can carry goes to the chat, or to `docs/` when reviewers need it.
- A function reads top to bottom: the main path on the left edge, each error handled where it occurs.
- Similar things look alike: same statement order, same error wording, same test layout across the codebase.

## Duplication

- Duplicated knowledge goes at once: a limit, a status name, a rule or a message that drives behaviour gets one constant or one function.
- Similar-looking code is not always duplicated knowledge. Extract it only when the shared part has a name and a single reason to change: duplication is far cheaper than the wrong abstraction.

## Tests are code too

- Same standards as production code: intention-revealing names, arrange / act / assert, nothing clever.
- A helper hides noise (setup, plumbing), never what the test checks.
- A failure message says what was expected, so a failure is understood without reading the test.

## Checklist

1. Can each function be explained in one sentence, and does its name say that sentence?
2. Would a Go newcomer understand every line without a Go tutorial?
3. Is every line required by a test or a ticket criterion?
4. Is anything unused: a parameter, field, return value, export, branch or import?
5. Does any name need a comment to be understood?
6. Is there any comment other than a toolchain directive?
7. Is any function longer than about 40 lines, mixing levels of abstraction, or nested more than two levels deep?
8. Is any knowledge duplicated: a literal, a rule or a message that drives behaviour?
9. Is there an abstraction (interface, type, helper, package) with a single use and no test need?
10. Does the new code look like the rest of the codebase: same patterns, vocabulary and error style?

## Sources

- Kent Beck's rules of simple design: https://martinfowler.com/bliki/BeckDesignRules.html
- Robert C. Martin, *Clean Code*. Its principles apply; where they clash with Go idioms, Go wins (errors are values, small interfaces declared by their consumer, functions need not be tiny).
- Go Proverbs: https://go-proverbs.github.io/ ("Clear is better than clever")
- Sandi Metz, The Wrong Abstraction: https://sandimetz.com/blog/2016/1/20/the-wrong-abstraction
- Effective Go: https://go.dev/doc/effective_go
