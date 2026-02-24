package domain

import "testing"

func TestValidate_NilReceiver(t *testing.T) {
	var r *RawEvent = nil
	err := r.Validate()
	if err == nil {
		t.Fatalf("expected error for nil receiver")
	}
	if !IsValidationError(err) {
		t.Fatalf("expected ValidationError for nil receiver")
	}
}

func TestValidate_TimestampZero(t *testing.T) {
	r := RawEvent{ID: "1", Type: "t", Timestamp: 0}
	if err := r.Validate(); err == nil {
		t.Fatalf("expected validation error for zero timestamp")
	}
}

func TestValidateQueryParams_GroupByVariants(t *testing.T) {
	q := QueryParams{EventName: "e", From: 1, To: 2, GroupBy: ""}
	if err := ValidateQueryParams(q); err != nil {
		t.Fatalf("unexpected error for empty group_by: %v", err)
	}
	q.GroupBy = "none"
	if err := ValidateQueryParams(q); err != nil {
		t.Fatalf("unexpected error for group_by none: %v", err)
	}
}

func TestCanonicalJSON_NullAndArray(t *testing.T) {
	b, err := canonicalJSON(nil)
	if err != nil {
		t.Fatalf("expected no error for nil canonicalJSON, got %v", err)
	}
	if string(b) != "null" {
		t.Fatalf("expected 'null' for nil canonicalJSON, got %s", string(b))
	}
	arr := []interface{}{1, "a", map[string]interface{}{"k": "v"}}
	b2, err := canonicalJSON(arr)
	if err != nil {
		t.Fatalf("canonicalJSON array err: %v", err)
	}
	if len(b2) == 0 {
		t.Fatalf("expected non-empty json for array")
	}
}
