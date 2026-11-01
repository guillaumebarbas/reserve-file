# T12 — Demo script and README

| | |
|---|---|
| Issue | [#12](https://github.com/guillaumebarbas/reserve-file/issues/12) |
| Branch | `chore/t12-demo-and-readme` |
| Priority | P0 |
| Estimate | 25 min |
| Depends on | T01 to T11 |
| References | The brief, "What to submit"; [testing.md, Demo script](../docs/testing.md#demo-script) |

## Overview

What the reviewer reads and runs first. The README answers the four points of the brief. The demo script shows the main behaviours (success, retry, conflict, concurrency, release) against the running stack, with nothing but Docker.

## Technical acceptance criteria

- [x] `scripts/demo.sh` uses only POSIX shell and curl. It runs in a `curlimages/curl` container through the `demo` Compose service, prints each step with the expected and actual status, and exits non-zero at the first mismatch.
- [x] The steps are those of testing.md, on `small_account`. The script checks that nothing is reserved before it starts and releases what it created at the end, so it can be run again.
- [x] The README covers:
  - how to start the stack (`docker compose up --build`);
  - the database setup: automatic schema and seed at startup, data on the `data` volume, reset with `docker compose down -v`;
  - the configuration: each environment variable and its default (`ENV_CONFIG_URL`, `SEED_ACCOUNTS`…), and how to override it with Compose;
  - example requests with curl;
  - how to verify: the demo, `docker compose run --rm test`, and the container tests once T22 is done;
  - time spent, what is complete and what is unfinished;
  - how AI was used;
  - links to `docs/` and `tasks/`.
- [x] Every command in the README has been run on a fresh clone.
- [x] The base images (`golang`, `gcr.io/distroless/static`) are pinned by digest in the `Dockerfile` and `compose.yaml`, so the reviewer builds exactly what was tested. Raised in the review of #25.
- [ ] At submission, the repository and the project board are made public: `gh repo edit guillaumebarbas/reserve-file --visibility public --accept-visibility-change-consequences` and `gh project edit 2 --owner guillaumebarbas --visibility PUBLIC`.
- [x] The README is updated after every ticket done later.

## User acceptance criteria

The user is the reviewer, with only Docker.

- Given a fresh clone, when they follow the README, then they start the stack, run the demo and run the tests without installing anything else.
- Given the demo runs, then it shows a successful reservation, a retry returning the same reservation, a conflict, a concurrent burst that respects the limit, and a double release that gives the space back once.
- Given they want to know what is missing, then the README lists the unfinished work and the gaps that would block a release.

## Tests

- [x] `scripts/demo.sh` exits with 0 on a fresh stack.
- [x] `scripts/demo.sh` exits with 0 when run a second time.
- [x] The README commands are checked by hand on a fresh clone.

## Test results

### Part 1: README and pinned images (2026-11-01)

- The base images are pinned by the digest of their multi-platform index, tag kept for readability: `golang:1.27@sha256:2920bb12…` (the image the tests already ran on) and `gcr.io/distroless/static:nonroot@sha256:25ac8f0c…`.
- With the pinned images, on a separate Compose project with its own volumes, removed afterwards:
  - `docker compose up --build`: the container becomes `healthy` in `docker compose ps`;
  - `curl -i` on `/healthz` and `/readyz`: `200` with `X-Request-Id`, and matching lines in `docker compose logs api`;
  - `docker compose down -v` empties the database;
  - `SEED_ACCOUNTS=… docker compose up` and a `compose.override.yaml` setting `LOG_LEVEL` are both taken into account;
  - `API_PORT` in `.env` changes the published port;
  - `docker compose run --rm test`: passes with the race detector; coverage of `./internal/...` is 100.0%.
- `README.md` answers the brief's four points with what exists today: how to run, database setup, configuration, example requests, how to verify, status and time, how AI was used, and links. Its sections on endpoints, verification and status are filled in as the next tickets land.
- Remaining for this ticket: the demo script and its two runs, the README pass once T04 to T11 are done, every README command run on a fresh clone, and making the repository and the board public at submission.

### Part 2: demo script (2026-11-01)

- `scripts/demo.sh`: POSIX shell and curl only, run by the `demo` Compose service (profile `demo`, `curlimages/curl:8.23.0` pinned by digest, after `api` is healthy). Each step prints the expected and the actual answer; the first mismatch stops the script with exit code 1. A trap on exit releases every reservation the run created, even after a failure, and the keys carry a run identifier, so the demo can always run again.
- Run on a stack built from this branch (separate Compose project, removed afterwards), with the real limit source (`MAX_PARALLEL_CHUNKS` = 5):
  - steps 1 to 5 pass: nothing reserved at the start; `201` for 30 MiB; `200` with `Idempotent-Replayed: true` and the same ID on the retry, usage unchanged; `409 insufficient_storage` for 80 MiB with its detail; 20 concurrent requests of 20 MiB give exactly 3 `201` and 17 `409`, and usage shows 90 MiB and 4 active reservations;
  - step 6 fails as expected on this branch: the same key with 31 MiB is replayed with `200` until T14 refuses it with `422`. The script stops with exit code 1 and releases everything;
  - a second run passes the same steps, so the clean-up after a failure works;
  - a temporary copy without step 6 shows the first release at `200` and stops at the second release, which answers `500` until T10;
  - a temporary copy without the steps that need T10 and T14 passes to the end: the remaining reservations are released and usage is back to zero.
- Remaining: rerun the whole demo twice once T10 and T14 are merged, the README pass once T04 to T11 are done, every README command on a fresh clone, and the publication at submission.
- After T10 and T14 were merged, the integrator ran the whole demo twice in a row on a fresh stack built from `main` merged into this branch: both runs pass all nine steps and exit with 0. Step 5 gives exactly 3 `201` and 17 `409`, step 6 gives `422 idempotency_key_reused`, and step 7 answers `200` twice with the same `released_at` while the space comes back once.

### Part 3: final README pass and fresh clone (2026-11-01)

- README: final status (every ticket done), the endpoints table without its "Available" column, a design section that says how each case stays consistent, the time spent, and the gaps that would block a production release, linked to consistency.md. Through wave 5 the status was updated once, here, rather than in each pull request, so the nine pull requests did not conflict on the same lines.
- Every command of the README run on a fresh clone of this branch, on Windows with Docker Desktop, after `echo API_PORT=18110 > .env` as the README says for reserved ports:
  - `docker compose up --build`: `healthy`; `/healthz` and `/readyz` answer `200` with `X-Request-Id`, and the readiness body is `{"database":"ok","limits":"ok"}`;
  - usage, reserve (`201` with `Location`), a too large request (`409 insufficient_storage`), the retry (`200`, `Idempotent-Replayed: true`), the read, the release and a second release (`200` both times), an unknown ID (`404`), usage back to zero; `docker compose logs api` shows one line per request;
  - `docker compose restart api`: usage unchanged, the retry still replayed;
  - `docker compose run --rm demo`, twice: both runs pass all nine steps;
  - `docker compose run --rm test`: race detector, 100.0%;
  - `docker compose stop`: the log ends with `server stopped`, exit code 0;
  - `SEED_ACCOUNTS=…,small_account:52428800 docker compose up --build`: `small_account` at 50 MiB;
  - `CORS_ALLOWED_ORIGIN=… docker compose --profile docs up --build`, with `DOCS_PORT` in `.env`: Swagger UI and `openapi.yaml` answer `200`, and a preflight from that origin gets `204` with the CORS headers;
  - `docker compose run --rm openapi-lint`: valid;
  - `go test -tags e2e ./e2e/...`: the eight container tests pass in 60 s;
  - `docker compose down -v`: everything removed.
- Remaining: making the repository and the project board public at submission, on the author's go.
