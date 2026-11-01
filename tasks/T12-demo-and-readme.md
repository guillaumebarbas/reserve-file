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

- [ ] `scripts/demo.sh` uses only POSIX shell and curl. It runs in a `curlimages/curl` container through the `demo` Compose service, prints each step with the expected and actual status, and exits non-zero at the first mismatch.
- [ ] The steps are those of testing.md, on `acc_small`. The script checks that nothing is reserved before it starts and releases what it created at the end, so it can be run again.
- [ ] The README covers:
  - how to start the stack (`docker compose up --build`);
  - the database setup: automatic schema and seed at startup, data on the `data` volume, reset with `docker compose down -v`;
  - the configuration: each environment variable and its default (`ENV_CONFIG_URL`, `SEED_ACCOUNTS`…), and how to override it with Compose;
  - example requests with curl;
  - how to verify: the demo, `docker compose run --rm test`, and the container tests once T22 is done;
  - time spent, what is complete and what is unfinished;
  - how AI was used;
  - links to `docs/` and `tasks/`.
- [ ] Every command in the README has been run on a fresh clone.
- [ ] The base images (`golang`, `gcr.io/distroless/static`) are pinned by digest in the `Dockerfile` and `compose.yaml`, so the reviewer builds exactly what was tested. Raised in the review of #25.
- [ ] At submission, the repository and the project board are made public: `gh repo edit guillaumebarbas/reserve-file --visibility public --accept-visibility-change-consequences` and `gh project edit 2 --owner guillaumebarbas --visibility PUBLIC`.
- [ ] The README is updated after every ticket done later.

## User acceptance criteria

The user is the reviewer, with only Docker.

- Given a fresh clone, when they follow the README, then they start the stack, run the demo and run the tests without installing anything else.
- Given the demo runs, then it shows a successful reservation, a retry returning the same reservation, a conflict, a concurrent burst that respects the limit, and a double release that gives the space back once.
- Given they want to know what is missing, then the README lists the unfinished work and the gaps that would block a release.

## Tests

- [ ] `scripts/demo.sh` exits with 0 on a fresh stack.
- [ ] `scripts/demo.sh` exits with 0 when run a second time.
- [ ] The README commands are checked by hand on a fresh clone.

## Test results

_Not run yet._
