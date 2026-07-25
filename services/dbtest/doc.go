// Package dbtest provides helpers for spinning up an ephemeral Dgraph instance
// (via testcontainers) for integration tests.
//
// The actual container code is behind the `integration` build tag, so this package
// compiles to an empty package in normal builds and adds no runtime dependencies to
// the main binary. Use it only from tests tagged `//go:build integration`:
//
//	//go:build integration
//
//	db, cleanup := dbtest.MustStart(t)
//	defer cleanup()
//
// Run with: go test -tags=integration ./...
package dbtest
