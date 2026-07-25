---
description: Run Cogged tests — fast unit tests by default, or integration (testcontainers Dgraph) with --integration
allowed-tools: Bash(go test:*), Bash(go build:*)
---

Run the test suite for the current change.

Arguments: `$ARGUMENTS`

- Default (no args): run fast, offline unit tests only:
  `go test ./...`
- If `$ARGUMENTS` contains `--integration` or `-i`: also run the integration tests, which use
  testcontainers to boot an ephemeral Dgraph (requires Docker running):
  `go test -tags=integration ./...`
- If `$ARGUMENTS` names a package (e.g. `services`), scope to it: `go test ./services/...`.
- Pass through `-run <regex>` / `-v` if present in `$ARGUMENTS`.

Always `go build ./...` first if the last edit could have broken compilation. Report failures
with the relevant output; do not mark work done while tests fail.
