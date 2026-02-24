package domain

import (
	"testing"
	"time"
)

func TestValidate_NilReceiver(t *testing.T) {
	var r *RawEvent = nil
	err := Validate(r, 1*time.Minute, 24*time.Hour)
	if err == nil {
		t.Fatalf("expected error for nil receiver")
	}
	if !IsValidationError(err) {
		t.Fatalf("expected ValidationError for nil receiver")
	}
}

func TestValidate_TimestampZero(t *testing.T) {
	r := RawEvent{EventName: "1", UserID: "u", Timestamp: 0}
	if err := Validate(&r, 1*time.Minute, 24*time.Hour); err == nil {
		t.Fatalf("expected validation error for zero timestamp")
	}
}

func TestValidate_Success(t *testing.T) {
	r := RawEvent{
		EventName: "ok",
		UserID:    "u",
		Timestamp: time.Now().UnixMilli(),
	}
	if err := Validate(&r, 1*time.Minute, 24*time.Hour); err != nil {
		t.Fatalf("unexpected validate error: %v", err)
	}
}

func TestNormalizeTimestamp_MSUnchanged(t *testing.T) {
	msInput := int64(1670000000000)
	out := NormalizeTimestamp(msInput)
	if out != uint64(msInput) {
		t.Fatalf("expected unchanged ms, got %d", out)
	}
}
