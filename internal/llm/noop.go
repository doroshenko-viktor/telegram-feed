package llm

import (
	"context"
	"fmt"

	"telegram_feed_summarizer/internal/domain"
	"telegram_feed_summarizer/internal/pipeline"
)

// NoOpProvider handles operations gracefully when no LLM API key is configured.
type NoOpProvider struct{}

// NewNoOpProvider creates a fallback provider.
func NewNoOpProvider() *NoOpProvider {
	return &NoOpProvider{}
}

func (n *NoOpProvider) HasAI() bool {
	return false
}

func (n *NoOpProvider) GenerateDigest(ctx context.Context, topic string, posts []*domain.Post) (*domain.Digest, error) {
	return nil, fmt.Errorf("local AI is not configured (GEMINI_API_KEY is not set). Use conversational MCP with Claude/Cursor/Antigravity, or set GEMINI_API_KEY to enable autonomous local digests")
}

func (n *NoOpProvider) ProfileChannel(ctx context.Context, ch *domain.Channel, samplePosts []string) (string, []string, string, error) {
	summary, topics, contentType := pipeline.HeuristicProfileChannel(ch, samplePosts)
	return summary, topics, contentType, nil
}
