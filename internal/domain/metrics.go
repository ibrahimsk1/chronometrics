package domain

import (
	"fmt"
	"strings"
)

// QueryParams for GET /metrics endpoint.
// Maps to system_design §6.2 F3 query contract.
type QueryParams struct {
	EventName string
	From      uint64
	To        uint64
	GroupBy   string
}

var ValidGroupByValues = map[string]bool{
	"":        true,
	"channel": true,
	"hour":    true,
	"day":     true,
}

func ValidateQueryParams(p *QueryParams) error {
	var errs []string
	if strings.TrimSpace(p.EventName) == "" {
		errs = append(errs, "event_name is required")
	}
	if p.From == 0 {
		errs = append(errs, "from is required")
	}
	if p.To == 0 {
		errs = append(errs, "to is required")
	}
	if p.From > 0 && p.To > 0 && p.From >= p.To {
		errs = append(errs, "from must be before to")
	}
	if !ValidGroupByValues[p.GroupBy] {
		errs = append(errs, fmt.Sprintf("invalid group_by: %q (valid: channel, hour, day)", p.GroupBy))
	}
	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	return nil
}

// MetricResult is the response shape for GET /metrics.
// Maps to system_design §4.1 Metric Result (ephemeral entity).
type MetricResult struct {
	EventName   string        `json:"event_name"`
	From        uint64        `json:"from"`
	To          uint64        `json:"to"`
	GroupBy     string        `json:"group_by,omitempty"`
	TotalCount  uint64        `json:"total_count"`
	UniqueCount uint64        `json:"unique_count"`
	Groups      []GroupResult `json:"groups,omitempty"`
}

type GroupResult struct {
	Key         string `json:"key"`
	TotalCount  uint64 `json:"total_count"`
	UniqueCount uint64 `json:"unique_count"`
}
