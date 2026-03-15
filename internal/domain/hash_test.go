package domain

import "testing"

func TestComputePayloadHash_Deterministic(t *testing.T) {
	meta := map[string]interface{}{
		"z": 1,
		"a": []interface{}{map[string]interface{}{"k": "v"}, 2},
	}
	tags1 := []string{"blue", "green", "red"}
	tags2 := []string{"red", "blue", "green"} // different order
	mb, err := canonicalJSON(meta)
	if err != nil {
		t.Fatalf("canonical json err: %v", err)
	}
	h1 := ComputePayloadHash("chan", "camp", tags1, string(mb), "")
	h2 := ComputePayloadHash("chan", "camp", tags2, string(mb), "")
	if h1 != h2 {
		t.Fatalf("expected equal hashes, got %d vs %d", h1, h2)
	}
}

func TestComputePayloadHash_MetadataCanonicalization(t *testing.T) {
	meta1 := map[string]interface{}{"a": 1, "b": map[string]interface{}{"x": 2, "y": 3}}
	meta2 := map[string]interface{}{"b": map[string]interface{}{"y": 3, "x": 2}, "a": 1}
	mb1, err := canonicalJSON(meta1)
	if err != nil {
		t.Fatalf("canonical json err: %v", err)
	}
	mb2, err := canonicalJSON(meta2)
	if err != nil {
		t.Fatalf("canonical json err: %v", err)
	}
	h1 := ComputePayloadHash("c", "d", []string{}, string(mb1), "")
	h2 := ComputePayloadHash("c", "d", []string{}, string(mb2), "")
	if h1 != h2 {
		t.Fatalf("expected equal hashes for canonicalized metadata, got %d vs %d", h1, h2)
	}
}
