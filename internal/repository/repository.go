package repository

import (
	"context"
	"fmt"
	"time"

	"eventmetrics/internal/health"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
)

type Config struct {
	QueryTimeout time.Duration
	// Connection pool
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	// Retry backoff applied during Connect
	MaxRetries     int
	RetryBaseDelay time.Duration
	RetryMaxDelay  time.Duration
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

// Connect opens a pooled ClickHouse connection and verifies it with an
// exponential-backoff ping, retrying up to cfg.MaxRetries times.
// Pool settings (MaxOpenConns, MaxIdleConns, ConnMaxLifetime) are applied
// to opts before opening so callers only need to set Addr and Auth.
func Connect(ctx context.Context, opts *clickhouse.Options, cfg Config) (*Repository, error) {
	// Apply pool defaults.
	if cfg.MaxOpenConns <= 0 {
		cfg.MaxOpenConns = 10
	}
	if cfg.MaxIdleConns <= 0 {
		cfg.MaxIdleConns = 5
	}
	if cfg.ConnMaxLifetime <= 0 {
		cfg.ConnMaxLifetime = time.Hour
	}
	opts.MaxOpenConns = cfg.MaxOpenConns
	opts.MaxIdleConns = cfg.MaxIdleConns
	opts.ConnMaxLifetime = cfg.ConnMaxLifetime

	// Apply retry defaults.
	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	baseDelay := cfg.RetryBaseDelay
	if baseDelay <= 0 {
		baseDelay = 500 * time.Millisecond
	}
	maxDelay := cfg.RetryMaxDelay
	if maxDelay <= 0 {
		maxDelay = 10 * time.Second
	}

	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open clickhouse: %w", err)
	}

	var pingErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		pingErr = conn.Ping(pingCtx)
		cancel()
		if pingErr == nil {
			break
		}
		if attempt < maxRetries {
			delay := retryDelay(attempt, baseDelay, maxDelay)
			select {
			case <-ctx.Done():
				_ = conn.Close()
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	if pingErr != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("clickhouse ping after %d retries: %w", maxRetries, pingErr)
	}

	return New(conn, cfg), nil
}

// retryDelay returns the delay for the given attempt using exponential backoff
// capped at max: base * 2^attempt.
func retryDelay(attempt int, base, max time.Duration) time.Duration {
	d := base
	for i := 0; i < attempt; i++ {
		d *= 2
		if d > max {
			return max
		}
	}
	return d
}

// Ping checks ClickHouse connectivity.
func (r *Repository) Ping(ctx context.Context) error {
	return r.conn.Ping(ctx)
}

// Health returns a domain.HealthStatus reflecting ClickHouse connectivity.
func (r *Repository) Health(ctx context.Context) health.HealthStatus {
	status := health.HealthStatus{
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
