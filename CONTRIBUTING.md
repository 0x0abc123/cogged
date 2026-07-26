# Contributing to Cogged

Thanks for helping improve Cogged. This guide covers the day-to-day development loop.
For architecture and conventions, read [`CLAUDE.md`](./CLAUDE.md) first — it is the fastest way
to understand the packages and the graph-based access-control model.

## Prerequisites

- Go (see the `go` directive in [`go.mod`](./go.mod) for the minimum version).
- Docker — only needed to run the **integration** tests (testcontainers boots Dgraph for you).
- `golangci-lint` **v2** (the lint pass is enforced in CI; v1 cannot analyze this module —
  its Go build version is older than the `go.mod` target). Install: `golangci-lint` v2.x.

## Development loop

```bash
go build ./...                  # compile everything
go test ./...                   # fast, offline unit tests — no database, no network
gofmt -l .                      # files needing formatting (new/changed files should be empty here)
go vet ./...
```

### Tests

Cogged uses a **layered** test strategy:

1. **Unit tests** (default, `go test ./...`) never touch a database. They cover pure logic —
   query construction and helpers in `services/db.go`, the AuthzData pack/verify + permission
   logic in `models/node.go`, crypto and tokens in `security`, and DTO validation in `requests`.
2. **Fake-client tests** exercise the DB layer through the `DgraphClient` interface seam
   (`services/db.go`) with an in-memory fake — see `services/dbfake_test.go`. Still no real DB.
3. **Integration tests** run against a real, ephemeral Dgraph started by
   [`services/dbtest`](./services/dbtest) via testcontainers. They are gated behind a build tag so
   the default suite stays fast and hermetic:

   ```bash
   go test -tags=integration ./...    # requires Docker
   ```

   If Docker is unavailable the integration helper calls `t.Skip`, so nothing hard-fails.

When you change anything in the access-control path (AuthzData, permissions, sharing), add both
an **allow** and a **deny** test case.

## Adding an endpoint

Use the checklist in `CLAUDE.md` (or the `/add-endpoint` Claude Code command). In short:
request DTO with `Validate()`/`AuthzDataUnpack()` → response DTO with `AuthzDataPack()` → thin
handler on the right `api/*.go` group → routing/allowlist wiring in `cmd/cogged/main.go` → DB work
in `services/db.go` behind the `DgraphClient` seam → `openapi3.yaml` entry → tests.

Keep logic in `services`/`models`; keep handlers as glue. Never trust a client-supplied UID —
round-trip it through AuthzData verification.

## Style

- Tabs for indentation; match the terse existing style.
- Standard import aliases: `cm/svc/sec/req/res/state` (see any existing file).
- All code must be `gofmt` clean — CI fails on any unformatted file. Run `gofmt -w .` before
  committing (the Claude Code PostToolUse hook does this automatically on save).

## Continuous integration

[`.github/workflows/ci.yml`](./.github/workflows/ci.yml) runs on every push to `main` and every
PR:

- **build-test** (blocking): `go build`, `go vet`, `go test -race ./...` (unit only), and a
  `gofmt` check that fails on any unformatted file. The unit suite includes an
  `openapi3.yaml` drift guard (`spec/`) that fails if a handler route or a
  `GraphNode`/`GraphUser` model field has no matching entry in the spec — if it trips, update
  `openapi3.yaml` (see `/doc-sync`), don't weaken the test.
- **lint** (blocking): `golangci-lint` v2 (errcheck, govet, ineffassign, misspell, staticcheck,
  unused). The tree is clean, so any new finding fails the build.
- **integration** (advisory for now): `go test -tags=integration ./...` with Docker.

## Claude Code

This repo ships a Claude Code harness under [`.claude/`](./.claude): commands (`/test`, `/cover`,
`/lint`, `/add-endpoint`, `/doc-sync`) and subagents (`dgraph-test-writer`, `api-feature`,
`doc-writer`). A PostToolUse hook auto-formats Go files on save.
