---
name: go-best-practices
description: Go practices and review checklist for this service, distilled from Go Code Review Comments, the Google and Uber style guides, Three Dots Labs, Ben Johnson's package layout and 100 Go Mistakes. Use when planning, writing, refactoring or reviewing Go code in this repository; /implement-task and /review-task load it.
user-invocable: false
---

# Go best practices

Complements `.claude/rules/` (style, architecture, testing, persistence, HTTP), which already encode most practices. This skill adds what they do not say, records the conflicts between sources and our rules, and gives the review checklist.

The reader is new to Go and must explain the code live. Between two correct options, take the more obvious one.

## Practices not covered by the rules

**Packages**
- Group files by feature inside a package; never one type per file. Avoid both tiny packages and huge files.
- No clean-architecture vocabulary in package or type names (`usecase`, `entities`, `interactor`, `adapters`). Go's implicit interfaces make a ports layer unnecessary: `service/ports.go` is enough.
- Never name a variable after an imported package (`url`, `limits`, `handler`). Alias an import only on a real conflict; the modernc driver is also named `sqlite`, so alias it inside `repository/sqlite` if it is imported by name.

**Interfaces and structs**
- Never use a pointer to an interface.
- Never embed a dependency (`*sql.DB`, `sync.Mutex`, `*http.Client`) in a struct: use a named, unexported field.
- No `if dep == nil { panic(…) }` in constructors: a missing dependency is a wiring bug that the `app.Run` tests catch.

**Errors**
- A sentinel goes first: `fmt.Errorf("%w: begin reserve: %v", domain.ErrServiceUnavailable, err)`. Otherwise `%w` goes last.
- Messages name the action that failed, never "failed to", "unable to" or "couldn't", and do not repeat the inner error.
- Never drop an error silently: write `_ = f()` when it is deliberate. The deferred `transaction.Rollback()` is the one documented exception.
- Return a literal `nil` on success, never a nil pointer of an error type: stored in an `error`, it is not equal to `nil`.
- Decide once whether each error type is returned as a pointer or a value, and use the same form as the `errors.As` target.

**Concurrency and context**
- Prefer synchronous functions: a component exposes a blocking `Run(ctx)`, and its caller starts and owns the goroutine.
- Never give a request's context to work that outlives the request. The shutdown deadline gets a fresh context, because the signal context is already cancelled.
- A struct that holds a mutex or an atomic has pointer receivers only and is never copied.
- Channels are unbuffered or of size 1; any other size needs a written reason.
- Standard typed atomics (`atomic.Int64`, `atomic.Bool`), never a third-party atomic package.

**Database**
- One repository per aggregate: account counters and reservation rows change through the same repository and the same transaction. No "transaction provider".

**HTTP**
- `return` immediately after `writeError` or `http.Error`: writing a response does not stop the handler.
- Outgoing calls use a dedicated `http.Client` with a timeout, never `http.Get` or `http.DefaultClient`, and always close the response body.
- `http.Server` sets `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout` (longer than the 5 s busy timeout) and `IdleTimeout`.
- Request IDs come from `crypto/rand`, never `math/rand`.

**Tests**
- Table rows hold data only. Cases that need different logic (`shouldErr` branches, per-case setup) become separate test functions.
- Subtest names read like identifiers (`zero_bytes`): no spaces, no slashes, never a row index.
- Tests live in the package they test (`package foo`); `package foo_test` only to test through the public API or break an import cycle. No `TestMain`.
- Run `go test -shuffle=on ./...` from time to time to catch order-dependent tests.

**Readability**
- `if err := f(); err != nil` only when the value is not needed after the `if`.
- Watch for `:=` creating a new variable that hides an outer one (`ctx`, `err`) inside a block: use `=` to assign the outer one.
- No `Get` prefix and no getters or setters for plain fields.
- Constants in MixedCaps (`maxBodyBytes`), never `MAX_BODY_BYTES`.
- Enums are typed constants whose zero value is invalid: `iota + 1`, or string constants matching the stored values (`"active"`, `"released"`).
- File order: types and constants, exported functions in call order, then unexported helpers. Struct literals name their fields.
- Accept a little repetition rather than an abstraction built to remove it.
- No comments, doc comments included: names say what is not obvious, and `docs/` holds the reasons a reviewer needs.

