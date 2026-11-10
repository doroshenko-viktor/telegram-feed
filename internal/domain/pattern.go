package domain

import "time"

// AdPattern represents a configurable rule (keyword or regex) stored in the database to detect ads and promo noise.
type AdPattern struct {
	ID          int64     `json:"id"`
	Pattern     string    `json:"pattern"`
	IsRegex     bool      `json:"is_regex"`
	Description string    `json:"description,omitempty"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
}
