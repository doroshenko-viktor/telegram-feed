package storage

import (
	"context"
	"testing"
	"time"

	"telegram_feed_summarizer/internal/config"
	"telegram_feed_summarizer/internal/domain"
)

func setupTestDB(t *testing.T) domain.Repository {
	ctx := context.Background()
	cfg := config.StorageConfig{
		Driver: "sqlite",
		DSN:    ":memory:",
	}
	repo, err := NewRepository(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to initialize test repository: %v", err)
	}
	t.Cleanup(func() {
		repo.Close()
	})
	return repo
}

func TestChannelsAndMCPAllowlist(t *testing.T) {
	ctx := context.Background()
	repo := setupTestDB(t)

	channels := []*domain.Channel{
		{
			ID:         1001,
			AccessHash: 12345,
			Title:      "Go Tech News",
			Username:   "golang_news",
			IsTracked:  true,
			MCPAllowed: true,
			FilterAds:  true,
			Topics:     []string{"go", "backend"},
		},
		{
			ID:         1002,
			AccessHash: 67890,
			Title:      "Private Personal Channel",
			Username:   "",
			IsTracked:  true,
			MCPAllowed: false, // Quarantined from MCP
			FilterAds:  false,
			Topics:     []string{"personal"},
		},
	}

	if err := repo.UpsertChannels(ctx, channels); err != nil {
		t.Fatalf("UpsertChannels failed: %v", err)
	}

	// Test ListChannels(all)
	all, err := repo.ListChannels(ctx, false)
	if err != nil {
		t.Fatalf("ListChannels failed: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 channels, got %d", len(all))
	}

	// Test ListMCPAllowedChannels (Zero-Trust Privacy Gateway)
	mcpAllowed, err := repo.ListMCPAllowedChannels(ctx)
	if err != nil {
		t.Fatalf("ListMCPAllowedChannels failed: %v", err)
	}
	if len(mcpAllowed) != 1 {
		t.Fatalf("expected 1 MCP allowed channel, got %d", len(mcpAllowed))
	}
	if mcpAllowed[0].ID != 1001 {
		t.Errorf("expected allowed channel ID 1001, got %d", mcpAllowed[0].ID)
	}

	// Test UpdateWatermark
	if err := repo.UpdateWatermark(ctx, 1001, 50); err != nil {
		t.Fatalf("UpdateWatermark failed: %v", err)
	}
	ch, err := repo.GetChannel(ctx, 1001)
	if err != nil {
		t.Fatalf("GetChannel failed: %v", err)
	}
	if ch.LastSyncMsgID != 50 {
		t.Errorf("expected last sync msg id 50, got %d", ch.LastSyncMsgID)
	}
}

