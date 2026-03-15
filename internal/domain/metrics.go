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
	"":            true,
	"channel":     true,
	"hour":        true,
	"day":         true,
	"campaign_id": true,
}

func ValidateQueryParams(p *QueryParams) error {
	var errs []error
	if strings.TrimSpace(p.EventName) == "" {
		errs = append(errs, ErrQueryEventNameMissing)
	}
	if p.From == 0 {
		errs = append(errs, ErrQueryFromMissing)
	}
	if p.To == 0 {
		errs = append(errs, ErrQueryToMissing)
	}
	if p.From > 0 && p.To > 0 && p.From >= p.To {
		errs = append(errs, ErrQueryRangeInvalid)
	}
	if !ValidGroupByValues[p.GroupBy] {
		errs = append(errs, fmt.Errorf("%q is not valid (valid: channel, hour, day): %w", p.GroupBy, ErrQueryGroupByInvalid))
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
