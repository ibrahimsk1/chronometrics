package domain

import "testing"

func TestComputePayloadHash_Deterministic(t *testing.T) {
	meta := map[string]interface{}{
		"z": 1,
		"a": []interface{}{map[string]interface{}{"k": "v"}, 2},
	}
	tags1 := []string{"blue", "green", "red"}
	tags2 := []string{"red", "blue", "green"} // different order

	h1, err := ComputePayloadHash("chan", "camp", tags1, meta)
	if err != nil {
		t.Fatalf("hash1 err: %v", err)
	}
	h2, err := ComputePayloadHash("chan", "camp", tags2, meta)
	if err != nil {
		t.Fatalf("hash2 err: %v", err)
	}
	if h1 != h2 {
		t.Fatalf("expected equal hashes, got %s vs %s", h1, h2)
	}
}

func TestComputePayloadHash_MetadataCanonicalization(t *testing.T) {
	meta1 := map[string]interface{}{"a": 1, "b": map[string]interface{}{"x": 2, "y": 3}}
	meta2 := map[string]interface{}{"b": map[string]interface{}{"y": 3, "x": 2}, "a": 1}
	h1, err := ComputePayloadHash("c", "d", []string{}, meta1)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	h2, err := ComputePayloadHash("c", "d", []string{}, meta2)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if h1 != h2 {
		t.Fatalf("expected equal hashes for canonicalized metadata, got %s vs %s", h1, h2)
	}
}

