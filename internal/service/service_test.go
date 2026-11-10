package service

import (
	"context"
	"testing"

	"telegram_feed_summarizer/internal/config"
	"telegram_feed_summarizer/internal/domain"
	"telegram_feed_summarizer/internal/storage"
)

func setupTestRepo(t *testing.T) domain.Repository {
	ctx := context.Background()
	cfg := config.StorageConfig{Driver: "sqlite", DSN: ":memory:"}
	repo, err := storage.NewRepository(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}
	t.Cleanup(func() { repo.Close() })
	return repo
}

func TestPrivacyGuard(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	guard := NewPrivacyGuard(repo)

	// Channel 10 is MCP allowed; Channel 20 is Quarantined
	_ = repo.UpsertChannels(ctx, []*domain.Channel{
		{ID: 10, AccessHash: 1, Title: "Public Channel", MCPAllowed: true},
		{ID: 20, AccessHash: 2, Title: "Secret Channel", MCPAllowed: false},
	})

	// 1. Requesting unapproved channel 20 alone must return empty slice
	safeIDs, err := guard.FilterAllowedChannelIDs(ctx, []int64{20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(safeIDs) != 0 {
		t.Errorf("SECURITY LEAK: unapproved channel ID 20 was not filtered out! got: %v", safeIDs)
	}

	// 2. Requesting mixed channels [10, 20] must strip channel 20 and keep only channel 10
	safeIDs, err = guard.FilterAllowedChannelIDs(ctx, []int64{10, 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(safeIDs) != 1 || safeIDs[0] != 10 {
		t.Errorf("expected only channel 10, got %v", safeIDs)
	}

	// 3. Digest visibility test:
	// Digest containing only channel 10 -> visible
	digestAllowed := &domain.Digest{ChannelIDs: []int64{10}}
	visible, err := guard.IsDigestVisibleToMCP(ctx, digestAllowed)
	if err != nil || !visible {
		t.Errorf("expected digest containing allowed channel to be visible, got %v (err: %v)", visible, err)
	}

	// Digest containing mixed channels [10, 20] -> MUST NOT BE VISIBLE!
	digestMixed := &domain.Digest{ChannelIDs: []int64{10, 20}}
	visible, err = guard.IsDigestVisibleToMCP(ctx, digestMixed)
	if err != nil || visible {
		t.Errorf("SECURITY LEAK: digest containing quarantined channel 20 was reported visible to MCP!")
	}
}
