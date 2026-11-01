---
name: clean-architecture
description: Clean architecture for this Go service — business rules independent of HTTP, SQLite and the external limit source, dependencies pointing inward, and no layer or abstraction without a current need. Use when deciding where code goes, adding a feature across layers, or reviewing package dependencies; /implement-task, the tdd skill and /review-task load it.
---

# Clean architecture

The package table, the dependency rule and "where code goes" are in `.claude/rules/architecture.md`; the full picture is in `docs/architecture.md`. This skill explains why they exist, how to apply them when adding code, and how to check them.

## Why

- The business rules (admission, idempotency, release) are what reviewers care about. In `domain`, free of HTTP, SQL and network code, they are tested in microseconds and read without infrastructure noise.
- Each piece of infrastructure (SQLite, `net/http`, env.hivenet.com) changes for its own reasons. Isolated in its own package, a change touches that package only: moving to PostgreSQL adds `repository/postgres` and nothing else changes.
- Dependencies point inward, so an inner package never breaks because an outer one changed.

## Minimal, not ceremonial

- A layer is a package. No `usecase`, `entities`, `interactor`, `ports` or `adapters` packages, no file per pattern.
- An interface is declared by the package that uses it (`service/ports.go`), and only when a second implementation or a test fake exists. Go satisfies interfaces implicitly, so the implementation never imports it.
- No mapper layer: converting a DTO or a row to a domain type is a plain function next to the code that needs it.
- No dependency-injection framework: `internal/app` builds everything by hand.
- Skip a layer that would only pass a call through. `/healthz` has no service and no domain code, because no rule stands behind it.

## Where does this code go?

Ask in this order; the first yes wins.
1. A business rule or a calculation on reservations, without I/O? → `domain`, as a pure function with a table test.
2. The coordination of a use case (in-flight guard, clock, IDs, calls to the repository and the limit provider)? → `service`.
3. SQL? → `repository/sqlite`. The call to env.hivenet.com? → `limits`. Our HTTP API? → `handler`.
4. Configuration, construction of components, process lifecycle? → `app`.
5. None of these: ask the user before creating a package.

## Adding a feature across layers

From the inside out, one TDD cycle at a time, skipping the steps the feature does not need:
1. `domain`: the rule as a pure function, and its error types.
2. `service`: the use case, tested against fakes of its ports.
3. The adapter: repository or client, tested against the real dependency (SQLite file, `httptest` server).
4. `handler`: the endpoint, its DTOs and its error mapping.
5. `app`: the wiring, if a new component appeared.

## Boundaries

- A rule that depends on stored state runs inside the repository transaction, through a decide function from `domain` (update-func pattern). The service never holds a transaction.
- Each adapter translates its dependency's errors into domain errors: no `database/sql`, `net/http` or driver type crosses a boundary inward.
- Domain types carry no `json` or `db` tags; handlers have their own DTOs, and repositories scan into local variables.
- `context.Context` crosses every boundary that does I/O; nothing else from the outer layers does.

## Checking it

List each internal package with its imports, and compare with the table in `.claude/rules/architecture.md`:

```sh
go list -f '{{.ImportPath}}: {{join .Imports " "}}' ./internal/... | sed 's#github.com/guillaumebarbas/reserve-file/##g'
```

Checklist:
1. Does each package import only what the dependency rule allows: `domain` the standard library only, adapters never each other, `app` alone importing everything?
2. Is every business rule in `domain`, as a pure function, rather than in a handler, the service or SQL?
3. Does every rule that depends on stored state run inside the repository transaction?
4. Does an HTTP, SQL or driver type appear in a `domain` or `service` signature?
5. Is every new interface declared by its consumer, and used by a second implementation or a fake?
6. Is every new package, layer or abstraction needed now rather than "for later"?
7. Can a feature be followed from `handler` to `domain` without opening an unrelated package?

## Sources

- Robert C. Martin, The Clean Architecture: https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html
- Three Dots Labs, Introducing Clean Architecture: https://threedots.tech/post/introducing-clean-architecture/
- The r/golang discussion summarised in the `go-best-practices` skill: start with few layers and grow them only when needed.
