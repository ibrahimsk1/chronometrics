package domain

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

var samplePostEvent = `{
  "event_name": "sample_event",
  "user_id": "user_123",
  "timestamp": 1670000000,
  "channel": "web",
  "campaign_id": "camp1",
  "tags": ["t1","t2"],
  "metadata": {"k":"v"}
}`

var samplePostBulk = `{
  "events": [
    {
      "event_name": "bulk_event_1",
      "user_id": "u1",
      "timestamp": 1670000000
    },
    {
      "event_name": "bulk_event_2",
      "user_id": "u2",
      "timestamp": 1670000001
    }
  ]
}`

func mustSampleRawEvent(t *testing.T) RawEvent {
	t.Helper()

	var r RawEvent
	if err := json.Unmarshal([]byte(samplePostEvent), &r); err != nil {
		t.Fatalf("unmarshal samplePostEvent: %v", err)
	}
	return r
}

func TestRawEvent_Unmarshal(t *testing.T) {
	r := mustSampleRawEvent(t)

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
	var br BulkRequest
	if err := json.Unmarshal([]byte(samplePostBulk), &br); err != nil {
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

func TestValidateTable_Fields(t *testing.T) {
	base := mustSampleRawEvent(t)
	base.Timestamp = time.Now().Unix()

	cases := []struct {
		name    string
		mutate  func(*RawEvent)
		wantErr error
	}{
		{
			name:    "missing event_name",
			mutate:  func(r *RawEvent) { r.EventName = "" },
			wantErr: ErrEventNameMissing,
		},
		{
			name:    "missing user_id",
			mutate:  func(r *RawEvent) { r.UserID = "" },
			wantErr: ErrUserIDMissing,
		},
		{
			name:    "non-positive timestamp",
			mutate:  func(r *RawEvent) { r.Timestamp = 0 },
			wantErr: ErrTimestampNonPositive,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			tc.mutate(&r)
			err := Validate(&r, 1*time.Minute, 24*time.Hour)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestNormalizeTimestamp_SecondsToMS(t *testing.T) {
	secs := int64(1670000000) // seconds
	ms := NormalizeTimestamp(secs)
	if ms != 1670000000000 {
		t.Fatalf("expected 1670000000000, got %d", ms)
	}
}

func TestValidate_TimezoneCheck(t *testing.T) {
	r := mustSampleRawEvent(t)

	r.Timestamp = time.Now().Add(time.Hour * -24).UnixMilli()
	if err := Validate(&r, time.Duration(time.Hour*1), time.Duration(time.Hour*10)); err == nil {
		t.Fatalf("Time validation error is expected")
	}
}

func TestToEvent_Normalization(t *testing.T) {
	var r RawEvent
	if err := json.Unmarshal([]byte(samplePostEvent), &r); err != nil {
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
