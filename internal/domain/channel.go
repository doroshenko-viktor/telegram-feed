package domain

import (
	"encoding/json"
	"time"
)

// Channel represents a Telegram channel or supergroup subscribed by the user.
type Channel struct {
	ID            int64      `json:"id"`
	AccessHash    int64      `json:"access_hash"`
	Title         string     `json:"title"`
	Username      string     `json:"username,omitempty"`
	About         string     `json:"about,omitempty"`
	IsTracked     bool       `json:"is_tracked"`     // When true, daemon/sync fetches messages
	MCPAllowed    bool       `json:"mcp_allowed"`    // Zero-trust gateway: true = exposed to external LLMs
	FilterAds     bool       `json:"filter_ads"`     // True = exclude ads from digests for this channel
	Summary       string     `json:"summary,omitempty"`
	Topics        []string   `json:"topics,omitempty"` // Extracted semantic topic tags (e.g. ["go", "kubernetes"])
	ContentType   string     `json:"content_type"`     // news, deep_dive, release, tutorial, general
	Language      string     `json:"language"`
	LastSyncMsgID int        `json:"last_sync_msg_id"`
	ProfiledAt    *time.Time `json:"profiled_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// TopicsJSON marshals topics slice into a JSON string for DB persistence.
func (c *Channel) TopicsJSON() string {
	if len(c.Topics) == 0 {
		return "[]"
	}
	bytes, err := json.Marshal(c.Topics)
	if err != nil {
		return "[]"
	}
	return string(bytes)
}

// ParseTopics parses a JSON string into the channel Topics slice.
func (c *Channel) ParseTopics(raw string) {
	if raw == "" || raw == "[]" {
		c.Topics = []string{}
		return
	}
	var topics []string
	if err := json.Unmarshal([]byte(raw), &topics); err != nil {
		c.Topics = []string{}
		return
	}
	c.Topics = topics
}
