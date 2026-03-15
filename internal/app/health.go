package app

import (
	"context"
	"time"

	"eventmetrics/internal/health"
	"eventmetrics/internal/repository"
)

// healthChecker enriches the repository's connectivity check with
// app-level metadata (buffer strategy, uptime) known only at this layer.
type healthChecker struct {
	repo           *repository.Repository
	bufferStrategy string
	startTime      time.Time
}

func newHealthChecker(repo *repository.Repository, bufferStrategy string) *healthChecker {
	return &healthChecker{repo: repo, bufferStrategy: bufferStrategy, startTime: time.Now()}
}

// Health implements handler.HealthChecker.
func (h *healthChecker) Health(ctx context.Context) health.HealthStatus {
	status := h.repo.Health(ctx)
	status.BufferStrategy = h.bufferStrategy
	status.UptimeSeconds = int64(time.Since(h.startTime).Seconds())
	return status
}
