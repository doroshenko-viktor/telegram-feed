package service

import (
	"context"
	"fmt"

	"telegram_feed_summarizer/internal/domain"
)

// PrivacyGuard enforces zero-trust boundaries between user Telegram data and external MCP clients.
type PrivacyGuard struct {
	repo domain.Repository
}

// NewPrivacyGuard creates a new privacy guard instance.
func NewPrivacyGuard(repo domain.Repository) *PrivacyGuard {
	return &PrivacyGuard{repo: repo}
}

// FilterAllowedChannelIDs filters requested channel IDs so that only explicitly allowlisted channels are returned.
// If requestedIDs is empty, it returns all currently allowlisted channel IDs.
func (g *PrivacyGuard) FilterAllowedChannelIDs(ctx context.Context, requestedIDs []int64) ([]int64, error) {
	allowedChannels, err := g.repo.ListMCPAllowedChannels(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing mcp allowed channels: %w", err)
	}

	allowedMap := make(map[int64]bool, len(allowedChannels))
	for _, ch := range allowedChannels {
		allowedMap[ch.ID] = true
	}

	// If no specific channels were requested, return all allowed channels
	if len(requestedIDs) == 0 {
		result := make([]int64, 0, len(allowedChannels))
		for _, ch := range allowedChannels {
			result = append(result, ch.ID)
		}
		return result, nil
	}

	// Filter requested channels: strictly drop any unapproved IDs!
	var safeIDs []int64
	for _, id := range requestedIDs {
		if allowedMap[id] {
			safeIDs = append(safeIDs, id)
		}
	}
	return safeIDs, nil
}

// IsDigestVisibleToMCP verifies whether a digest can be exposed over MCP.
// It returns true ONLY if 100% of the channels included in the digest are allowlisted.
func (g *PrivacyGuard) IsDigestVisibleToMCP(ctx context.Context, digest *domain.Digest) (bool, error) {
	if digest == nil || len(digest.ChannelIDs) == 0 {
		return false, nil
	}

	safeIDs, err := g.FilterAllowedChannelIDs(ctx, digest.ChannelIDs)
	if err != nil {
		return false, err
	}

	return len(safeIDs) == len(digest.ChannelIDs), nil
}
