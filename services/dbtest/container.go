//go:build integration

package dbtest

import (
	"context"
	"testing"
	"time"

	svc "cogged/services"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// DgraphImage is the Dgraph standalone image used for integration tests.
// Keep it in sync with the version the framework targets (README / go docs).
const DgraphImage = "dgraph/standalone:v25.3.1"

// grpcPort is the Dgraph Alpha gRPC port; httpPort serves the health endpoint we wait on.
const (
	grpcPort = "9080/tcp"
	httpPort = "8080/tcp"
)

// MustStart boots an ephemeral Dgraph container, connects a *svc.DB to it (applying the
// latest schema via NewDB), and returns the DB plus a cleanup func. The cleanup is also
// registered with t.Cleanup, so callers may ignore the returned func if they prefer.
//
// It skips the test (t.Skip) rather than failing if Docker/testcontainers is unavailable,
// so a machine without Docker can still run the offline unit suite via `go test ./...`.
func MustStart(t *testing.T) (*svc.DB, func()) {
	t.Helper()
	ctx := context.Background()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        DgraphImage,
			ExposedPorts: []string{grpcPort, httpPort},
			WaitingFor: wait.ForHTTP("/health").
				WithPort(httpPort).
				WithStartupTimeout(180 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Skipf("skipping integration test: could not start Dgraph container: %v", err)
	}

	cleanup := func() {
		// give termination its own short-lived context
		tctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = container.Terminate(tctx)
	}
	t.Cleanup(cleanup)

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("container host: %v", err)
	}
	mapped, err := container.MappedPort(ctx, grpcPort)
	if err != nil {
		t.Fatalf("container mapped port: %v", err)
	}

	cfg := svc.Config{
		"db.host": host,
		"db.port": mapped.Port(),
	}

	// NewDB connects and applies the schema; it panics on connection failure, so retry
	// briefly to absorb the gap between the health check passing and gRPC being ready.
	var db *svc.DB
	deadline := time.Now().Add(60 * time.Second)
	for {
		db = tryNewDB(&cfg)
		if db != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("could not connect svc.DB to Dgraph at %s:%s", host, mapped.Port())
		}
		time.Sleep(2 * time.Second)
	}

	return db, cleanup
}

// tryNewDB wraps svc.NewDB, recovering its panic-on-failure into a nil return so the
// caller can retry.
func tryNewDB(cfg *svc.Config) (db *svc.DB) {
	defer func() {
		if r := recover(); r != nil {
			db = nil
		}
	}()
	return svc.NewDB(cfg)
}
