# Onboarding — Cogged

Welcome. Cogged is a Go framework for multitenant web/mobile backends on the **Dgraph** graph
database, providing auth and **graph-based cross-user access control**. This guide gets you from
clone to your first change. For the deeper architecture reference, read [`CLAUDE.md`](./CLAUDE.md);
for the contribution workflow, [`CONTRIBUTING.md`](./CONTRIBUTING.md).

## 1. Prerequisites

- **Go** — version per the `go` directive in [`go.mod`](./go.mod).
- **Docker** — only for the integration tests (testcontainers starts Dgraph for you).
- **golangci-lint v2** — the lint pass is enforced in CI (v1 can't analyze this module).

## 2. Get it building and tested

```bash
git clone https://github.com/0x0abc123/cogged && cd cogged

go build ./...                     # compile everything
go test ./...                      # fast, offline unit tests — no DB, no network
go test -tags=integration ./...    # integration tests (needs Docker; boots ephemeral Dgraph)

./build.sh                         # -> ./bin/cogged
```

No database setup is needed for day-to-day work: the unit suite is fully hermetic, and the
integration suite provisions its own Dgraph via testcontainers.

## 3. How the code is laid out

| Area | Where |
|---|---|
| HTTP server, routing, CLI | `cmd/cogged/main.go` |
| HTTP handlers (one per route group) | `api/` |
| Request / response DTOs | `requests/`, `responses/` |
| Domain graph types + AuthzData logic | `models/` |
| Crypto, tokens, password hashing | `security/` |
| Config + all Dgraph access | `services/` |
| In-memory session manager | `state/` |

**The one concept to learn first:** access control. Nodes carry an owner and per-node permission
bits; the server hands clients signed **AuthzData** tokens instead of raw UIDs, verifies them on
the way back in, and only then acts. The `models/node.go` pack/unpack functions are the heart of
it — see the "access-control model" section of `CLAUDE.md`.

## 4. The development loop

```bash
go build ./... && go test ./...    # after each change
gofmt -w .                         # formatting is a blocking CI gate
go vet ./...
golangci-lint run                  # blocking CI gate; keep it at 0 issues
```

Adding an endpoint? Follow the checklist in `CLAUDE.md` (request DTO → response DTO → thin handler
→ routing → DB work behind the `DgraphClient` seam → OpenAPI entry → tests). Keep logic in
`services`/`models`; keep handlers as glue. **Never trust a client-supplied UID** — always
round-trip it through AuthzData verification. When you touch the access-control path, add both an
allow and a deny test.

## 5. Working with Claude Code

This repo ships a Claude Code harness under [`.claude/`](./.claude):

- **Commands:** `/test`, `/cover`, `/lint`, `/add-endpoint`, `/doc-sync`.
- **Subagents:** `dgraph-test-writer`, `api-feature`, `doc-writer`.
- A save hook auto-formats Go files, so you rarely hit the gofmt gate by hand.

`CLAUDE.md` is loaded automatically as project context, so Claude already knows the architecture,
conventions, and commands above.

## 6. CI and merging

Every PR runs [`.github/workflows/ci.yml`](./.github/workflows/ci.yml):

- **build-test** (blocking) — build, vet, `go test -race`, and a gofmt check.
- **golangci-lint** (blocking) — v2 with a clean baseline; new findings fail the build.
- **integration** (advisory) — `go test -tags=integration ./...` against ephemeral Dgraph.

Branch off `main`, open a PR, get CI green, and merge with a **merge commit** (keeps the tree
formatted and history linear-per-feature).

## Quick links

- Architecture & conventions: [`CLAUDE.md`](./CLAUDE.md)
- Contribution workflow: [`CONTRIBUTING.md`](./CONTRIBUTING.md)
- Framework concepts (DCG data model, access control): [`docs/about.md`](./docs/about.md)
- API reference: [`openapi3.yaml`](./openapi3.yaml)
