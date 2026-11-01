---
paths:
  - "**/*.go"
---

# Go style

The reader is new to Go: choose the most obvious construct, not the shortest or the cleverest.

## Formatting

- `gofmt` output only. Imports in two groups: standard library, then everything else.

## Naming

- Packages: short, lowercase, singular, no underscores (`domain`, `sqlite`, `limits`).
- No stutter: `service.New`, not `service.NewReservationService`.
- Short names only where Go convention expects them: `ctx`, `err`, `tx`, `db`, `w`, `r`, `t`, `i`, and one- or two-letter receivers (`s *Service`). Everything else gets a descriptive name: `reservedBytes`, not `rb`.
- Initialisms keep a single case: `accountID`, `requestID`, `URL`, `HTTPAddr`.
- Booleans read as statements: `known`, `replayed`, `exists`.
- Quantities carry their unit: `limitBytes`, `availableBytes`. Durations are `time.Duration`, never plain integers.

## Functions

- One job per function. Split it when it mixes several steps or grows past about 40 lines.
- Return early on errors so the main path stays unindented. No `else` after a `return`.
- `context.Context` is the first parameter of every function that does I/O. Never store it in a struct.
- More than four parameters besides `ctx`: pass a struct. `ctx` never goes inside a struct.
- No named results, no bare `return`.
- No boolean parameter that switches behaviour: write two functions.

## Errors

- Return errors. Never panic for an expected failure.
- Handle an error once: wrap and return it, or log it and stop. Never both.
- Wrap with context: `fmt.Errorf("release %s: %w", id, err)`. Lowercase message, no final punctuation.
- Inspect errors with `errors.Is` / `errors.As`, never by comparing strings.

## Types

- Constructors are named `New…` and return concrete types (`*Service`), not interfaces. An aggregate rebuilt from storage uses `Restore…` (`domain.RestoreAccount`).
- Byte counts are `int64`. Check capacity with a subtraction (`bytes > limit-reserved`), never with an addition that can overflow.
- Times are `time.Time` in UTC. Code that needs the current time receives a clock (`now func() time.Time`).
- No `any` in signatures, no generics, no reflection, unless the user agrees that it removes real duplication.

## Concurrency

- Goroutines only where the design requires them (the limits refresher, tests). Each one has an owner that stops it through a context and waits for it.
- Shared state: `sync.Mutex`, with `defer mu.Unlock()` right after `Lock()`. Values read on every request: `sync/atomic` types.
- Channels only to signal or hand over work, never as a lock.

## Comments

- No comments in Go code, package and doc comments included: Go requires none. Only the directives the toolchain reads stay (`//go:embed`, `//go:build`).
- What a comment would say goes into a name: `deadlineIfRunHangs`, `listensOnEveryInterface`, `synchronousFull`.
- A reason that no name can carry goes to the user in the chat, or into `docs/` when reviewers need it (server timeouts, health check host).
- No annotation comments (`// @Summary`, `// @Param`, swaggo or similar): the API is described in `docs/api.md`.
- No commented-out code, no TODOs.

## Logging

- `log/slog` with key-value pairs: `logger.Info("reservation created", "account_id", accountID)`.
- Never `fmt.Println` or the `log` package. Never log secrets, request bodies or the env.hivenet.com payload.

## Dependencies

- Standard library first. Allowed: `modernc.org/sqlite`, `github.com/google/uuid`. Ask the user before adding anything else.
