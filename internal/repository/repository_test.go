//go:build integration
package repository_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"eventmetrics/internal/domain"
	repo "eventmetrics/internal/repository"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"path/filepath"
)

var chConn clickhouse.Conn
var repository *repo.Repository
var migrationsDir string

func TestMain(m *testing.M) {
	if os.Getenv("INTEGRATION_TEST") != "1" {
		// skip running container/tests unless explicit
		fmt.Println("INTEGRATION_TEST not set; skipping integration tests")
		os.Exit(0)
	}

	ctx := context.Background()

	req := tc.ContainerRequest{
		Image:        "clickhouse/clickhouse-server:24.8-alpine",
		ExposedPorts: []string{"9000/tcp"},
		Env: map[string]string{
			"CLICKHOUSE_PASSWORD": "clickhouse",
		},
		WaitingFor: wait.ForListeningPort("9000/tcp").WithStartupTimeout(2 * time.Minute),
	}
	container, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		fmt.Println("failed to start clickhouse container:", err)
		os.Exit(1)
	}

	host, err := container.Host(ctx)
	if err != nil {
		fmt.Println("failed to get container host:", err)
		_ = container.Terminate(ctx)
		os.Exit(1)
	}
	port, err := container.MappedPort(ctx, "9000")
	if err != nil {
		fmt.Println("failed to get mapped port:", err)
		_ = container.Terminate(ctx)
		os.Exit(1)
	}

	addr := fmt.Sprintf("%s:%s", host, port.Port())

	opts := &clickhouse.Options{
		Addr: []string{addr},
		Auth: clickhouse.Auth{
			Database: "default",
			Username: "default",
			Password: "clickhouse",
		},
	}

	// Use repo.Connect for pooled connection with exponential-backoff ping retries.
	repoCfg := repo.Config{
		QueryTimeout:   30 * time.Second,
		MaxRetries:     10,
		RetryBaseDelay: 1 * time.Second,
		RetryMaxDelay:  5 * time.Second,
	}
	var connectErr error
	repository, connectErr = repo.Connect(ctx, opts, repoCfg)
	if connectErr != nil {
		fmt.Println("failed to connect to clickhouse:", connectErr)
		_ = container.Terminate(ctx)
		os.Exit(1)
	}

	// chConn is a raw connection used for direct Exec and ApplyMigrations calls in tests.
	// The server is confirmed up by repo.Connect above, so no retry is needed here.
	chConn, err = clickhouse.Open(opts)
	if err != nil {
		fmt.Println("failed to open raw clickhouse connection:", err)
		_ = repository.Close()
		_ = container.Terminate(ctx)
		os.Exit(1)
	}

	// ensure clean state
	_ = chConn.Exec(ctx, "DROP TABLE IF EXISTS events")

	// run migrations
	// migrations dir is at project root
	migrationsDir = filepath.Join("..", "..", "migrations")
	if err := repo.ApplyMigrations(ctx, chConn, migrationsDir); err != nil {
		fmt.Println("failed to run migrations:", err)
		_ = container.Terminate(ctx)
		os.Exit(1)
	}

	code := m.Run()

	_ = container.Terminate(ctx)
	os.Exit(code)
}

func TestRepository_Ping(t *testing.T) {
	ctx := context.Background()
	if err := repository.Ping(ctx); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

func TestRepository_RunMigrations_Idempotent(t *testing.T) {
	ctx := context.Background()
	// run migrations twice using the same migrationsDir as TestMain
	if err := repo.ApplyMigrations(ctx, chConn, migrationsDir); err != nil {
		t.Fatalf("first ApplyMigrations failed: %v", err)
	}
	if err := repo.ApplyMigrations(ctx, chConn, migrationsDir); err != nil {
		t.Fatalf("second ApplyMigrations failed: %v", err)
	}
}

func TestRepository_Flush_BatchInsert_and_Query(t *testing.T) {
	ctx := context.Background()
	now := uint64(time.Now().UnixMilli())
	events := []domain.Event{
		{EventName: "e1", UserID: "u1", TimestampMs: now, PayloadHash: 1, Channel: "web"},
		{EventName: "e1", UserID: "u2", TimestampMs: now, PayloadHash: 2, Channel: "mobile"},
		{EventName: "e1", UserID: "u3", TimestampMs: now, PayloadHash: 3, Channel: "web"},
	}
	if err := repository.Flush(ctx, events); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// allow ClickHouse to accept data
	time.Sleep(500 * time.Millisecond)

	params := domain.QueryParams{EventName: "e1", From: now - 10000, To: now + 10000}
	res, err := repository.Query(ctx, params)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if res.TotalCount != 3 {
		t.Fatalf("expected total_count=3, got %d", res.TotalCount)
	}
	if res.UniqueCount != 3 {
		t.Fatalf("expected unique_count=3, got %d", res.UniqueCount)
	}
}

func TestRepository_DuplicateInsert_and_QueryWithFinal(t *testing.T) {
	ctx := context.Background()
	now := uint64(time.Now().UnixMilli())
	ev := domain.Event{EventName: "dup", UserID: "u1", TimestampMs: now, PayloadHash: 42, Channel: "web"}
	// insert twice
	if err := repository.Flush(ctx, []domain.Event{ev}); err != nil {
		t.Fatalf("first Flush failed: %v", err)
	}
	if err := repository.Flush(ctx, []domain.Event{ev}); err != nil {
		t.Fatalf("second Flush failed: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	params := domain.QueryParams{EventName: "dup", From: now - 10000, To: now + 10000}
	res, err := repository.Query(ctx, params)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if res.TotalCount != 1 {
		t.Fatalf("expected total_count=1 after duplicate inserts, got %d", res.TotalCount)
	}
}

