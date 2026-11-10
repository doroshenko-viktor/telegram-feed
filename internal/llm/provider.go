package llm

import (
	"context"

	"telegram_feed_summarizer/internal/domain"
)

// Provider defines the interface for local AI summarization and channel semantic profiling.
type Provider interface {
	// GenerateDigest analyzes a slice of posts and returns a structured markdown digest.
	GenerateDigest(ctx context.Context, topic string, posts []*domain.Post) (*domain.Digest, error)

	// ProfileChannel analyzes channel metadata and sample posts to extract summary and topic tags.
	ProfileChannel(ctx context.Context, ch *domain.Channel, samplePosts []string) (summary string, topics []string, contentType string, err error)

	// HasAI returns true if an actual LLM provider is active (false for NoOp fallback).
	HasAI() bool
}
