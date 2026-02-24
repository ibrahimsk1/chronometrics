package repository

import (
	"context"
	"time"

	"eventmetrics/internal/handler"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
)

type Config struct {
	QueryTimeout time.Duration
}

type Repository struct {
	conn clickhouse.Conn
	cfg  Config
}

// Close closes the underlying ClickHouse connection.
func (r *Repository) Close() error {
	return r.conn.Close()
}

// New constructs a Repository with an open ClickHouse connection.
func New(conn clickhouse.Conn, cfg Config) *Repository {
	if cfg.QueryTimeout == 0 {
		cfg.QueryTimeout = 30 * time.Second
	}
	return &Repository{conn: conn, cfg: cfg}
}

// Ping checks ClickHouse connectivity.
func (r *Repository) Ping(ctx context.Context) error {
	return r.conn.Ping(ctx)
}

// Health implements handler.HealthChecker by delegating to Ping.
func (r *Repository) Health(ctx context.Context) handler.HealthStatus {
	status := handler.HealthStatus{
		Status: "degraded",
	}
	if err := r.Ping(ctx); err == nil {
		status.Status = "healthy"
		status.ClickHouse = "connected"
	} else {
		status.ClickHouse = "disconnected"
	}
	return status
}

// RunMigrations applies SQL files in the migrations/ directory.
// For now it applies the single migrations/001_create_events.sql file.
func (r *Repository) RunMigrations(ctx context.Context) error {
	// Apply all SQL files in migrations/ in sorted order.
	return ApplyMigrations(ctx, r.conn, "migrations")
}

