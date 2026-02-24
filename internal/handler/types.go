package handler

import "context"

// ServerConfig represents minimal server config needed by handlers.
type ServerConfig struct {
	MaxBodyBytes int64
}

// RawEvent is a minimal payload shape for handler-level parsing used by tests.
type RawEvent struct {
	EventName  string                 `json:"event_name"`
	UserID     string                 `json:"user_id"`
	Timestamp  int64                  `json:"timestamp"`
	Channel    string                 `json:"channel,omitempty"`
	CampaignID string                 `json:"campaign_id,omitempty"`
	Tags       []string               `json:"tags,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// Ingester is the handler-level interface that bridges to the ingest usecase.
type Ingester interface {
	Ingest(ctx context.Context, e *RawEvent) error
}

// MetricsQuerier is a minimal query interface used by the handler.
type MetricsQuerier interface {
	Query(ctx context.Context, params map[string][]string) (interface{}, error)
}

// HealthChecker provides a health snapshot for the handler.
type HealthChecker interface {
	Health(ctx context.Context) (interface{}, error)
}
