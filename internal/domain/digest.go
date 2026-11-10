package domain

import (
	"encoding/json"
	"time"
)

// Digest represents a compiled summary of posts across channels for a given period.
type Digest struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	Topic           string    `json:"topic"`
	PeriodStart     time.Time `json:"period_start"`
	PeriodEnd       time.Time `json:"period_end"`
	ChannelIDs      []int64   `json:"channel_ids"`
	SummaryMarkdown string    `json:"summary_markdown"`
	PostCount       int       `json:"post_count"`
	MCPVisible      bool      `json:"mcp_visible"` // True ONLY if all channels in ChannelIDs have mcp_allowed == true
	CreatedAt       time.Time `json:"created_at"`
}

// ChannelIDsJSON marshals channel IDs into a JSON string for DB persistence.
func (d *Digest) ChannelIDsJSON() string {
	if len(d.ChannelIDs) == 0 {
		return "[]"
	}
	bytes, err := json.Marshal(d.ChannelIDs)
	if err != nil {
		return "[]"
	}
	return string(bytes)
}

// ParseChannelIDs parses a JSON string into ChannelIDs slice.
func (d *Digest) ParseChannelIDs(raw string) {
	if raw == "" || raw == "[]" {
		d.ChannelIDs = []int64{}
		return
	}
	var ids []int64
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		d.ChannelIDs = []int64{}
		return
	}
	d.ChannelIDs = ids
}
