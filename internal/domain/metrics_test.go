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

