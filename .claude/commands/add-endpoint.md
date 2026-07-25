---
description: Scaffold a new Cogged API endpoint end-to-end (request/response DTOs, handler, routing, OpenAPI, tests)
---

Add a new HTTP endpoint following Cogged's conventions. Target: `$ARGUMENTS`
(e.g. "POST /graph/tags to attach tags to a node").

Work through the checklist in `CLAUDE.md` ("Adding an endpoint"), reading the closest existing
endpoint first to mirror its style:

1. **Request DTO** — new type in `requests/`, implement `Validate() bool` (`requests/validater.go`)
   and, if it carries node/user AuthzData tokens, `AuthzDataUnpack(uad, permsRequired)` — reuse
   `cm.AuthzDataUnpackADStringSlice` / `AuthzDataUnpackADString` rather than reimplementing checks.
2. **Response DTO** — in `responses/`; implement `AuthzDataPack(uad)` if it returns nodes/users so
   outbound UIDs are replaced by signed AuthzData.
3. **Handler** — add a method on the correct `api/*.go` group, dispatched from that group's
   `HandleRequest` by the `"METHOD endpoint"` key. Keep it thin: validate → unpack authz → call
   `services`/`models` → pack response.
4. **DB layer** — any Dgraph work goes in `services/db.go` behind the `DgraphClient` seam. Use the
   short predicate names; sanitize UIDs via `SanitiseUID`.
5. **Routing** — wire in `cmd/cogged/main.go` if a new route group; add to `allowList`
   (unauthenticated) or `adminList` (admin-only) as needed.
6. **OpenAPI** — add the path/schema to `openapi3.yaml`.
7. **Tests** — a pure/fake-client unit test for validation + authz logic, and (if it hits the DB)
   an integration subtest using `services/dbtest`, behind `//go:build integration`.

Finish by running `/test` and `/lint`. Do not invent new patterns where an existing one fits.
