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

- [x] `api/openapi.yaml` describes every endpoint, the headers (`Idempotency-Key`, `Idempotent-Replayed`, `Location`, `Retry-After`, `X-Request-Id`), the bodies, and every error code of api.md as `application/problem+json`.
- [x] The file passes a linter run in Docker (`redocly/cli lint`).
- [x] A `docs` Compose profile starts `swaggerapi/swagger-ui` on host port `${DOCS_PORT:-8081}` with this file.
- [x] "Try it out" works against the running service: the API allows the Swagger UI origin through CORS, configured with `CORS_ALLOWED_ORIGIN` and off by default.
- [x] api.md points to the OpenAPI file, and the two stay consistent.

## User acceptance criteria

The user is the reviewer.

- Given the stack is running, when they run `docker compose --profile docs up`, then http://localhost:8081 shows every endpoint with its errors.
- Given they use "Try it out" on reserve, then the request reaches the service and the response is displayed.

## Tests

- [x] The OpenAPI lint passes.
- [x] `TestCORS_AllowsConfiguredOrigin` (unit, `internal/handler`) — preflight allowed for the configured origin, refused for others, and no CORS headers when nothing is configured.
- [x] `TestLoadConfig_CORSAllowedOrigin` *(added, unit, `internal/app`)* — off by default, an origin is read; three rows of `TestLoadConfig_InvalidValues` refuse a value without a scheme, with a path, or with a trailing slash, naming the variable.
- [x] `TestRun_AllowsTheConfiguredCORSOrigin` *(added, integration, `internal/app`)* — a preflight to the real `app.Run` gets `204` with the allowed origin, proving the wiring.

## Test results

Run on 2026-11-01.

- `gofmt -l .` prints nothing; `go vet ./...` passes; no Go comment apart from `//go:embed`.
- `go test ./...` (also with `-shuffle=on`): 118 tests and 173 subtests pass, none fail or skip.
- `docker compose run --rm test`: passes with the race detector; coverage of `./internal/...` is 100.0%.
- Test first: `TestLoadConfig_CORSAllowedOrigin` and the three invalid rows failed against a stub field; `TestCORS_AllowsConfiguredOrigin` failed on four rows against a `WithCORS` that returned the next handler; `TestRun_AllowsTheConfiguredCORSOrigin` got `405` before `app` wrapped the routes.
- `docker compose run --rm openapi-lint` (`redocly/cli:2.72.1` pinned by digest, the rules of `api/redocly.yaml`): "Woohoo! Your API description is valid", no warning. Three rules of the recommended set are off, each for a reason: `info-license` (the project has no license), `no-server-example.com` (the only server is the local Compose stack) and `operation-4xx-response` (`/healthz` and `/readyz` have no client error).
- Stack started with `CORS_ALLOWED_ORIGIN=http://localhost:18108 docker compose --profile docs up --build`, ports 18109 (API) and 18108 (docs) on this machine, removed afterwards:
  - Swagger UI answers `200`, and its initializer loads `./openapi.yaml`, served at `200`;
  - "Try it out" on reserve, replayed with curl exactly as the browser sends it: the preflight `OPTIONS` with `Origin: http://localhost:18108`, `Access-Control-Request-Method: POST` and `Access-Control-Request-Headers: content-type,idempotency-key` gets `204` with the allowed origin, methods and headers; the `POST` then gets `201` with `Access-Control-Allow-Origin` and `Access-Control-Expose-Headers`, so the page can read `Location` and `X-Request-Id`;
  - a preflight from `http://attacker.example` gets no CORS header (`405` from the router), so a browser blocks it.
- Not done: the click itself in a browser. No browser was available to this agent; the two requests above are what the browser sends.
