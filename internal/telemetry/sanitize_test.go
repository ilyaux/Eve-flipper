package telemetry

import "testing"

func TestClientEventAllowlist(t *testing.T) {
	if !ClientEventAllowed("feature_opened") {
		t.Fatal("feature_opened must be accepted")
	}
	if ClientEventAllowed("raw_wallet_dump") {
		t.Fatal("raw_wallet_dump must not be accepted from browser telemetry")
	}
}

func TestSanitizeMapRedactsTokens(t *testing.T) {
	got := sanitizeMap(map[string]interface{}{
		"feature":       "station",
		"access_token":  "abc",
		"refresh_token": "def",
		"nested": map[string]interface{}{
			"client_secret": "ghi",
			"safe":          "ok",
		},
	})
	if got["access_token"] != "[redacted]" || got["refresh_token"] != "[redacted]" {
		t.Fatalf("tokens were not redacted: %#v", got)
	}
	nested := got["nested"].(map[string]interface{})
	if nested["client_secret"] != "[redacted]" || nested["safe"] != "ok" {
		t.Fatalf("nested redaction mismatch: %#v", nested)
	}
}
