---
description: Check and refresh docs — OpenAPI vs actual routes, about.md, and godoc — against the code
allowed-tools: Bash(go doc:*), Bash(grep:*), Bash(rg:*)
---

Keep documentation in sync with the code.

1. **Routes vs OpenAPI.** Enumerate the real routes: the `switch routeGroup` in
   `cmd/cogged/main.go` plus each group's `HandleRequest` keys in `api/*.go`. Compare against the
   paths in `openapi3.yaml`. Report missing, extra, or mismatched entries; add/fix OpenAPI entries.
2. **Schema/config.** Verify the predicate list in `docs/about.md` and any config keys match
   `services/dbsetup.go` and `services/config.go`.
3. **about.md.** Update sections that describe behavior which has since changed.
4. **godoc.** Ensure each package's primary file has a package comment; add short doc comments on
   exported types/functions that lack them.

Scope: `$ARGUMENTS` (if given, restrict to that area, e.g. "auth endpoints"). Make documentation
changes only — never change behavior here. Summarize every drift found and what you changed.