func TestPostsAndAdFiltering(t *testing.T) {
	ctx := context.Background()
	repo := setupTestDB(t)

	// Create 1 public channel and 1 quarantined channel
	channels := []*domain.Channel{
		{
			ID:         2001,
			AccessHash: 111,
			Title:      "Public Go Channel",
			MCPAllowed: true,
			IsTracked:  true,
			FilterAds:  true,
			Topics:     []string{"go"},
		},
		{
			ID:         2002,
			AccessHash: 222,
			Title:      "Secret Channel",
			MCPAllowed: false,
			IsTracked:  true,
			FilterAds:  false,
			Topics:     []string{"secret"},
		},
	}
	if err := repo.UpsertChannels(ctx, channels); err != nil {
		t.Fatalf("failed to insert channels: %v", err)
	}

	now := time.Now()
	posts := []*domain.Post{
		{
			ChannelID: 2001,
			MsgID:     1,
			Date:      now.Add(-2 * time.Hour),
			Text:      "Go 1.28 is released with great features!",
			Views:     150,
			IsAd:      false,
			IsRead:    false,
		},
		{
			ChannelID: 2001,
			MsgID:     2,
			Date:      now.Add(-1 * time.Hour),
			Text:      "#реклама Buy crypto signals at 50% discount! t.me/joinchat/xyz",
			Views:     80,
			IsAd:      true, // Marked as advertisement
			IsRead:    false,
		},
		{
			ChannelID: 2002,
			MsgID:     10,
			Date:      now.Add(-30 * time.Minute),
			Text:      "Secret company discussion",
			Views:     5,
			IsAd:      false,
			IsRead:    false,
		},
	}

	if err := repo.UpsertPosts(ctx, posts); err != nil {
		t.Fatalf("UpsertPosts failed: %v", err)
	}

	// Query 1: FilterAds = true -> Ad post should be excluded
	cleanPosts, err := repo.GetPosts(ctx, domain.PostFilter{
		ChannelIDs: []int64{2001},
		FilterAds:  true,
	})
	if err != nil {
		t.Fatalf("GetPosts with FilterAds failed: %v", err)
	}
	if len(cleanPosts) != 1 {
		t.Errorf("expected 1 clean post, got %d", len(cleanPosts))
	}
	if cleanPosts[0].MsgID != 1 {
		t.Errorf("expected clean post msg_id 1, got %d", cleanPosts[0].MsgID)
	}

	// Query 2: MCPOnly = true -> Posts from quarantined channel 2002 must NEVER be returned!
	mcpPosts, err := repo.GetPosts(ctx, domain.PostFilter{
		MCPOnly: true,
	})
	if err != nil {
		t.Fatalf("GetPosts with MCPOnly failed: %v", err)
	}
	for _, p := range mcpPosts {
		if p.ChannelID == 2002 {
			t.Errorf("SECURITY LEAK: quarantined channel post returned to MCP! post ID: %d", p.ID)
		}
	}

	// Query 3: Search with MCPOnly
	searchResults, err := repo.SearchPosts(ctx, "Secret", true, 10)
	if err != nil {
		t.Fatalf("SearchPosts failed: %v", err)
	}
	if len(searchResults) != 0 {
		t.Errorf("SECURITY LEAK: secret channel returned in MCP search!")
	}
}

func TestDigestPrivacyIsolation(t *testing.T) {
	ctx := context.Background()
	repo := setupTestDB(t)

	// Channel 3001 is MCP allowed, Channel 3002 is Quarantined
	_ = repo.UpsertChannels(ctx, []*domain.Channel{
		{ID: 3001, AccessHash: 1, Title: "Allowed Channel", MCPAllowed: true},
		{ID: 3002, AccessHash: 2, Title: "Quarantined Channel", MCPAllowed: false},
	})

	now := time.Now()

	// Digest A: Contains ONLY allowed channel 3001
	digestA := &domain.Digest{
		ID:              "digest-a",
		Title:           "Public Tech Digest",
		Topic:           "go",
		PeriodStart:     now.Add(-24 * time.Hour),
		PeriodEnd:       now,
		ChannelIDs:      []int64{3001},
		SummaryMarkdown: "# Digest A Content",
		PostCount:       5,
	}
	if err := repo.SaveDigest(ctx, digestA); err != nil {
		t.Fatalf("SaveDigest A failed: %v", err)
	}

	// Digest B: Contains a quarantined channel 3002
	digestB := &domain.Digest{
		ID:              "digest-b",
		Title:           "Mixed Digest",
		Topic:           "general",
		PeriodStart:     now.Add(-24 * time.Hour),
		PeriodEnd:       now,
		ChannelIDs:      []int64{3001, 3002},
		SummaryMarkdown: "# Digest B Content containing private channel summaries",
		PostCount:       10,
	}
	if err := repo.SaveDigest(ctx, digestB); err != nil {
		t.Fatalf("SaveDigest B failed: %v", err)
	}

	// Verify MCPOnly query: Digest B MUST NOT be returned!
	mcpDigests, err := repo.ListDigests(ctx, "all", true, 10)
	if err != nil {
		t.Fatalf("ListDigests failed: %v", err)
	}
	if len(mcpDigests) != 1 {
		t.Fatalf("expected 1 MCP visible digest, got %d", len(mcpDigests))
	}
	if mcpDigests[0].ID != "digest-a" {
		t.Errorf("SECURITY LEAK: expected digest-a, got %s", mcpDigests[0].ID)
	}
}

