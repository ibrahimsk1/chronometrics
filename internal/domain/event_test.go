package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRawEvent_Unmarshal(t *testing.T) {
	path := filepath.Join("..", "..", "tests", "contract", "examples", "post_events.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read example file: %v", err)
	}

	var r RawEvent
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatalf("unmarshal RawEvent: %v", err)
	}

	if r.EventName == "" {
		t.Errorf("expected event_name, got empty")
	}
	if r.UserID == "" {
		t.Errorf("expected user_id, got empty")
	}
	if r.Timestamp == 0 {
		t.Errorf("expected timestamp, got 0")
	}
}

func TestBulkRequest_Unmarshal(t *testing.T) {
	path := filepath.Join("..", "..", "tests", "contract", "examples", "post_events_bulk.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read example file: %v", err)
	}

	var br BulkRequest
	if err := json.Unmarshal(b, &br); err != nil {
		t.Fatalf("unmarshal BulkRequest: %v", err)
	}
	if len(br.Events) == 0 {
		t.Fatalf("expected events, got 0")
	}
}

func TestValidate_MissingFields(t *testing.T) {
	var r RawEvent
	if err := Validate(&r, 1*time.Minute, 24*time.Hour); err == nil {
		t.Fatalf("expected validation error for empty raw event")
	} else if !IsValidationError(err) {
		t.Fatalf("expected ValidationError, got: %v", err)
	}
}

func TestNormalizeTimestamp_SecondsToMS(t *testing.T) {
	secs := int64(1670000000) // seconds
	ms := NormalizeTimestamp(secs)
	if ms != 1670000000000 {
		t.Fatalf("expected 1670000000000, got %d", ms)
	}
}

func TestToEvent_Normalization(t *testing.T) {
	path := filepath.Join("..", "..", "tests", "contract", "examples", "post_events.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read example file: %v", err)
	}
	var r RawEvent
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatalf("unmarshal RawEvent: %v", err)
	}
	ev, err := ToEvent(&r)
	if err != nil {
		t.Fatalf("ToEvent failed: %v", err)
	}
	if ev.TimestampMs < 1e12 {
		t.Fatalf("expected timestamp normalized to ms, got %d", ev.TimestampMs)
	}
}

func TestValidate_InvalidTimestamp(t *testing.T) {
	r := RawEvent{
		EventName: "x",
		UserID:    "u",
		Timestamp: 99999999999999999, // unreasonably large
	}
	if err := Validate(&r, 1*time.Minute, 24*time.Hour); err == nil {
		t.Fatalf("expected validation error for invalid timestamp")
	} else if !IsValidationError(err) {
		t.Fatalf("expected ValidationError, got: %v", err)
	}
}

func TestToEvent_PayloadHash(t *testing.T) {
	r := RawEvent{
		EventName:  "e1",
		UserID:     "u1",
		Timestamp:  1670000000,
		Channel:    "c1",
		CampaignID: "camp1",
		Tags:       []string{"a", "b"},
		Metadata:   map[string]interface{}{"k": "v"},
	}
	ev, err := ToEvent(&r)
	if err != nil {
		t.Fatalf("ToEvent failed: %v", err)
	}
	if ev.PayloadHash == 0 {
		t.Fatalf("expected non-zero payload hash")
	}
	// ensure metadata preserved
	if ev.Metadata == "" {
		t.Fatalf("expected metadata serialized in Event.Metadata")
	}
}
