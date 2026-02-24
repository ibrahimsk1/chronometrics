package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"eventmetrics/internal/buffer"
	"eventmetrics/internal/config"
	"eventmetrics/internal/ingest"
	"eventmetrics/internal/repository"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
)

// Base holds shared application resources.
type Base struct {
	Config    config.Config
	Publisher ingest.EventPublisher
	Buffer    *buffer.Buffer
	Repository *repository.Repository
}

// Setup loads configuration and returns a Base with the validated config and
// a strategy-appropriate publisher. Currently supports "memory" strategy.
func Setup(ctx context.Context) (*Base, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	base := &Base{Config: cfg}

	// Initialize ClickHouse repository before selecting strategy so flushers can be wired.
	chCfg := cfg.ClickHouse
	repoCfg := repository.Config{
		QueryTimeout:    chCfg.QueryTimeout,
		MaxOpenConns:    chCfg.MaxOpenConns,
		MaxIdleConns:    chCfg.MaxIdleConns,
		ConnMaxLifetime: chCfg.ConnMaxLifetime,
		MaxRetries:      chCfg.MaxRetries,
		RetryBaseDelay:  chCfg.RetryBaseDelay,
		RetryMaxDelay:   chCfg.RetryMaxDelay,
	}
	opts := &clickhouse.Options{
		Addr: []string{chCfg.Addr},
		Auth: clickhouse.Auth{
			Database: chCfg.Database,
			Username: chCfg.Username,
			Password: chCfg.Password,
		},
	}
	repo, err := repository.Connect(ctx, opts, repoCfg)
	if err != nil {
		return nil, fmt.Errorf("connect clickhouse: %w", err)
	}
	// Run migrations (idempotent)
	if err := repo.RunMigrations(ctx); err != nil {
		_ = repo.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	base.Repository = repo

	switch cfg.BufferStrategy {
	case "memory", "":
		// Create in-memory buffer publisher with repository as flusher.
		buf := buffer.New(ctx, cfg.Buffer)
		base.Buffer = buf
		base.Publisher = buf
		// Start flush goroutine with repository flusher.
		buf.Start(ctx, repo)
		log.Printf("memory buffer publisher created (capacity=%d)", buf)
	default:
		// Other strategies not yet implemented
		_ = repo.Close()
		return nil, fmt.Errorf("unsupported buffer strategy: %s", cfg.BufferStrategy)
	}

	// Minimal logging to surface loaded configuration (non-sensitive fields).
	log.Printf("config loaded: buffer_strategy=%s server.port=%d buffer.capacity=%d", cfg.BufferStrategy, cfg.Server.Port, cfg.Buffer.Capacity)

	// Basic sanity check: ensure publisher is present.
	if base.Publisher == nil {
		return nil, errors.New("no publisher configured")
	}

	// Allow some time for background components to initialize (best-effort).
	time.Sleep(10 * time.Millisecond)

	return base, nil
}

// Shutdown gracefully closes resources owned by Base.
func (b *Base) Shutdown(ctx context.Context) {
	// Close publisher (preferred via its Close(ctx))
	if b.Publisher != nil {
		_ = b.Publisher.Close(ctx)
	}
	// Close buffer (drain + stop)
	if b.Buffer != nil {
		_ = b.Buffer.Close(ctx)
	}
	// Close repository / DB connection
	if b.Repository != nil {
		_ = b.Repository.Close()
	}
}
