package domain

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestValidate_Success(t *testing.T) {
	r := RawEvent{
		ID:        "ok",
		Type:      "t",
		Timestamp: 1670000000000, // already ms
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("unexpected validate error: %v", err)
	}
}

func TestNormalizeTimestamp_MSUnchanged(t *testing.T) {
	ms := int64(1670000000000)
	out := NormalizeTimestamp(ms)
	if out != ms {
		t.Fatalf("expected unchanged ms, got %d", out)
	}
}

func TestToEvent_TagsStringSlice(t *testing.T) {
	r := RawEvent{
		ID:        "1",
		Type:      "t",
		Timestamp: 1670000000,
		Data: map[string]interface{}{
			"tags": []string{"x", "y"},
		},
	}
	ev, err := r.ToEvent()
	if err != nil {
		t.Fatalf("ToEvent failed: %v", err)
	}
	if ev.PayloadHash == "" {
		t.Fatalf("expected payload hash for string-slice tags")
	}
}

func TestCanonicalJSON_OrderAndTypes(t *testing.T) {
	in := map[string]interface{}{
		"b": 2,
		"a": []interface{}{map[string]interface{}{"z": 3, "y": 4}, 1},
	}
	b, err := canonicalJSON(in)
	if err != nil {
		t.Fatalf("canonicalJSON err: %v", err)
	}
	// Expect keys in order "a","b"
	if len(b) == 0 {
		t.Fatalf("empty canonical json")
	}
	if !strings.HasPrefix(string(b), "{\"a\"") {
		t.Fatalf("expected canonical json to start with {\"a\", got: %s", string(b))
	}
	// re-parse using standard marshal to ensure valid JSON structure via canonicalJSON
	var parsed interface{}
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("canonicalJSON produced invalid json: %v", err)
	}
	// parsed should be a map with same keys
	if m, ok := parsed.(map[string]interface{}); ok {
		if !reflect.DeepEqual(len(m), 2) && len(m) != 2 {
			t.Fatalf("expected map with 2 keys")
		}
	} else {
		t.Fatalf("expected map result from canonicalJSON")
	}
}

func TestValidateQueryParams_ToLessThanFrom(t *testing.T) {
	q := QueryParams{
		EventName: "e",
		From:      2000,
		To:        1000,
	}
	if err := ValidateQueryParams(q); err == nil {
		t.Fatalf("expected error when to < from")
	}
}

