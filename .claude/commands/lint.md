---
description: Format-check, vet, and lint the Go code
allowed-tools: Bash(gofmt:*), Bash(go vet:*), Bash(golangci-lint:*)
---

Run the static-analysis pass and fix what is safe to fix.

1. `gofmt -l .` — list unformatted files. If any, run `gofmt -w` on them.
2. `go vet ./...` — report and fix vet issues.
3. `golangci-lint run` if the binary is installed (see `.golangci.yml`); otherwise say it was
   skipped (not installed) and rely on `go vet`.

Report a concise summary. Do not make behavioral changes under the guise of linting — only
formatting and clearly-safe fixes. Flag anything requiring a judgement call instead of silently
changing it.
