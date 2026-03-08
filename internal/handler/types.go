package handler

import (
	"context"

	"eventmetrics/internal/domain"
	"eventmetrics/internal/health"
)

// ServerConfig represents minimal server config needed by handlers.
type ServerConfig struct {
	MaxBodyBytes  int64
	MaxBulkEvents int // maximum events per bulk request; defaults to 1000
}

// Ingester is the handler-level interface that bridges to the ingest usecase.
// Aligned with TDD: accepts domain.RawEvent.
type Ingester interface {
	Ingest(ctx context.Context, e *domain.RawEvent) error
}

// MetricsQuerier executes aggregation queries. Uses domain types.
type MetricsQuerier interface {
	Query(ctx context.Context, params domain.QueryParams) (*domain.MetricResult, error)
}

// HealthChecker provides a health snapshot for the handler.
// Aligned with TDD: returns HealthStatus (no error).
type HealthChecker interface {
	Health(ctx context.Context) health.HealthStatus
}
