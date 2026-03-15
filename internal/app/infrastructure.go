package app

import (
	"context"
	"fmt"
	"log/slog"

	"eventmetrics/internal/buffer"
	"eventmetrics/internal/config"
	"eventmetrics/internal/observability"
	"eventmetrics/internal/repository"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
)

func connectRepository(ctx context.Context, cfg config.Config, metrics *observability.Metrics, log *slog.Logger) (*repository.Repository, error) {
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
	return repository.Connect(ctx, opts, repoCfg, metrics, log)
}

func startBuffer(ctx context.Context, cfg config.Config, repo *repository.Repository, metrics *observability.Metrics, log *slog.Logger) (*buffer.Buffer, error) {
	switch cfg.BufferStrategy {
	case "memory", "":
		buf := buffer.New(ctx, cfg.Buffer, metrics, log)
		buf.Start(ctx, repo)
		log.Info("memory buffer started", "capacity", cfg.Buffer.Capacity)
		return buf, nil
	default:
		return nil, fmt.Errorf("unsupported buffer strategy: %s", cfg.BufferStrategy)
	}
}
