package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"
)

// RawEvent is the incoming JSON payload before validation and normalization.
// Field names match system_design §0.1 glossary.
type RawEvent struct {
	EventName  string                 `json:"event_name"`
	UserID     string                 `json:"user_id"`
	Timestamp  int64                  `json:"timestamp"`
	Channel    string                 `json:"channel,omitempty"`
	CampaignID string                 `json:"campaign_id,omitempty"`
	Tags       []string               `json:"tags,omitempty"`
	Priority   *string                `json:"priority,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// Event is the canonical domain entity after validation and normalization.
// Maps to system_design §4.1 Event entity.
// JSON tags are present for API mapping and persistence.
type Event struct {
	EventName   string   `json:"event_name"`
	UserID      string   `json:"user_id"`
	TimestampMs uint64   `json:"timestamp_ms"`
	PayloadHash uint64   `json:"payload_hash"`
	Channel     string   `json:"channel"`
	CampaignID  string   `json:"campaign_id"`
	Tags        []string `json:"tags"`
	Priority    *string  `json:"priority"`
	Metadata    string   `json:"metadata"`
}

// BulkRequest is the incoming JSON for POST /events/bulk.
type BulkRequest struct {
	Events []RawEvent `json:"events"`
}

var eventRules = []rule{
	validateEventName,
}

type rule func(*RawEvent, []error) []error

func validateEventName(e *RawEvent, errs []error) []error {
	if len(e.EventName) > 100 {
		errs = append(errs, fmt.Errorf("event_name exceeds 100 chars: %w", ErrEventNameMissing))
	}
	return errs
}

// Validate enforces invariants I1 (required fields) and I2 (timestamp bounds).
// See system_design §4.3.
func Validate(e *RawEvent, maxFuture, maxPast time.Duration) error {
	if e == nil {
		return &ValidationError{Errors: []error{ErrRawEventNil}}
	}
	var errs []error

	for _, rule := range eventRules {
		errs = rule(e, errs)
	}
	if strings.TrimSpace(e.EventName) == "" {
		errs = append(errs, ErrEventNameMissing)
	}
	if strings.TrimSpace(e.UserID) == "" {
		errs = append(errs, ErrUserIDMissing)
	}
	if e.Timestamp <= 0 {
		errs = append(errs, ErrTimestampNonPositive)
	} else {
		tsMs := NormalizeTimestamp(e.Timestamp)
		now := uint64(time.Now().UnixMilli())
		if tsMs > now+uint64(maxFuture.Milliseconds()) {
			errs = append(errs, fmt.Errorf("max %s: %w", maxFuture, ErrTimestampTooFuture))
		}
		if tsMs < now-uint64(maxPast.Milliseconds()) {
			errs = append(errs, fmt.Errorf("max %s: %w", maxPast, ErrTimestampTooPast))
		}
	}
	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	return nil
}

// NormalizeTimestamp converts seconds to milliseconds if needed.
// Detection: values < 10^12 are treated as seconds (see system_design §7.4).
func NormalizeTimestamp(ts int64) uint64 {
	const msThreshold = int64(1e12)
	if ts < msThreshold {
		return uint64(ts) * 1000
	}
	return uint64(ts)
}

// ComputePayloadHash computes xxHash64 over canonical payload fields.
// Canonical form: channel + campaignID + sorted(tags) + JSON(metadata).
// Null byte separators prevent field boundary ambiguity.
// See system_design §7.4, ADR-0005.
func ComputePayloadHash(channel, campaignID string, tags []string, priority, metadata string) uint64 {
	var b strings.Builder
	b.WriteString(channel)
	b.WriteByte(0)
	b.WriteString(campaignID)
	b.WriteByte(0)
	sorted := make([]string, len(tags))
	copy(sorted, tags)
	sort.Strings(sorted)
	for _, tag := range sorted {
		b.WriteString(tag)
		b.WriteByte(0)
	}
	b.WriteString(priority)
	b.WriteByte(0)
	b.WriteString(metadata)
	return xxhash.Sum64String(b.String())
}

// ToEvent converts a validated RawEvent to a domain Event.
func ToEvent(raw *RawEvent) (Event, error) {
	var metadataStr string
	if raw.Metadata != nil {
		b, err := json.Marshal(raw.Metadata)
		if err != nil {
			return Event{}, fmt.Errorf("marshal metadata: %w", err)
		}
		metadataStr = string(b)
	}
	tags := raw.Tags
	if tags == nil {
		tags = []string{}
	}
	tsMs := NormalizeTimestamp(raw.Timestamp)
	var priorityStr string
	if raw.Priority != nil {
		priorityStr = *raw.Priority
	}
	hash := ComputePayloadHash(raw.Channel, raw.CampaignID, tags, priorityStr, metadataStr)
	return Event{
		EventName:   raw.EventName,
		UserID:      raw.UserID,
		TimestampMs: tsMs,
		PayloadHash: hash,
		Channel:     raw.Channel,
		CampaignID:  raw.CampaignID,
		Tags:        tags,
		Priority:    raw.Priority,
		Metadata:    metadataStr,
	}, nil
}
