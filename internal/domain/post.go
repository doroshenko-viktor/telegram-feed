package domain

import "time"

// Post represents an individual message published in a Telegram channel.
type Post struct {
	ID        int64     `json:"id"`
	ChannelID int64     `json:"channel_id"`
	MsgID     int       `json:"msg_id"`
	Date      time.Time `json:"date"`
	Text      string    `json:"text"`
	Views     int       `json:"views"`
	Forwards  int       `json:"forwards"`
	HasMedia  bool      `json:"has_media"`
	IsAd      bool      `json:"is_ad"`
	IsRead    bool      `json:"is_read"`
	CreatedAt time.Time `json:"created_at"`
}

// PostFilter specifies query parameters for retrieving messages from the database.
type PostFilter struct {
	ChannelIDs []int64
	Topic      string
	Since      time.Time
	Limit      int
	FilterAds  bool // If true, only posts with IsAd == false are returned
	UnreadOnly bool
	MCPOnly    bool // If true, only returns posts from channels where mcp_allowed == true
}
