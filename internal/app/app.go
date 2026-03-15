package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"eventmetrics/internal/buffer"
	"eventmetrics/internal/config"
	"eventmetrics/internal/observability"
	"eventmetrics/internal/repository"
)

// App is the composition root. It owns every long-lived component and
// encodes their start and shutdown order.
type App struct {
	cfg     config.Config
	log     *slog.Logger
	metrics *observability.Metrics
	repo    *repository.Repository
	buffer  *buffer.Buffer
	srv     *http.Server
}

// Build constructs the dependency graph in explicit order:
// config → metrics → repo → migrations → buffer → handler → server
func Build(ctx context.Context, log *slog.Logger) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	metrics := observability.New()

	repo, err := connectRepository(ctx, cfg, metrics, log)
	if err != nil {
		return nil, fmt.Errorf("connect clickhouse: %w", err)
	}

	if err := repo.RunMigrations(ctx); err != nil {
		_ = repo.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	buf, err := startBuffer(ctx, cfg, repo, metrics, log)
	if err != nil {
		_ = repo.Close()
		return nil, err
	}

	log.Info("config loaded",
		"buffer_strategy", cfg.BufferStrategy,
		"server.port", cfg.Server.Port,
		"buffer.capacity", cfg.Buffer.Capacity,
	)

	h := compose(ctx, cfg, buf, repo, metrics, log)
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: h,
	}

	return &App{
		cfg:     cfg,
		log:     log,
		metrics: metrics,
		repo:    repo,
		buffer:  buf,
		srv:     srv,
	}, nil
}

// Run starts the HTTP server and blocks until ctx is cancelled.
func (a *App) Run(ctx context.Context) {
	a.log.Info("starting ingestor", "addr", a.srv.Addr, "buffer_strategy", a.cfg.BufferStrategy)
	go func() {
		if err := a.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.log.Error("server error", "error", err)
		}
	}()

	<-ctx.Done()
	a.shutdown()
}

// shutdown stops components in reverse-start order so no data is lost:
// 1. stop accepting requests
// 2. drain buffer (flush remaining events to DB)
// 3. close DB connection
func (a *App) shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	a.log.Info("shutting down ingestor")
	_ = a.srv.Shutdown(ctx)
	_ = a.buffer.Close(ctx)
	_ = a.repo.Close()
	a.log.Info("ingestor shutdown complete")
}
