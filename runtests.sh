#!/bin/bash
set -e

# Cogged test runner.
#   ./runtests.sh              # fast, offline unit tests (no database required)
#   ./runtests.sh integration  # integration tests via testcontainers (requires Docker)

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
cd "$SCRIPT_DIR"

if [ "$1" = "integration" ] || [ "$1" = "-i" ]; then
	echo "Running integration tests (testcontainers will start an ephemeral Dgraph; Docker required)..."
	go test -tags=integration -timeout 15m ./...
else
	echo "Running offline unit tests..."
	go test ./...
fi
