package handler

import (
	"context"

	"eventmetrics/internal/domain"
)

// ServerConfig represents minimal server config needed by handlers.
type ServerConfig struct {
	MaxBodyBytes int64
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

// HealthStatus is the JSON shape returned by health endpoint.
type HealthStatus struct {
	Status         string `json:"status"`
	BufferStrategy string `json:"buffer_strategy,omitempty"`
	UptimeSeconds  int64  `json:"uptime_seconds,omitempty"`
	ClickHouse     string `json:"clickhouse,omitempty"`
	NATS           string `json:"nats,omitempty"`
}

// HealthChecker provides a health snapshot for the handler.
// Aligned with TDD: returns HealthStatus (no error).
type HealthChecker interface {
	Health(ctx context.Context) HealthStatus
}
