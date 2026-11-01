# T23 — OpenAPI description and Swagger UI

| | |
|---|---|
| Issue | [#23](https://github.com/guillaumebarbas/reserve-file/issues/23) |
| Branch | `feat/t23-openapi` |
| Priority | P2 (optional) |
| Estimate | 30 min |
| Depends on | T12 |
| References | [api.md](../docs/api.md) |

## Overview

A machine-readable description of the API and an interactive page to browse and try it.

- **OpenAPI** (called Swagger up to version 2.0) is the standard format that describes an HTTP API: endpoints, parameters, schemas, examples, errors. **Swagger UI** is a web page that displays such a file and lets you send the calls.
- The file is written by hand from api.md (spec-first). What Java code-first tools express with annotations such as `@Schema` or `@ExampleObject` is written in the YAML itself: `schema`, `example`, `required`, `minimum`, `maxLength`.
- Swagger UI runs in its own container: no Go dependency, no annotations in the code.
- The code-first alternative in Go, swaggo/swag, generates the file from `// @Summary`, `// @Param` and `// @Success` comments above each handler and from `example:` struct tags. It was not chosen: it adds a dependency, a generation step, and annotation comments to code that must stay easy to read.

## Technical acceptance criteria

- [ ] `api/openapi.yaml` describes every endpoint, the headers (`Idempotency-Key`, `Idempotent-Replayed`, `Location`, `Retry-After`, `X-Request-Id`), the bodies, and every error code of api.md as `application/problem+json`.
- [ ] The file passes a linter run in Docker (`redocly/cli lint`).
- [ ] A `docs` Compose profile starts `swaggerapi/swagger-ui` on port 8081 with this file.
- [ ] "Try it out" works against the running service: the API allows the Swagger UI origin through CORS, configured with `CORS_ALLOWED_ORIGIN` and off by default.
- [ ] api.md points to the OpenAPI file, and the two stay consistent.

## User acceptance criteria

The user is the reviewer.

- Given the stack is running, when they run `docker compose --profile docs up`, then http://localhost:8081 shows every endpoint with its errors.
- Given they use "Try it out" on reserve, then the request reaches the service and the response is displayed.

## Tests

- [ ] The OpenAPI lint passes.
- [ ] `TestCORS_AllowsConfiguredOrigin` (unit, `internal/handler`) — preflight allowed for the configured origin, refused for others, and no CORS headers when nothing is configured.

## Test results

_Not run yet._
