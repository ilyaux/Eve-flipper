package telemetry

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

type Config struct {
	Enabled  bool
	Endpoint string
	APIKey   string
	Salt     string
	Env      string
}

type Event struct {
	EventType       string                 `json:"event_type"`
	Source          string                 `json:"source"`
	Env             string                 `json:"env"`
	Module          string                 `json:"module,omitempty"`
	UserID          string                 `json:"user_id,omitempty"`
	UserHash        string                 `json:"user_hash,omitempty"`
	SessionID       string                 `json:"session_id,omitempty"`
	CharacterID     *int64                 `json:"character_id,omitempty"`
	OccurredAt      string                 `json:"occurred_at,omitempty"`
	Path            string                 `json:"path,omitempty"`
	Method          string                 `json:"method,omitempty"`
	Status          int                    `json:"status,omitempty"`
	DurationMS      float64                `json:"duration_ms,omitempty"`
	ErrorCode       string                 `json:"error_code,omitempty"`
	IP              string                 `json:"ip,omitempty"`
	Country         string                 `json:"country,omitempty"`
	ASN             string                 `json:"asn,omitempty"`
	UserAgent       string                 `json:"user_agent,omitempty"`
	Properties      map[string]interface{} `json:"properties,omitempty"`
	Private         bool                   `json:"private,omitempty"`
	SnapshotType    string                 `json:"snapshot_type,omitempty"`
	SnapshotPayload map[string]interface{} `json:"snapshot_payload,omitempty"`
}

type Client struct {
	cfg    Config
	http   *http.Client
	events chan Event
}

func LoadConfigFromEnv() Config {
	return Config{
		Enabled:  envBool("TELEMETRY_ENABLED", false),
		Endpoint: env("TELEMETRY_ENDPOINT", "http://127.0.0.1:13371/v1/events"),
		APIKey:   strings.TrimSpace(os.Getenv("TELEMETRY_API_KEY")),
		Salt:     strings.TrimSpace(os.Getenv("TELEMETRY_SALT")),
		Env:      env("TELEMETRY_ENV", "hosted"),
	}
}

func NewFromEnv() *Client {
	return New(LoadConfigFromEnv())
}

func New(cfg Config) *Client {
	c := &Client{
		cfg:    cfg,
		http:   &http.Client{Timeout: 2 * time.Second},
		events: make(chan Event, 256),
	}
	if c.Enabled() {
		go c.run()
	}
	return c
}

func (c *Client) Enabled() bool {
	return c != nil &&
		c.cfg.Enabled &&
		strings.TrimSpace(c.cfg.Endpoint) != "" &&
		strings.TrimSpace(c.cfg.APIKey) != "" &&
		strings.TrimSpace(c.cfg.Salt) != ""
}

func (c *Client) Track(event Event) {
	if !c.Enabled() {
		return
	}
	event.Env = firstNonEmpty(event.Env, c.cfg.Env)
	event.Source = firstNonEmpty(event.Source, "backend")
	event.OccurredAt = firstNonEmpty(event.OccurredAt, time.Now().UTC().Format(time.RFC3339Nano))
	if event.UserHash == "" && event.UserID != "" {
		event.UserHash = stableHash(c.cfg.Salt, event.UserID)
		event.UserID = ""
	}
	event.Properties = sanitizeMap(event.Properties)
	event.SnapshotPayload = sanitizeMap(event.SnapshotPayload)
	select {
	case c.events <- event:
	default:
		// Drop on overload; telemetry must never slow down trading workflows.
	}
}

func (c *Client) run() {
	for event := range c.events {
		c.post(event)
	}
}

func (c *Client) post(event Event) {
	body, err := json.Marshal(map[string]interface{}{"events": []Event{event}})
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, c.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Telemetry-Key", c.cfg.APIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

func stableHash(salt, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(salt) + ":" + value))
	return hex.EncodeToString(sum[:])
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
