package telegram

import (
	"strconv"
	"strings"
)

// NormalizeChannelID normalizes any Telegram channel ID into a positive raw int64 MTProto channel ID.
// For example, "-1001234567890" or -1001234567890 becomes 1234567890.
func NormalizeChannelID(id int64) int64 {
	s := strconv.FormatInt(id, 10)
	if strings.HasPrefix(s, "-100") {
		parsed, err := strconv.ParseInt(s[4:], 10, 64)
		if err == nil {
			return parsed
		}
	}
	if id < 0 {
		return -id
	}
	return id
}

// ParseChannelIdentifier parses a string that might be a raw number, a -100... ID,
// or a username (@channel or channel), returning the numeric ID if numeric.
func ParseChannelIdentifier(input string) (int64, string, bool) {
	trimmed := strings.TrimSpace(input)
	trimmed = strings.TrimPrefix(trimmed, "https://t.me/")
	trimmed = strings.TrimPrefix(trimmed, "t.me/")

	// If it's a numeric ID
	if id, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return NormalizeChannelID(id), "", true
	}

	// Username
	username := strings.TrimPrefix(trimmed, "@")
	return 0, username, false
}
