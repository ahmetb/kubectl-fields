package managed

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseFieldsV1Key splits a FieldsV1 map key into its prefix and content.
// Expected prefixes are "f" (field), "k" (associative key), "v" (value),
// and "i" (index). The special key "." returns prefix "." with empty content.
// Malformed keys with no colon return empty prefix with the full key as content.
func ParseFieldsV1Key(key string) (prefix string, content string) {
	if key == "." {
		return ".", ""
	}
	idx := strings.IndexByte(key, ':')
	if idx < 0 {
		return "", key
	}
	return key[:idx], key[idx+1:]
}

// ParseAssociativeKey parses the JSON content of a k: prefix key into a map.
// For example, `{"name":"nginx"}` returns map["name"]="nginx".
func ParseAssociativeKey(jsonStr string) (map[string]any, error) {
	var result map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("parsing associative key JSON %q: %w", jsonStr, err)
	}
	return result, nil
}
