package mcp

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

func TestMCPServerInitialization(t *testing.T) {
	repo := setupTestRepo(t)
	server := NewServer(repo)

	if server == nil {
		t.Fatalf("expected non-nil server")
	}

	raw := server.RawMCPServer()
	if raw == nil {
		t.Fatalf("expected non-nil raw mcp server")
	}
}

func TestMCPSettingsAndAdPatterns(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	server := NewServer(repo)

	// Verify default settings can be read and updated
	if err := server.repo.SetSetting(ctx, "filter_ads", "true"); err != nil {
		t.Fatalf("failed to set setting: %v", err)
	}
	val, err := server.repo.GetSetting(ctx, "filter_ads", "false")
	if err != nil || val != "true" {
		t.Errorf("expected setting 'true', got %s", val)
	}

	// Verify ad pattern addition
	pat := &domain.AdPattern{
		Pattern:     "test ad pattern",
		IsRegex:     false,
		Description: "test description",
		Enabled:     true,
	}
	if err := server.repo.AddAdPattern(ctx, pat); err != nil {
		t.Fatalf("failed to add ad pattern: %v", err)
	}

	patterns, err := server.repo.ListAdPatterns(ctx, true)
	if err != nil {
		t.Fatalf("failed to list ad patterns: %v", err)
	}

	found := false
	for _, p := range patterns {
		if p.Pattern == "test ad pattern" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected to find newly added ad pattern in list")
	}
}
