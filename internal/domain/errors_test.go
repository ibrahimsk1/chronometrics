package domain

import (
	"errors"
	"testing"
	"time"
)

func TestSentinelErrors_Is(t *testing.T) {
	err := ErrPublishFailed
	if !IsPublishFailed(err) {
		t.Fatalf("IsPublishFailed should recognize ErrPublishFailed")
	}
	wrapped := errors.New("upstream: publish failed")
	// wrap using fmt.Errorf would be typical, but errors.Is should work with identical sentinel
	if IsPublishFailed(wrapped) {
		t.Fatalf("IsPublishFailed should not recognize unrelated error")
	}
}

func TestSentinelErrors_Wrapping(t *testing.T) {
	// simulate wrapping
	w := errors.New("wrapper: publish failed")
	if IsPublishFailed(w) {
		// fine only if same sentinel; ensure false here
		t.Fatalf("unexpected IsPublishFailed true for non-sentinel wrapped error")
	}
	// direct sentinel should be true
	if !IsQueryTimeout(ErrQueryTimeout) {
		t.Fatalf("IsQueryTimeout should recognize ErrQueryTimeout")
	}
}

func TestValidationError_Is(t *testing.T) {
	var r RawEvent
	err := Validate(&r, 1*time.Minute, 24*time.Hour)
	if err == nil {
		t.Fatalf("expected ValidationError")
	}
	if !IsValidationError(err) {
		t.Fatalf("IsValidationError should detect validation error")
	}
}
