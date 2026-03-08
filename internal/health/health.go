package health

// HealthStatus is the JSON shape returned by health endpoint.
type HealthStatus struct {
	Status         string `json:"status"`
	BufferStrategy string `json:"buffer_strategy,omitempty"`
	UptimeSeconds  int64  `json:"uptime_seconds,omitempty"`
	ClickHouse     string `json:"clickhouse,omitempty"`
}
