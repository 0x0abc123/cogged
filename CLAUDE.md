# CLAUDE.md

Guidance for Claude Code when working in the **Cogged** repo.

## What this is

Cogged is a lightweight Go framework for multitenant web/mobile backends, backed by the
**Dgraph** graph database (v22.0.2). It provides authentication, a graph data schema, and
**graph-based cross-user access control**. Data is modelled as Directed Cyclic Graphs (DCGs);
every node carries an owner and per-node permissions, and access is granted by sharing edges.

- Module: `cogged` (see `go.mod`), Go 1.20.
- Entry point / HTTP server: `cmd/cogged/main.go`.

## Package map

| Package | Responsibility |
|---|---|
| `cmd/cogged` | `main`, HTTP server, routing (`DefaultHandler.ServeHTTP`), CLI flags, `-adduser`. |
| `api` | HTTP handlers, one per route group, each implements `api.Handler` (`api/handler.go`). Files: `auth.go`, `admin.go`, `graph.go`, `user.go`, `health.go`, plus `marshal.go`, `errors.go`. Keep handlers thin. |
| `requests` | Inbound JSON DTOs. Two seams: `Validater.Validate()` (`validater.go`) and `AuthzDataUnpacker.AuthzDataUnpack()` (`adunpacker.go`) which converts client-supplied AuthzData tokens back to real UIDs after checking access. |
| `responses` | Outbound JSON DTOs. `AuthzDataPacker.AuthzDataPack()` (`adpacker.go`) stamps signed AuthzData onto nodes/users before they leave the server. |
| `models` | Domain graph types: `GraphNode`, `GraphUser`, `GraphBase` (embeds Uid/type/AuthzData), `Geoloc`. The AuthzData pack/unpack + permission logic lives in `models/node.go`. |
| `security` | Crypto + auth: Argon2id password hashing, HMAC-SHA256 MACs, bearer token construct/verify (`auth.go`), AES-GCM, GUID/SGI generation (`crypto.go`). |
| `services` | `config.go` (flat JSON config), `db.go` (all Dgraph access), `dbsetup.go` (Dgraph schema + versioning). |
| `state` | In-memory user-session manager (`Usm*`): tracks live token IDs and which SGIs a user may access. |
| `client` | Minimal Go API client. |
| `log` | Tiny leveled logger. |

## The access-control model (the least obvious part — read this first)

Nodes are stored in Dgraph with **short predicate names** (`services/dbsetup.go` is the schema):

- Identity/edges: `uid`, `own` (owner uid), `e` (out-edges, `@reverse`), `nodes` (user→node),
  `shr`/`~shr` (share edge + reverse), `sgi` (share-group id).
- Permission bools: `r` read, `w` write, `o` out-edge, `i` in-edge, `d` delete, `s` share.
- Data fields: `ty` type, `id`, `p` private, `s1..s4` strings, `b` blob, `n1/n2` floats,
  `c` created, `m` modified, `t1/t2` times, `g` geo.
- User fields: `un` username, `ph` password hash, `us` userdata, `intd` internal, `role`.

**AuthzData tokens** are how the server avoids trusting client-supplied UIDs:

- Outbound: `GraphNode.AuthzDataPack(uad)` builds `uid.owner.sgi.perms`, MACs it with the
  per-user key, and puts the result in the node's `ad` field (`models/node.go`).
- Inbound: `AuthzDataUnpack` / `AuthzDataUnpackADString` verify the MAC, then allow the op only
  if the caller **owns** the node, is role `sys`, or **can access the SGI and holds the required
  permissions** (`state.UsmUserCanAccessSgi` + `HasRequiredPermissions`). Only then is the real
  UID substituted back in. Changing anything here without a test is dangerous.

Per-user keys derive from a master secret: `UserKeyFromMasterSecret(master, uid, role)`.

## Build / run / test

```bash
./build.sh                      # go build -o ./bin/cogged ./cmd/cogged
go build ./...                  # compile everything

go test ./...                   # fast, offline unit tests (no DB, no network)
go test -tags=integration ./... # integration tests: testcontainers boots ephemeral Dgraph (needs Docker)
go test -cover ./...            # coverage

gofmt -l .                      # list unformatted files (should be empty)
go vet ./...
golangci-lint run               # if installed
```

Prefer the slash commands: `/test`, `/cover`, `/lint`, `/add-endpoint`, `/doc-sync`.

**Testing model:** unit tests never touch a database — they cover pure logic (query
construction, AuthzData round-trips, crypto, validation) and the DB layer via a fake
`DgraphClient`. Anything needing a real Dgraph goes behind the `//go:build integration` tag and
uses the `services/dbtest` helper (testcontainers). Keep `go test ./...` fast and hermetic.

## Config & env

- Config file `cogged.conf.json` (flat keys like `db.host`, `db.port`, `listen.host`,
  `listen.port`, `auth.tokenexpiry`). Resolution order: `-conf` flag → `COGGED_CONFIG_FILE` →
  cwd → exe dir (`services/config.go`).
- Env vars: `COGGED_KEY` (master secret; else `cogged.key` file, else random), `COGGED_CONFIG_FILE`.
- CLI overrides: `-dh`/`-dp` (Dgraph host/port), `-ip`/`-p` (listen), `-adduser username,role`.

## Conventions

- **Tabs** for indentation. Match the terse existing style.
- Import aliases are consistent across the repo: `cm "cogged/models"`, `svc "cogged/services"`,
  `sec "cogged/security"`, `req "cogged/requests"`, `res "cogged/responses"`, `state "cogged/state"`.
- Errors: return `error`; DB layer uses `DBError`; handler errors may carry a `StatusCode` field
  that `ServeHTTP` reflects into the HTTP status.
- HTTP routing is manual string-splitting in `ServeHTTP`: `/routegroup/endpoint[/:param]`, handler
  key is `"METHOD endpoint"`. Unauthenticated routes must be added to the `allowList`; admin-only
  groups to the `adminList` (both in `CreateDefaultHandler`).
- Put real logic in `services`/`models`, keep `api` handlers as glue.

## Adding an endpoint (checklist)

1. Request DTO in `requests/` implementing `Validate()` (and `AuthzDataUnpack()` if it carries
   node/user AuthzData).
2. Response DTO in `responses/` (implement `AuthzDataPack()` if it returns nodes/users).
3. Handler method on the right `api/*.go` group, dispatched by `HandleRequest`.
4. Wire routing / `allowList` / `adminList` in `cmd/cogged/main.go` if needed.
5. DB work in `services/db.go` (behind the `DgraphClient` seam).
6. Add an `openapi3.yaml` entry and tests (unit first; integration if it hits the DB).

## Known rough edges (verify with a test before "fixing")

- `cmd/cogged/main.go:121` dereferences `userAuthData.IsAdmin()` — `userAuthData` can be `nil`
  for allowlisted routes; only safe today because no allowlisted route is under an admin group.
- No-op self-assignments exist (`statusCode = statusCode`, `configFilePath = configFilePath`);
  harmless but flagged by linters.
- `NewDB` panics on connect failure and eagerly alters the Dgraph schema; use `NewDBWithClient`
  in tests to avoid that path.