## Where sources disagree with our rules

| Source says | We do | Why |
|---|---|---|
| Blank imports (`import _ "driver"`) only in `main` | The driver import stays in `repository/sqlite` | It is the only package allowed to know the driver, and `main` stays trivial |
| Interfaces next to the domain type, shared `mock` package (Three Dots, Ben Johnson) | Interfaces in `service/ports.go`, fakes in the test files of the package that uses them | Consumer-side interfaces; test doubles are not exported |
| `var _ Iface = (*T)(nil)` compliance check (Uber) | None in adapters | It would make `repository/sqlite` import `service`; the wiring in `app` already fails to compile |
| Assertion helpers are not idiomatic (Google) | `assertInvariants(t, database)` with `t.Helper()` | One invariant check reused by every database test; a deliberate project choice |
| Short names for short-lived variables and receivers (`w`, `r`, `tx`, `i`, `s *Store`) | Full words everywhere; only `ctx`, `err`, `t`, `got` and `want` stay short | The reader is new to Go, and the user wants every name to say what it holds |
| A doc comment on every package and exported name (Go Code Review Comments, Google) | No comments in Go code, only toolchain directives (`//go:embed`) | The user's choice: Go requires none; names carry the meaning, and the reasons go to the chat or to `docs/` |

## Review checklist

1. Does every new interface live in the package that uses it, and does a fake or a second implementation actually use it?
2. Is every rule that depends on stored state evaluated inside the repository transaction, through the methods of the `Account` aggregate, with no network call inside?
3. Is each error handled exactly once, and is every ignored error explicit (`_ =`), except the deferred `transaction.Rollback()`?
4. Is no driver or transport error reachable with `%w` from outside `repository/sqlite` or `limits`?
5. Do successful paths return a literal `nil`, and does every `errors.As` target match the form of the returned error?
6. Does every `writeError` or `http.Error` call return immediately?
7. Does every outgoing HTTP call have a timeout and close its body, and does `http.Server` set its timeouts?
8. Does every goroutine have an owner that cancels it and waits for it, and does no request context outlive its request?
9. Are structs holding a mutex or an atomic used through pointers only, with the mutex unexported and not embedded?
10. Is there no new global state, `init()`, embedded dependency, pointer to an interface, or registry?
11. Are request and response structs, row scanning and domain types separate, with no `json` or `db` tags on domain types and no annotation comments?
12. Can a Go newcomer follow each function: early returns, no shadowed `ctx` or `err`, no `Get` prefixes, MixedCaps constants, enums with an invalid zero value?
13. Do tests call `t.Fatal` only from the test goroutine, keep table rows free of behaviour flags, name subtests like identifiers, and print input, got and want?
14. Does each new branch have a test that reproduces the real failure, and does each concurrency test count every outcome?
15. Is the code free of comments other than toolchain directives (`//go:embed`, `//go:build`)?

## Sources

- Go Code Review Comments: https://go.dev/wiki/CodeReviewComments
- Google Go Style Guide: https://google.github.io/styleguide/go/decisions and https://google.github.io/styleguide/go/best-practices
- Uber Go Style Guide: https://github.com/uber-go/guide/blob/master/style.md
- Errors in Go 1.13: https://go.dev/blog/go1.13-errors · Package names: https://go.dev/blog/package-names
- Module layout: https://go.dev/doc/modules/layout · Go 1.25 (`WaitGroup.Go`, `testing/synctest`): https://go.dev/doc/go1.25
- Ben Johnson, Standard Package Layout: https://www.gobeyond.dev/standard-package-layout/
- Three Dots Labs: https://threedots.tech/post/database-transactions-in-go/, https://threedots.tech/post/repository-pattern-in-go/, https://threedots.tech/post/introducing-clean-architecture/, https://threedots.tech/post/common-anti-patterns-in-go-web-applications/
- 100 Go Mistakes: https://100go.co/
- r/golang, "Clean architecture best practices in Go": https://www.reddit.com/r/golang/comments/t9no58/clean_architecture_best_practices_in_go/ (consensus: start with a few layers and grow them; accept interfaces, return structs; do not import 2005-era Java habits)
