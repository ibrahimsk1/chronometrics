package domain

import (
	"errors"
	"fmt"
)

// RawEvent represents the incoming event payload as sent by the API.
type RawEvent struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Timestamp int64                  `json:"timestamp"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// Event is the canonical domain representation after normalization.
type Event struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	TimestampMS int64                  `json:"timestamp_ms"`
	Data        map[string]interface{} `json:"data,omitempty"`
	PayloadHash string                 `json:"payload_hash,omitempty"`
}

// BulkRequest represents a bulk ingest request containing multiple RawEvent entries.
type BulkRequest struct {
	Events []RawEvent `json:"events"`
}

// ValidationError indicates input failed domain validation.
type ValidationError struct {
	Msg string
}

func (v *ValidationError) Error() string { return fmt.Sprintf("validation: %s", v.Msg) }

// IsValidationError returns true when err is a ValidationError.
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}

// Validate ensures required fields are present and within expected bounds.
func (r *RawEvent) Validate() error {
	if r == nil {
		return &ValidationError{Msg: "raw event is nil"}
	}
	if r.ID == "" {
		return &ValidationError{Msg: "id required"}
	}
	if r.Type == "" {
		return &ValidationError{Msg: "type required"}
	}
	if r.Timestamp <= 0 {
		return &ValidationError{Msg: "timestamp required and must be > 0"}
	}
	// Basic sanity bound: not unreasonably large (milliseconds epoch ~1e12..1e14)
	if r.Timestamp > 1e16 {
		return &ValidationError{Msg: "timestamp out of bounds"}
	}
	return nil
}

// NormalizeTimestamp converts seconds to milliseconds when needed.
// Heuristic: if timestamp < 1e12 treat as seconds and convert to milliseconds.
func NormalizeTimestamp(ts int64) int64 {
	if ts < 1e12 {
		return ts * 1000
	}
	return ts
}

// ToEvent converts RawEvent to canonical Event applying normalization.
func (r *RawEvent) ToEvent() (Event, error) {
	if err := r.Validate(); err != nil {
		return Event{}, err
	}
	ev := Event{
		ID:          r.ID,
		Type:        r.Type,
		TimestampMS: NormalizeTimestamp(r.Timestamp),
		Data:        r.Data,
	}

	// extract optional metadata for hash inputs
	channel := ""
	campaignID := ""
	var tags []string
	if r.Data != nil {
		if v, ok := r.Data["channel"].(string); ok {
			channel = v
		}
		if v, ok := r.Data["campaign_id"].(string); ok {
			campaignID = v
		}
		if v, ok := r.Data["tags"]; ok {
			switch tv := v.(type) {
			case []interface{}:
				for _, e := range tv {
					if s, ok := e.(string); ok {
						tags = append(tags, s)
					}
				}
			case []string:
				tags = append(tags, tv...)
			}
		}
	}

	ph, err := ComputePayloadHash(channel, campaignID, tags, r.Data)
	if err != nil {
		return Event{}, err
	}
	ev.PayloadHash = ph
	return ev, nil
}
