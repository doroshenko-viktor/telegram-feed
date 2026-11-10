package telegram

import "testing"

func TestNormalizeChannelID(t *testing.T) {
	tests := []struct {
		input    int64
		expected int64
	}{
		{input: 1234567890, expected: 1234567890},
		{input: -1001234567890, expected: 1234567890},
		{input: -987654321, expected: 987654321},
		{input: 0, expected: 0},
	}

	for _, tt := range tests {
		got := NormalizeChannelID(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeChannelID(%d) = %d; want %d", tt.input, got, tt.expected)
		}
	}
}

func TestParseChannelIdentifier(t *testing.T) {
	// Numeric ID with -100 prefix
	id, username, isNumeric := ParseChannelIdentifier("-1001234567890")
	if !isNumeric || id != 1234567890 || username != "" {
		t.Errorf("expected numeric 1234567890, got id=%d username=%s isNum=%v", id, username, isNumeric)
	}

	// Username with @
	id, username, isNumeric = ParseChannelIdentifier("@golang_news")
	if isNumeric || username != "golang_news" {
		t.Errorf("expected username golang_news, got id=%d username=%s isNum=%v", id, username, isNumeric)
	}

	// URL format
	id, username, isNumeric = ParseChannelIdentifier("https://t.me/golang_news")
	if isNumeric || username != "golang_news" {
		t.Errorf("expected username golang_news from URL, got id=%d username=%s isNum=%v", id, username, isNumeric)
	}
}
