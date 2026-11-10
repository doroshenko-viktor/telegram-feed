package mcp

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"telegram_feed_summarizer/internal/domain"
	"telegram_feed_summarizer/internal/service"
)

// Server encapsulates the MCP server and its dependencies.
type Server struct {
	mcpServer *mcp.Server
	repo      domain.Repository
	guard     *service.PrivacyGuard
}

// NewServer creates and initializes an MCP server exposing high-signal tools to LLM chats.
func NewServer(repo domain.Repository) *Server {
	guard := service.NewPrivacyGuard(repo)
	impl := &mcp.Implementation{
		Name:    "telegram-feed",
		Version: "1.0.0",
	}
	s := mcp.NewServer(impl, nil)

	srv := &Server{
		mcpServer: s,
		repo:      repo,
		guard:     guard,
	}

	srv.registerTools()
	return srv
}

// RawMCPServer returns the underlying mcp.Server pointer.
func (s *Server) RawMCPServer() *mcp.Server {
	return s.mcpServer
}

// RunStdio starts the MCP server over standard I/O for Claude Desktop, Cursor, and Antigravity.
func (s *Server) RunStdio(ctx context.Context) error {
	return s.mcpServer.Run(ctx, &mcp.StdioTransport{})
}

func (s *Server) registerTools() {
	// 1. telegram_list_channels
	type ListChannelsIn struct {
		// Optional topic tag to filter channels by (e.g. 'go', 'kubernetes')
		Topic string `json:"topic,omitempty"`
	}
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "telegram_list_channels",
		Description: "List all Telegram channels allowlisted for LLM access. Excludes quarantined/private channels.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ListChannelsIn) (*mcp.CallToolResult, any, error) {
		channels, err := s.repo.ListMCPAllowedChannels(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("listing channels: %w", err)
		}
		if in.Topic != "" {
			var filtered []*domain.Channel
			for _, ch := range channels {
				for _, t := range ch.Topics {
					if t == in.Topic {
						filtered = append(filtered, ch)
						break
					}
				}
			}
			return nil, filtered, nil
		}
		return nil, channels, nil
	})

	// 2. telegram_get_unread_posts
	type GetUnreadPostsIn struct {
		Topic      string  `json:"topic,omitempty"`
		ChannelIDs []int64 `json:"channel_ids,omitempty"`
		SinceHours int     `json:"since_hours,omitempty"`
		Limit      int     `json:"limit,omitempty"`
		FilterAds  *bool   `json:"filter_ads,omitempty"`
	}
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "telegram_get_unread_posts",
		Description: "Retrieve unread, high-signal posts from allowlisted channels for digest generation or answering questions.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in GetUnreadPostsIn) (*mcp.CallToolResult, any, error) {
		// Zero-Trust Privacy Enforcement: Guarantee unapproved channel IDs are stripped
		safeChannelIDs, err := s.guard.FilterAllowedChannelIDs(ctx, in.ChannelIDs)
		if err != nil {
			return nil, nil, err
		}
		if len(in.ChannelIDs) > 0 && len(safeChannelIDs) == 0 {
			// All requested channels were non-allowlisted
			return nil, []*domain.Post{}, nil
		}

		limit := in.Limit
		if limit <= 0 {
			limit = 50
		}

		sinceHours := in.SinceHours
		if sinceHours <= 0 {
			sinceHours = 24
		}
		since := time.Now().Add(-time.Duration(sinceHours) * time.Hour)

		filterAds := true
		if in.FilterAds != nil {
			filterAds = *in.FilterAds
		} else {
			// Read from DB setting
			val, err := s.repo.GetSetting(ctx, domain.SettingKeyFilterAds, "true")
			if err == nil && val == "false" {
				filterAds = false
			}
		}

		posts, err := s.repo.GetPosts(ctx, domain.PostFilter{
			ChannelIDs: safeChannelIDs,
			Topic:      in.Topic,
			Since:      since,
			Limit:      limit,
			FilterAds:  filterAds,
			UnreadOnly: true,
			MCPOnly:    true, // Strict database level join: c.mcp_allowed = true
		})
		if err != nil {
			return nil, nil, fmt.Errorf("getting posts: %w", err)
		}
		return nil, posts, nil
	})

	// 3. telegram_search_posts
	type SearchPostsIn struct {
		Query string `json:"query"`
		Limit int    `json:"limit,omitempty"`
	}
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "telegram_search_posts",
		Description: "Full-text search across cached messages belonging exclusively to allowlisted channels.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in SearchPostsIn) (*mcp.CallToolResult, any, error) {
		limit := in.Limit
		if limit <= 0 {
			limit = 20
		}
		results, err := s.repo.SearchPosts(ctx, in.Query, true, limit)
		if err != nil {
			return nil, nil, fmt.Errorf("searching posts: %w", err)
		}
		return nil, results, nil
	})

	// 4. telegram_get_past_digests
	type GetPastDigestsIn struct {
		Topic string `json:"topic,omitempty"`
		Limit int    `json:"limit,omitempty"`
	}
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "telegram_get_past_digests",
		Description: "Recall previously generated and stored digests from the database without re-analyzing raw message history.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in GetPastDigestsIn) (*mcp.CallToolResult, any, error) {
		limit := in.Limit
		if limit <= 0 {
			limit = 5
		}
		digests, err := s.repo.ListDigests(ctx, in.Topic, true, limit)
		if err != nil {
			return nil, nil, fmt.Errorf("listing past digests: %w", err)
		}
		return nil, digests, nil
	})

	// 5. telegram_save_digest
	type SaveDigestIn struct {
		Title           string  `json:"title"`
		Topic           string  `json:"topic"`
		SummaryMarkdown string  `json:"summary_markdown"`
		ChannelIDs      []int64 `json:"channel_ids"`
	}
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "telegram_save_digest",
		Description: "Save a newly generated digest to the database for future recall and fast querying.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in SaveDigestIn) (*mcp.CallToolResult, any, error) {
		digestID := fmt.Sprintf("digest-%s-%d", in.Topic, time.Now().Unix())
		digest := &domain.Digest{
			ID:              digestID,
			Title:           in.Title,
			Topic:           in.Topic,
			PeriodStart:     time.Now().Add(-24 * time.Hour),
			PeriodEnd:       time.Now(),
			ChannelIDs:      in.ChannelIDs,
			SummaryMarkdown: in.SummaryMarkdown,
			PostCount:       len(in.ChannelIDs),
			CreatedAt:       time.Now(),
		}

		if err := s.repo.SaveDigest(ctx, digest); err != nil {
			return nil, nil, fmt.Errorf("saving digest: %w", err)
		}

		return nil, map[string]any{
			"id":          digest.ID,
			"mcp_visible": digest.MCPVisible,
			"success":     true,
		}, nil
	})

	// 6. telegram_get_settings
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "telegram_get_settings",
		Description: "Retrieve current application preferences (ad filtering, keywords, etc.) from the database.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		settings, err := s.repo.GetAllSettings(ctx)
		if err != nil {
			return nil, nil, err
		}
		return nil, settings, nil
	})

	// 7. telegram_update_setting
	type UpdateSettingIn struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "telegram_update_setting",
		Description: "Update a persistent application configuration setting in the database.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in UpdateSettingIn) (*mcp.CallToolResult, any, error) {
		if err := s.repo.SetSetting(ctx, in.Key, in.Value); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "key": in.Key, "value": in.Value}, nil
	})

	// 8. telegram_list_ad_patterns
	type ListAdPatternsIn struct {
		EnabledOnly bool `json:"enabled_only,omitempty"`
	}
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "telegram_list_ad_patterns",
		Description: "List all dynamic ad and promotional filtering rules stored in the database.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ListAdPatternsIn) (*mcp.CallToolResult, any, error) {
		patterns, err := s.repo.ListAdPatterns(ctx, in.EnabledOnly)
		if err != nil {
			return nil, nil, err
		}
		return nil, patterns, nil
	})

	// 9. telegram_add_ad_pattern
	type AddAdPatternIn struct {
		Pattern     string `json:"pattern"`
		IsRegex     bool   `json:"is_regex"`
		Description string `json:"description,omitempty"`
	}
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "telegram_add_ad_pattern",
		Description: "Add a new ad or promo detection rule to the database.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in AddAdPatternIn) (*mcp.CallToolResult, any, error) {
		pat := &domain.AdPattern{
			Pattern:     in.Pattern,
			IsRegex:     in.IsRegex,
			Description: in.Description,
			Enabled:     true,
		}
		if err := s.repo.AddAdPattern(ctx, pat); err != nil {
			return nil, nil, fmt.Errorf("adding ad pattern: %w", err)
		}
		return nil, map[string]any{"success": true, "pattern": in.Pattern}, nil
	})

	// 10. telegram_toggle_channel_ad_filter
	type ToggleChannelAdFilterIn struct {
		ChannelID int64 `json:"channel_id"`
		FilterAds bool  `json:"filter_ads"`
	}
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "telegram_toggle_channel_ad_filter",
		Description: "Enable or disable ad filtering for a specific channel.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ToggleChannelAdFilterIn) (*mcp.CallToolResult, any, error) {
		// Verify channel is allowlisted
		safeIDs, err := s.guard.FilterAllowedChannelIDs(ctx, []int64{in.ChannelID})
		if err != nil || len(safeIDs) == 0 {
			return nil, nil, fmt.Errorf("channel %d is not accessible via MCP", in.ChannelID)
		}
		if err := s.repo.SetFilterAds(ctx, []int64{in.ChannelID}, in.FilterAds); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"channel_id": in.ChannelID, "filter_ads": in.FilterAds, "success": true}, nil
	})
}

// ParseChannelIDHelper is a utility for string to int64 conversion if needed.
func ParseChannelIDHelper(s string) int64 {
	id, _ := strconv.ParseInt(s, 10, 64)
	return id
}
