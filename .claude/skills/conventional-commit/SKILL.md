---
name: conventional-commit
description: How to write commit messages in this repository, following Conventional Commits 1.0.0. Use whenever a commit message is written, in particular from /push-code.
---

# Conventional commits

Specification: https://www.conventionalcommits.org/en/v1.0.0/

```
<type>(<scope>): <description>

<body, optional: why the change was made>

Refs: #<issue>
```

## Header

| Type | Use for |
|---|---|
| `feat` | New behaviour, including a newly handled edge case |
| `fix` | A bug in existing behaviour |
| `test` | Tests only, no production change |
| `refactor` | A code change that does not change behaviour |
| `docs` | Docs, tickets, README |
| `build` | Dockerfile, Compose, `go.mod` |
| `chore` | Anything else: tooling, `.claude/` |

- Scope: the package or area touched (`domain`, `service`, `sqlite`, `limits`, `handler`, `app`, `docker`, `docs`, `tasks`, `claude`). Omit it when the change spans many.
- Description: imperative mood, lowercase, no final period. Say what the change does for the reader ("refuse reservations beyond the storage limit"), not how ("add an if").
- The whole header fits in 72 characters.
- An incompatible change of the API contract or of the schema takes a `!` after the scope and a `BREAKING CHANGE:` footer.

## Body

- Optional. Explains why, not what. Lines wrap at 72 characters.
- Mentions the decision or problem it implements (D-number, P-number) when relevant.

## Footer

- `Refs: #<issue>`, the ticket's issue. Never `Closes`: the pull request closes the issue.
- No `Co-Authored-By` trailer, at the user's request: the use of AI is described in the README instead.

## Granularity

- One commit per validated TDD cycle, or per coherent change. The tests pass at every commit.
- Never mix unrelated changes. Never commit failing tests, databases, coverage profiles, binaries or secrets.

## Examples

```
feat(domain): refuse reservations beyond the storage limit

The check subtracts instead of adding, so it cannot overflow.

Refs: #6
```

```
test(sqlite): prove concurrent reservations never exceed the limit

Refs: #7
```

```
build(docker): run as non-root with a writable data volume

Refs: #1
```