func TestSessionStorageInDB(t *testing.T) {
	ctx := context.Background()
	repo := setupTestDB(t)

	// Initially session should be nil (not found)
	session, err := repo.LoadSession(ctx, "default")
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}
	if session != nil {
		t.Errorf("expected nil session initially, got %v", session)
	}

	// Store mock session bytes
	mockData := []byte("mtproto-encrypted-session-token-bytes-12345")
	if err := repo.StoreSession(ctx, "default", mockData); err != nil {
		t.Fatalf("StoreSession failed: %v", err)
	}

	// Retrieve session bytes
	loaded, err := repo.LoadSession(ctx, "default")
	if err != nil {
		t.Fatalf("LoadSession after store failed: %v", err)
	}
	if string(loaded) != string(mockData) {
		t.Errorf("expected %s, got %s", string(mockData), string(loaded))
	}

	// Overwrite session bytes
	updatedData := []byte("new-mtproto-session-data-67890")
	if err := repo.StoreSession(ctx, "default", updatedData); err != nil {
		t.Fatalf("StoreSession update failed: %v", err)
	}

	loaded, err = repo.LoadSession(ctx, "default")
	if err != nil {
		t.Fatalf("LoadSession after update failed: %v", err)
	}
	if string(loaded) != string(updatedData) {
		t.Errorf("expected %s, got %s", string(updatedData), string(loaded))
	}
}

func TestAdPatternsPersistence(t *testing.T) {
	ctx := context.Background()
	repo := setupTestDB(t)

	// Verify default seeded patterns exist
	defaults, err := repo.ListAdPatterns(ctx, true)
	if err != nil {
		t.Fatalf("ListAdPatterns failed: %v", err)
	}
	if len(defaults) == 0 {
		t.Errorf("expected default seeded patterns, got 0")
	}

	// Add new custom pattern
	custom := &domain.AdPattern{
		Pattern:     "buy vip access now",
		IsRegex:     false,
		Description: "VIP pump scam",
		Enabled:     true,
	}
	if err := repo.AddAdPattern(ctx, custom); err != nil {
		t.Fatalf("AddAdPattern failed: %v", err)
	}

	patterns, err := repo.ListAdPatterns(ctx, false)
	if err != nil {
		t.Fatalf("ListAdPatterns failed: %v", err)
	}
	var found *domain.AdPattern
	for _, p := range patterns {
		if p.Pattern == "buy vip access now" {
			found = p
			break
		}
	}
	if found == nil {
		t.Fatalf("custom pattern not found in database")
	}

	// Toggle pattern disabled
	if err := repo.ToggleAdPattern(ctx, found.ID, false); err != nil {
		t.Fatalf("ToggleAdPattern failed: %v", err)
	}

	// Enabled-only query should not include it
	enabledList, err := repo.ListAdPatterns(ctx, true)
	if err != nil {
		t.Fatalf("ListAdPatterns enabledOnly failed: %v", err)
	}
	for _, p := range enabledList {
		if p.Pattern == "buy vip access now" {
			t.Errorf("disabled pattern appeared in enabled-only list!")
		}
	}

	// Delete pattern
	if err := repo.DeleteAdPattern(ctx, found.ID); err != nil {
		t.Fatalf("DeleteAdPattern failed: %v", err)
	}
}

func TestSettingsPersistence(t *testing.T) {
	ctx := context.Background()
	repo := setupTestDB(t)

	// Test default fallback
	val, err := repo.GetSetting(ctx, "filter_ads", "true")
	if err != nil {
		t.Fatalf("GetSetting failed: %v", err)
	}
	if val != "true" {
		t.Errorf("expected default 'true', got %s", val)
	}

	// Update setting
	if err := repo.SetSetting(ctx, "filter_ads", "false"); err != nil {
		t.Fatalf("SetSetting failed: %v", err)
	}

	// Retrieve updated setting
	val, err = repo.GetSetting(ctx, "filter_ads", "true")
	if err != nil {
		t.Fatalf("GetSetting after update failed: %v", err)
	}
	if val != "false" {
		t.Errorf("expected updated 'false', got %s", val)
	}
}
