package domain

import (
	"encoding/json"
	"sort"
	"strings"
)

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
