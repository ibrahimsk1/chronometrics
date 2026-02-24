package domain

// QueryParams represents the validated query parameters for GET /metrics.
type QueryParams struct {
	EventName string `json:"event_name"`
	From      int64  `json:"from"`
	To        int64  `json:"to"`
	GroupBy   string `json:"group_by,omitempty"`
}

// MetricResult is a top-level metric response for an event.
type MetricResult struct {
	EventName string        `json:"event_name"`
	Groups    []GroupResult `json:"groups"`
	Total     int64         `json:"total"`
}

// GroupResult represents an aggregation bucket result.
type GroupResult struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

var allowedGroupBy = map[string]bool{
	"":         true,
	"none":     true,
	"channel":  true,
	"campaign": true,
	"tag":      true,
	"hour":     true,
	"day":      true,
}

// ValidateQueryParams returns a ValidationError when required fields are missing
// or group_by is invalid.
func ValidateQueryParams(q QueryParams) error {
	if q.EventName == "" {
		return &ValidationError{Msg: "event_name required"}
	}
	if q.From <= 0 {
		return &ValidationError{Msg: "from required and must be > 0"}
	}
	if q.To <= 0 {
		return &ValidationError{Msg: "to required and must be > 0"}
	}
	if q.To < q.From {
		return &ValidationError{Msg: "to must be >= from"}
	}
	if !allowedGroupBy[q.GroupBy] {
		return &ValidationError{Msg: "invalid group_by"}
	}
	return nil
}

