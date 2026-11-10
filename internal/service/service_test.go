package service

import (
	"context"
	"strings"
	"testing"
	"time"

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

func TestGenerateDigestExtractiveFallback(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)

	_ = repo.UpsertChannels(ctx, []*domain.Channel{
		{
			ID:        101,
			Title:     "DevOps Daily",
			Username:  "devops_daily",
			IsTracked: true,
		},
	})

	now := time.Now()
	posts := []*domain.Post{
		{
			ChannelID: 101,
			MsgID:     1,
			Date:      now.Add(-2 * time.Hour),
			Text:      "Docker 26 released with rootless improvements and buildx updates.",
			Views:     1500,
			Forwards:  30,
			IsRead:    false,
		},
		{
			ChannelID: 101,
			MsgID:     2,
			Date:      now.Add(-1 * time.Hour),
			Text:      "Terraform best practices for monorepo infrastructures.",
			Views:     800,
			Forwards:  12,
			IsRead:    false,
		},
	}
	if err := repo.UpsertPosts(ctx, posts); err != nil {
		t.Fatalf("UpsertPosts failed: %v", err)
	}

	feedSvc := NewFeedService(&config.Config{}, repo, nil, nil)

	digest, err := feedSvc.GenerateDigest(ctx, "all")
	if err != nil {
		t.Fatalf("GenerateDigest extractive fallback failed: %v", err)
	}

	if digest == nil {
		t.Fatal("expected non-nil digest")
	}

	if digest.PostCount != 2 {
		t.Errorf("expected PostCount 2, got %d", digest.PostCount)
	}

	// Verify posts are now marked as read
	statsTotal, statsUnread, err := feedSvc.GetPostStats(ctx)
	if err != nil {
		t.Fatalf("GetPostStats failed: %v", err)
	}
	if statsTotal != 2 || statsUnread != 0 {
		t.Errorf("expected 2 total, 0 unread after digest; got %d total, %d unread", statsTotal, statsUnread)
	}

	// Verify digest is saved in repo
	savedDigests, err := repo.ListDigests(ctx, "all", false, 10)
	if err != nil {
		t.Fatalf("ListDigests failed: %v", err)
	}
	if len(savedDigests) != 1 {
		t.Fatalf("expected 1 saved digest, got %d", len(savedDigests))
	}
}

func TestGenerateDigestWithOptions_IndividualChannelAndPeriods(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)

	_ = repo.UpsertChannels(ctx, []*domain.Channel{
		{ID: 201, Title: "Golang Insights", Username: "golang_insights", IsTracked: true},
		{ID: 202, Title: "Rust Updates", Username: "rust_updates", IsTracked: true},
	})

	now := time.Now()
	// Insert posts across different days and channels
	posts := []*domain.Post{
		{
			ChannelID: 201,
			MsgID:     1,
			Date:      now.Add(-2 * time.Hour), // Today
			Text:      "Go 1.25 Release candidate testing started.",
			Views:     500,
			IsRead:    false,
		},
		{
			ChannelID: 201,
			MsgID:     2,
			Date:      now.Add(-3 * 24 * time.Hour), // 3 days ago (in past week)
			Text:      "Deep dive into Go scheduler and netpoll.",
			Views:     1200,
			IsRead:    false,
		},
		{
			ChannelID: 202,
			MsgID:     1,
			Date:      now.Add(-2 * time.Hour), // Today, Rust channel
			Text:      "Rust 2024 edition stabilization updates.",
			Views:     800,
			IsRead:    false,
		},
	}
	if err := repo.UpsertPosts(ctx, posts); err != nil {
		t.Fatalf("UpsertPosts failed: %v", err)
	}

	feedSvc := NewFeedService(&config.Config{}, repo, nil, nil)

	// 1. Digest for individual channel 201 only (Today)
	digestToday, err := feedSvc.GenerateDigestWithOptions(ctx, DigestOptions{
		ChannelID: 201,
		TimeRange: TimeRangeToday,
	})
	if err != nil {
		t.Fatalf("GenerateDigestWithOptions Today failed: %v", err)
	}
	if digestToday.PostCount != 1 {
		t.Errorf("expected 1 post in Today digest for channel 201, got %d", digestToday.PostCount)
	}
	if len(digestToday.ChannelIDs) != 1 || digestToday.ChannelIDs[0] != 201 {
		t.Errorf("expected channel ID 201, got %v", digestToday.ChannelIDs)
	}
	if !strings.Contains(digestToday.SummaryMarkdown, "Golang Insights") {
		t.Errorf("expected channel name 'Golang Insights' in digest summary, got: %s", digestToday.SummaryMarkdown)
	}

	// Historical digest should NOT mark posts as read
	_, unreadCount, _ := feedSvc.GetPostStats(ctx)
	if unreadCount != 3 {
		t.Errorf("expected 3 posts to remain unread after historical Today digest, got %d", unreadCount)
	}

	// 2. Digest for channel 201 over Past 7 Days (should find both posts from channel 201, excluding channel 202)
	digestWeek, err := feedSvc.GenerateDigestWithOptions(ctx, DigestOptions{
		ChannelID: 201,
		TimeRange: TimeRangePast7Days,
	})
	if err != nil {
		t.Fatalf("GenerateDigestWithOptions Past7Days failed: %v", err)
	}
	if digestWeek.PostCount != 2 {
		t.Errorf("expected 2 posts in Week digest for channel 201, got %d", digestWeek.PostCount)
	}

	// 3. Digest for ALL channels over Past 7 Days (should find all 3 posts)
	digestAllWeek, err := feedSvc.GenerateDigestWithOptions(ctx, DigestOptions{
		ChannelID: 0,
		TimeRange: TimeRangePast7Days,
	})
	if err != nil {
		t.Fatalf("GenerateDigestWithOptions All Week failed: %v", err)
	}
	if digestAllWeek.PostCount != 3 {
		t.Errorf("expected 3 posts in All Week digest, got %d", digestAllWeek.PostCount)
	}
}
