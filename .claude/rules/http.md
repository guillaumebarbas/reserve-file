---
paths:
  - "internal/handler/**"
---

# HTTP layer rules

`docs/api.md` is the contract: paths, status codes, headers, error codes and messages. If the contract has to change, update it first, with the user's agreement.

- Standard library router: `mux.HandleFunc("POST /accounts/{accountID}/reservations", h.reserve)`; read path values with `r.PathValue("accountID")`.
- Handlers stay thin: parse and validate the input, call the service, write the response. No business rule, no SQL.
- Request and response bodies are dedicated structs with snake_case `json` tags, separate from domain types.
- Decode strictly: `http.MaxBytesReader` (4 KiB), `DisallowUnknownFields`, reject trailing data, and decode `bytes` into `*int64` to tell a missing field from zero.
- One function, `writeError`, maps errors to responses with `errors.Is` / `errors.As`: status, `code`, `detail`, extension fields, `Retry-After`. Unknown errors become `500 internal_error` with the request ID; the error itself only goes to the log.
- `detail` messages are full sentences with numbers in human units (GiB, MiB) and the next step for the client.
- Set headers, including `Content-Type`, before `WriteHeader`. Errors use `application/problem+json`.
- Middleware: request ID (`X-Request-Id`), access log, panic recovery.
- Never log request bodies.
