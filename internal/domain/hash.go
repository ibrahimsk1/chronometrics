package domain

import (
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/cespare/xxhash/v2"
)

// ComputePayloadHash computes a deterministic hash for payload components.
// It sorts tags, canonicalizes metadata to deterministic JSON, concatenates
// the parts and returns a hex-encoded xxHash64.
func ComputePayloadHash(channel, campaignID string, tags []string, metadata map[string]interface{}) (string, error) {
	// sort tags deterministically
	sortedTags := append([]string{}, tags...)
	sort.Strings(sortedTags)

	// canonicalize metadata
	metaJSON, err := canonicalJSON(metadata)
	if err != nil {
		return "", err
	}

	// build payload string
	parts := []string{channel, campaignID, strings.Join(sortedTags, ","), string(metaJSON)}
	payload := strings.Join(parts, "|")

	h := xxhash.Sum64String(payload)
	b := make([]byte, 8)
	// Convert uint64 to bytes big-endian
	for i := 0; i < 8; i++ {
		b[7-i] = byte(h >> (uint(i) * 8))
	}
	return hex.EncodeToString(b), nil
}

// canonicalJSON returns deterministic JSON for interface{} values composed of
// maps and slices. Maps are serialized with keys in sorted order.
func canonicalJSON(v interface{}) ([]byte, error) {
	switch t := v.(type) {
	case nil:
		return []byte("null"), nil
	case map[string]interface{}:
		// sort keys
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			valBytes, err := canonicalJSON(t[k])
			if err != nil {
				return nil, err
			}
			// marshal key and raw value bytes (value already JSON)
			keyb, _ := json.Marshal(k)
			parts = append(parts, string(keyb)+":"+string(valBytes))
		}
		return []byte("{" + strings.Join(parts, ",") + "}"), nil
	case []interface{}:
		elems := make([]string, 0, len(t))
		for _, e := range t {
			b, err := canonicalJSON(e)
			if err != nil {
				return nil, err
			}
			elems = append(elems, string(b))
		}
		return []byte("[" + strings.Join(elems, ",") + "]"), nil
	default:
		// primitive: use standard json.Marshal
		return json.Marshal(t)
	}
}
