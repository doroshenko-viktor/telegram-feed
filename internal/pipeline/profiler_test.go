package pipeline

import (
	"slices"
	"testing"

	"telegram_feed_summarizer/internal/domain"
)

func TestHeuristicProfileChannel(t *testing.T) {
	ch := &domain.Channel{
		Title: "Golang & Cloud Architecture",
		About: "All about Go programming, Docker, and Kubernetes controllers",
	}
	sample := []string{
		"Postgres performance tips for Go backend developers",
	}

	summary, topics, contentType := HeuristicProfileChannel(ch, sample)

	if summary == "" {
		t.Errorf("expected summary, got empty")
	}
	if contentType != "news" {
		t.Errorf("expected news content type, got %s", contentType)
	}

	// Verify extracted topics
	for _, expected := range []string{"go", "kubernetes", "devops", "databases"} {
		if !slices.Contains(topics, expected) {
			t.Errorf("expected topic %s in %v", expected, topics)
		}
	}
}
