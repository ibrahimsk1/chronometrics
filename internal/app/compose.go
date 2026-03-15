package app

import (
	"context"
	"log/slog"
	"net/http"

	"eventmetrics/internal/buffer"
	"eventmetrics/internal/config"
	"eventmetrics/internal/handler"
	"eventmetrics/internal/ingest"
	"eventmetrics/internal/observability"
	"eventmetrics/internal/repository"
)

// compose wires use cases, adapters, and the HTTP handler from explicit dependencies.
func compose(ctx context.Context, cfg config.Config, buf *buffer.Buffer, repo *repository.Repository, metrics *observability.Metrics, log *slog.Logger) http.Handler {
	uc := ingest.NewUseCase(
		buf,
		cfg.Validation.MaxFutureDuration,
		cfg.Validation.MaxPastDuration,
	)
	ing := handler.NewUseCaseAdapter(uc)

	var healthChecker handler.HealthChecker
	var querier handler.MetricsQuerier
	if repo != nil {
		healthChecker = newHealthChecker(repo, cfg.BufferStrategy)
		querier = repo
	}

	rl := handler.NewLimiterRegistry(ctx, handler.RateLimiterConfig{
		Ingest:  25000,
		Bulk:    500,
		Metrics: 100,
		Health:  60,
	})
	h := handler.New(ing, querier, healthChecker, handler.ServerConfig{MaxBodyBytes: 1 << 20}, rl, log, metrics)
	return h.Router()
}
