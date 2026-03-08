package app

import (
	"context"
	"time"

	"eventmetrics/internal/health"
)

// HealthChecker enriches the repository's connectivity check with
// app-level metadata (buffer strategy, uptime) known only at this layer.
type HealthChecker struct {
	base      *Base
	startTime time.Time
}

// NewHealthChecker returns a HealthChecker tied to base, recording startup time.
func NewHealthChecker(base *Base) *HealthChecker {
	return &HealthChecker{base: base, startTime: time.Now()}
}

// Health implements handler.HealthChecker.
func (h *HealthChecker) Health(ctx context.Context) health.HealthStatus {
	status := h.base.Repository.Health(ctx)
	status.BufferStrategy = h.base.Config.BufferStrategy
	status.UptimeSeconds = int64(time.Since(h.startTime).Seconds())
	return status
}
