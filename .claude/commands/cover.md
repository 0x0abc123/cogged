---
description: Report test coverage and surface untested packages/functions
allowed-tools: Bash(go test:*), Bash(go tool:*)
---

Measure coverage and identify gaps.

1. Run: `go test -coverprofile=/tmp/cogged.cover ./...`
2. Summarize per-function coverage: `go tool cover -func=/tmp/cogged.cover | sort -k3 -n`
3. Report the lowest-covered packages and the most important **untested** exported functions
   (prioritize `services/db.go` query construction, `models/node.go` AuthzData logic, and
   `security` crypto/auth).
4. If `$ARGUMENTS` asks, write untested-function suggestions or generate an HTML report:
   `go tool cover -html=/tmp/cogged.cover -o /tmp/cogged-cover.html`.

Only pure/fake-client-testable code counts toward the offline number; note anything that needs
the `integration` tag separately.
