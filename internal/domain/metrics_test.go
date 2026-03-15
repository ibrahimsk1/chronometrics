package domain

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestValidateQueryParams(t *testing.T) {
	var q QueryParams
	if err := ValidateQueryParams(&q); err == nil {
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
	if err := ValidateQueryParams(&q); err == nil {
		t.Fatalf("expected validation error for invalid group_by")
	}

	q.GroupBy = "channel"
	if err := ValidateQueryParams(&q); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidateQueryParams_MoreCases(t *testing.T) {
	// missing event name
	q := QueryParams{From: 1, To: 2}
	if err := ValidateQueryParams(&q); err == nil {
		t.Fatalf("expected error for missing event_name")
	}
	// non-positive from/to
	q = QueryParams{EventName: "e", From: 0, To: 0}
	if err := ValidateQueryParams(&q); err == nil {
		t.Fatalf("expected error for non-positive from/to")
	}
	// to < from
	q = QueryParams{EventName: "e", From: 200, To: 100}
	if err := ValidateQueryParams(&q); err == nil {
		t.Fatalf("expected error when to < from")
	}
}

func TestMetricResult_JSONSerialization(t *testing.T) {
	mr := MetricResult{
		EventName:   "e1",
		From:        1000,
		To:          2000,
		GroupBy:     "channel",
		TotalCount:  42,
		UniqueCount: 7,
		Groups: []GroupResult{
			{Key: "k1", TotalCount: 30, UniqueCount: 5},
			{Key: "k2", TotalCount: 12, UniqueCount: 2},
		},
	}
	b, err := json.Marshal(mr)
	if err != nil {
		t.Fatalf("marshal MetricResult: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal MetricResult json: %v", err)
	}
	// check presence and basic types/values for top-level keys
	if _, ok := m["event_name"]; !ok {
		t.Fatalf("expected event_name in json")
	}
	var from float64
	if err := json.Unmarshal(m["from"], &from); err != nil {
		t.Fatalf("expected numeric from field, err: %v", err)
	}
	if uint64(from) != mr.From {
		t.Fatalf("unexpected from value: got %v want %v", from, mr.From)
	}
	var total float64
	if err := json.Unmarshal(m["total_count"], &total); err != nil {
		t.Fatalf("expected numeric total_count field, err: %v", err)
	}
	if uint64(total) != mr.TotalCount {
		t.Fatalf("unexpected total_count: got %v want %v", total, mr.TotalCount)
	}
	// validate groups array length and first group's total_count
	var groups []map[string]json.RawMessage
	if err := json.Unmarshal(m["groups"], &groups); err != nil {
		t.Fatalf("expected groups array, err: %v", err)
	}
	if len(groups) != len(mr.Groups) {
		t.Fatalf("unexpected groups length: got %d want %d", len(groups), len(mr.Groups))
	}
	var g0Total float64
	if err := json.Unmarshal(groups[0]["total_count"], &g0Total); err != nil {
		t.Fatalf("expected numeric total_count in group, err: %v", err)
	}
	if uint64(g0Total) != mr.Groups[0].TotalCount {
		t.Fatalf("unexpected first group total_count: got %v want %v", g0Total, mr.Groups[0].TotalCount)
	}
}

func TestValidationError_ErrorFormatting(t *testing.T) {
	ve := &ValidationError{Errors: []error{errors.New("a"), errors.New("b")}}
	if ve.Error() != "validation failed: a; b" {
		t.Fatalf("unexpected ValidationError.Error(): %s", ve.Error())
	}
}
