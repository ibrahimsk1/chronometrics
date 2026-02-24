package domain

import "testing"

func TestValidateQueryParams(t *testing.T) {
	var q QueryParams
	if err := ValidateQueryParams(q); err == nil {
		t.Fatalf("expected validation error for missing fields")
	} else if !IsValidationError(err) {
		t.Fatalf("expected ValidationError, got: %v", err)
	}

	q = QueryParams{
		EventName: "e",
		From:      1000,
		To:        2000,
		GroupBy:   "invalid",
	}
	if err := ValidateQueryParams(q); err == nil {
		t.Fatalf("expected validation error for invalid group_by")
	}

	q.GroupBy = "channel"
	if err := ValidateQueryParams(q); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidateQueryParams_MoreCases(t *testing.T) {
	// missing event name
	q := QueryParams{From: 1, To: 2}
	if err := ValidateQueryParams(q); err == nil {
		t.Fatalf("expected error for missing event_name")
	}
	// non-positive from/to
	q = QueryParams{EventName: "e", From: 0, To: 0}
	if err := ValidateQueryParams(q); err == nil {
		t.Fatalf("expected error for non-positive from/to")
	}
	// to < from
	q = QueryParams{EventName: "e", From: 200, To: 100}
	if err := ValidateQueryParams(q); err == nil {
		t.Fatalf("expected error when to < from")
	}
}
