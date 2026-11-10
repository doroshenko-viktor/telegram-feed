package pipeline

import (
	"testing"

	"telegram_feed_summarizer/internal/domain"
)

func TestCleaner(t *testing.T) {
	patterns := []*domain.AdPattern{
		{ID: 1, Pattern: "#реклама", IsRegex: false, Enabled: true},
		{ID: 2, Pattern: "#ad", IsRegex: false, Enabled: true},
		{ID: 3, Pattern: `t\.me\/(?:joinchat|\+|addlist)\/[a-zA-Z0-9_\-]+`, IsRegex: true, Enabled: true},
		{ID: 4, Pattern: `скидк[аеи] \d+%`, IsRegex: true, Enabled: true},
		{ID: 5, Pattern: "disabled pattern", IsRegex: false, Enabled: false},
	}

	cleaner := NewCleaner(patterns)

	// Ad samples
	adPosts := []string{
		"Сегодня у нас #реклама отличного курса по Kubernetes!",
		"New sponsors for this issue: #AD Check out their new dev tool.",
		"Подписывайтесь на закрытый канал: https://t.me/joinchat/AbCdEf12345",
		"Только сегодня скидка 50% на все тарифы хостинга!",
	}

	for _, text := range adPosts {
		if !cleaner.IsAdvertisement(text) {
			t.Errorf("expected text to be detected as ad: %s", text)
		}
	}

	// Clean technical posts (Must NOT be flagged as ads)
	cleanPosts := []string{
		"Go 1.28 introduces improved escape analysis for heap allocations.",
		"Understanding PostgreSQL WAL and checkpoints: an in-depth architecture writeup.",
		"Kubernetes 1.34 release notes: what changed in client-go and controllers.",
		"This post mentions disabled pattern but it should not be filtered.",
	}

	for _, text := range cleanPosts {
		if cleaner.IsAdvertisement(text) {
			t.Errorf("false positive: clean post was flagged as ad: %s", text)
		}
	}
}
