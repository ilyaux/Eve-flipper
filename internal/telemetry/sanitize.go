package telemetry

import (
	"fmt"
	"sort"
	"strings"
)

var allowedClientEvents = map[string]bool{
	"active_session":      true,
	"feature_opened":      true,
	"scan_started":        true,
	"scan_finished":       true,
	"auth_clicked":        true,
	"vault_setup_clicked": true,
}

var forbiddenKeyFragments = []string{
	"access_token",
	"refresh_token",
	"authorization",
	"bearer",
	"cookie",
	"password",
	"passwd",
	"secret",
	"session_token",
	"sso_token",
	"client_secret",
}

func ClientEventAllowed(eventType string) bool {
	return allowedClientEvents[strings.TrimSpace(eventType)]
}

func sanitizeMap(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return map[string]interface{}{}
	}
	cleaned, _ := sanitizeValue(input, 0)
	out, ok := cleaned.(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	return out
}

func sanitizeValue(value interface{}, depth int) (interface{}, bool) {
	if depth > 6 {
		return "[redacted]", true
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := map[string]interface{}{}
		for i, key := range keys {
			if i >= 120 || forbiddenKey(key) {
				out[key] = "[redacted]"
				continue
			}
			child, _ := sanitizeValue(typed[key], depth+1)
			out[key] = child
		}
		return out, false
	case []interface{}:
		limit := len(typed)
		if limit > 120 {
			limit = 120
		}
		out := make([]interface{}, 0, limit)
		for i := 0; i < limit; i++ {
			child, _ := sanitizeValue(typed[i], depth+1)
			out = append(out, child)
		}
		return out, len(typed) > limit
	case string:
		if len(typed) > 2048 {
			return typed[:2048] + "[truncated]", true
		}
		return typed, false
	case fmt.Stringer:
		return typed.String(), false
	default:
		return typed, false
	}
}

func forbiddenKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	for _, fragment := range forbiddenKeyFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}
