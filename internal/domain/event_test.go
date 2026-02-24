package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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

	if r.ID == "" {
		t.Errorf("expected id, got empty")
	}
	if r.Type == "" {
		t.Errorf("expected type, got empty")
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
	if err := r.Validate(); err == nil {
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
	ev, err := r.ToEvent()
	if err != nil {
		t.Fatalf("ToEvent failed: %v", err)
	}
	if ev.TimestampMS < 1e12 {
		t.Fatalf("expected timestamp normalized to ms, got %d", ev.TimestampMS)
	}
}

func TestValidate_InvalidTimestamp(t *testing.T) {
	r := RawEvent{
		ID:        "x",
		Type:      "t",
		Timestamp: 99999999999999999, // unreasonably large
	}
	if err := r.Validate(); err == nil {
		t.Fatalf("expected validation error for invalid timestamp")
	} else if !IsValidationError(err) {
		t.Fatalf("expected ValidationError, got: %v", err)
	}
}

func TestToEvent_PayloadHash(t *testing.T) {
	r := RawEvent{
		ID:        "1",
		Type:      "test",
		Timestamp: 1670000000,
		Data: map[string]interface{}{
			"channel":     "c1",
			"campaign_id": "camp1",
			"tags":        []interface{}{"a", "b"},
			"meta":        map[string]interface{}{"k": "v"},
		},
	}
	ev, err := r.ToEvent()
	if err != nil {
		t.Fatalf("ToEvent failed: %v", err)
	}
	if ev.PayloadHash == "" {
		t.Fatalf("expected non-empty payload hash")
	}
	// ensure metadata preserved
	if ev.Data == nil || ev.Data["meta"] == nil {
		t.Fatalf("expected metadata preserved in Event.Data")
	}
}
